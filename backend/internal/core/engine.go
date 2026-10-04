package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tullips/tullips/backend/internal/automation"
	"github.com/tullips/tullips/backend/internal/integrations"
	"os"
	"strings"
	"time"
)

func blocked(message string) map[string]any {
	return map[string]any{"__status": "blocked", "__run_at": time.Now(), "__error": message}
}
func later(state string, at time.Time) map[string]any {
	return map[string]any{"__status": state, "__run_at": at}
}
func number(m map[string]any, k string, def int) int {
	if n, ok := m[k].(float64); ok {
		return int(n)
	}
	return def
}
func window(account map[string]any) automation.Window {
	w := automation.DefaultWindow()
	if v := str(account, "timezone"); v != "" {
		w.Timezone = v
	}
	w.StartHour = number(account, "start_hour", 9)
	w.EndHour = number(account, "end_hour", 18)
	if days, ok := account["working_days"].([]any); ok {
		w.WorkingDays = []int{}
		for _, v := range days {
			if n, ok := v.(float64); ok {
				w.WorkingDays = append(w.WorkingDays, int(n))
			}
		}
	}
	return w
}
func facts(p map[string]any) automation.Facts {
	f := automation.Facts{Accepted: str(p, "accepted_at") != "" || str(p, "status") == "connected", Replied: str(p, "replied_at") != "" || str(p, "status") == "replied", HasEmail: str(p, "email") != "", Status: str(p, "status")}
	f.EmailOpened, _ = p["email_opened"].(bool)
	f.LinkClicked, _ = p["link_clicked"].(bool)
	f.MessageSeen, _ = p["message_seen"].(bool)
	if tags, ok := p["tags"].([]any); ok {
		for _, v := range tags {
			if t, ok := v.(string); ok {
				f.Tags = append(f.Tags, t)
			}
		}
	}
	return f
}
func renderMessage(text string, p map[string]any) string {
	for _, k := range []string{"first_name", "last_name", "company", "title"} {
		text = strings.ReplaceAll(text, "{{"+k+"}}", str(p, k))
	}
	return text
}
func (s *Server) WorkflowStep(ctx context.Context, ws, job string, payload map[string]any) (map[string]any, error) {
	enrollment := str(payload, "enrollment_id")
	var campaignID, prospectID, accountID, state string
	var stepIndex int
	var raw, pr, cr []byte
	e := s.DB.QueryRow(ctx, `SELECT e.campaign_id,e.prospect_id,e.account_id,e.status,e.step,e.workflow,p.data,c.data FROM enrollments e JOIN records p ON p.id=e.prospect_id JOIN records c ON c.id=e.campaign_id WHERE e.id::text=$1 AND e.workspace_id=$2`, enrollment, ws).Scan(&campaignID, &prospectID, &accountID, &state, &stepIndex, &raw, &pr, &cr)
	if e != nil {
		return nil, e
	}
	var workflow []map[string]any
	var prospect, campaign map[string]any
	json.Unmarshal(raw, &workflow)
	json.Unmarshal(pr, &prospect)
	json.Unmarshal(cr, &campaign)
	if state != "active" {
		return map[string]any{"stopped": true}, nil
	}
	f := facts(prospect)
	if f.Replied || f.Status == "unsubscribed" || f.Status == "do_not_contact" {
		s.DB.Exec(ctx, `UPDATE enrollments SET status='stopped' WHERE id=$1`, enrollment)
		return map[string]any{"stopped": true}, nil
	}
	if str(campaign, "status") != "active" {
		return later("pending", time.Now().Add(time.Hour)), nil
	}
	if stepIndex >= len(workflow) {
		s.DB.Exec(ctx, `UPDATE enrollments SET status='completed' WHERE id=$1`, enrollment)
		return map[string]any{"completed": true}, nil
	}
	step := workflow[stepIndex]
	kind := str(step, "type")
	senderID := accountID
	if kind == "email" {
		senderID = str(step, "email_account_id")
		if senderID == "" {
			senderID = str(campaign, "email_account_id")
		}
		if senderID == "" {
			return later("blocked", time.Now()), nil
		}
	}
	account, secret, e := s.accountData(ctx, ws, senderID)
	if e != nil {
		return nil, e
	}
	if kind == "email" {
		if str(account, "channel") != "email" {
			return nil, errors.New("email step requires an email sender account")
		}
		s.DB.Exec(ctx, `UPDATE jobs SET account_id=$2 WHERE id=$1`, job, senderID)
	}
	w := window(account)
	now := time.Now()
	if kind == "delay" {
		anchor, err := time.Parse(time.RFC3339Nano, str(payload, "anchor_at"))
		if err != nil {
			if s.DB.QueryRow(ctx, `SELECT created_at FROM jobs WHERE id=$1`, job).Scan(&anchor) != nil {
				anchor = now
			}
		}
		due, err := automation.BusinessDays(anchor, number(step, "days", 0), w)
		if err != nil {
			return nil, err
		}
		return s.advance(ctx, ws, enrollment, campaignID, prospectID, accountID, stepIndex, workflow, campaign, due)
	}
	next, e := automation.NextActive(now, w)
	if e != nil {
		return nil, e
	}
	if next.After(now.Add(time.Second)) {
		return later("pending", next), nil
	}
	switch kind {
	case "condition":
		condition := str(step, "condition")
		if condition == "email_clicked" {
			condition = "link_clicked"
		}
		if condition == "tag_present" {
			condition = "tag"
		}
		if condition == "status_matches" {
			condition = "status"
		}
		pass := false
		if condition == "accepted" && !f.Accepted && str(account, "status") == "connected" {
			var checked int
			s.DB.QueryRow(ctx, `SELECT count(*) FROM events WHERE workspace_id=$1 AND record_id=$2 AND action='profile.check' AND created_at::date=current_date`, ws, accountID).Scan(&checked)
			if checked < number(account, "profile_daily_limit", 40) {
				client, probeErr := integrations.NewLinkedIn()
				if probeErr == nil {
					probeErr = client.Restore([]byte(str(secret, "session")))
				}
				if probeErr == nil {
					s.DB.Exec(ctx, `INSERT INTO events(workspace_id,actor,action,record_id) VALUES($1,'worker','profile.check',$2)`, ws, accountID)
					connected, probeErr := client.IsConnected(ctx, str(prospect, "linkedin_url"))
					if probeErr == nil && connected {
						f.Accepted = true
						prospect["accepted_at"] = now.Format(time.RFC3339)
						s.DB.Exec(ctx, `UPDATE records SET data=data || jsonb_build_object('accepted_at',$2::text) WHERE id=$1`, prospectID, now.Format(time.RFC3339))
					}
				}
			}
		}
		if condition == "ai" {
			evidence, _ := json.Marshal(prospect)
			result, err := s.AIAction(ctx, ws, job, "workflow.condition", map[string]any{"prompt": "Answer only true or false. Evaluate this condition: " + str(step, "value") + ". Prospect data: " + string(evidence)})
			if err != nil {
				return nil, err
			}
			value := strings.TrimSpace(strings.ToLower(str(result, "text")))
			if value != "true" && value != "false" {
				return nil, errors.New("AI condition must return true or false")
			}
			pass = value == "true"
		} else if condition == "score" {
			pass = number(prospect, "score", 0) >= number(step, "value", 0)
		} else if condition == "field" {
			pass = str(prospect, str(step, "field")) == str(step, "value")
		} else if condition == "no_email" {
			pass = !f.HasEmail
		} else {
			pass, e = automation.Evaluate(automation.Condition{Kind: condition, Value: str(step, "value")}, f)
			if e != nil {
				return nil, e
			}
		}
		if branch, ok := step[map[bool]string{true: "then_step", false: "else_step"}[pass]].(float64); ok {
			target := int(branch)
			if target <= stepIndex || target >= len(workflow) {
				return nil, errors.New("branch target must be a later existing step")
			}
			return s.advance(ctx, ws, enrollment, campaignID, prospectID, accountID, stepIndex, workflow, campaign, now, target)
		}
		if !pass {
			if condition == "accepted" {
				invited, _ := time.Parse(time.RFC3339, str(prospect, "invited_at"))
				if !invited.IsZero() && !now.Before(invited.AddDate(0, 0, number(step, "timeout_days", 30))) {
					s.DB.Exec(ctx, `UPDATE enrollments SET status='expired' WHERE id=$1`, enrollment)
					s.DB.Exec(ctx, `UPDATE records SET data=data || '{"status":"invitation_not_accepted"}'::jsonb WHERE id=$1`, prospectID)
					return map[string]any{"expired": true}, nil
				}
			}
			return later("pending", now.Add(time.Hour)), nil
		}
	case "stop":
		s.DB.Exec(ctx, `UPDATE enrollments SET status='completed' WHERE id=$1`, enrollment)
		return map[string]any{"completed": true}, nil
	case "tag":
		if tag := str(step, "value"); tag != "" {
			_, e = s.DB.Exec(ctx, `UPDATE records SET data=jsonb_set(data,'{tags}',coalesce(data->'tags','[]'::jsonb) || to_jsonb($2::text)) WHERE id=$1`, prospectID, tag)
			if e != nil {
				return nil, e
			}
		}
	case "invitation", "linkedin_message", "email":
		if !s.PaidActive(ctx, ws) {
			return blocked("Active subscription required"), nil
		}
		if os.Getenv("LIVE_SENDS") != "true" || str(account, "status") != "connected" {
			return later("blocked", now), nil
		}
		var activated time.Time
		s.DB.QueryRow(ctx, `SELECT created_at FROM records WHERE id=$1`, senderID).Scan(&activated)
		limit, limitErr := s.accountLimit(ctx, ws, account, activated)
		if limitErr != nil {
			return nil, limitErr
		}
		var count int
		e = s.DB.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE account_id=$1 AND status='completed' AND (kind='message.send' OR (kind='workflow.step' AND payload->'step'->>'type' IN ('invitation','linkedin_message','email'))) AND (updated_at AT TIME ZONE $2)::date=(now() AT TIME ZONE $2)::date`, senderID, w.Timezone).Scan(&count)
		if e != nil {
			return nil, e
		}
		if count >= limit {
			due, _ := automation.NextActive(now.AddDate(0, 0, 1), w)
			return later("pending", due), nil
		}
		if kind == "email" {
			m, err := s.mailbox(ctx, senderID, account, secret)
			if err != nil {
				return nil, err
			}
			err = m.Send(ctx, integrations.Email{From: m.Email, To: str(prospect, "email"), Subject: renderMessage(str(step, "subject"), prospect), Text: renderMessage(str(step, "message"), prospect), MessageID: job + "@tullips.local"})
			if err != nil {
				return nil, err
			}
			return s.advance(ctx, ws, enrollment, campaignID, prospectID, accountID, stepIndex, workflow, campaign, now)
		}
		client, e := integrations.NewLinkedIn()
		if e != nil {
			return nil, e
		}
		if e = client.Restore([]byte(str(secret, "session"))); e != nil {
			return nil, e
		}
		if kind == "invitation" {
			e = client.Invite(ctx, str(prospect, "linkedin_url"))
		} else {
			if str(prospect, "conversation_urn") == "" {
				return blocked("No LinkedIn conversation exists yet. Start the conversation on LinkedIn and sync this account before retrying."), nil
			}
			if !f.Accepted {
				return later("pending", now.Add(time.Hour)), nil
			}
			_, e = client.SendMessage(ctx, str(prospect, "conversation_urn"), str(account, "mailbox_urn"), renderMessage(str(step, "message"), prospect), job, job)
		}
		if errors.Is(e, integrations.ErrCheckpoint) {
			account["status"] = "verification_required"
			secret["checkpoint"] = client.CheckpointURL()
			s.saveAccount(ctx, accountID, account, secret)
			return later("blocked", now), nil
		}
		if e != nil {
			return nil, e
		}
		session, _ := client.Session()
		secret["session"] = string(session)
		if e = s.saveAccount(ctx, accountID, account, secret); e != nil {
			return nil, e
		}
		if kind == "invitation" {
			_, e = s.DB.Exec(ctx, `UPDATE records SET data=data || jsonb_build_object('invited_at',$2::text,'status','invited') WHERE id=$1`, prospectID, now.Format(time.RFC3339))
			if e != nil {
				return nil, e
			}
		}
	default:
		return nil, fmt.Errorf("unsupported workflow step %q", kind)
	}
	return s.advance(ctx, ws, enrollment, campaignID, prospectID, accountID, stepIndex, workflow, campaign, now)
}
func (s *Server) advance(ctx context.Context, ws, enrollment, campaign, prospect, account string, index int, workflow []map[string]any, campaignData map[string]any, due time.Time, branch ...int) (map[string]any, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	nextIndex := index + 1
	if len(branch) > 0 {
		nextIndex = branch[0]
	}
	status := "active"
	if nextIndex >= len(workflow) {
		status = "completed"
	}
	tag, e := tx.Exec(ctx, `UPDATE enrollments SET step=$4,status=$3 WHERE id=$1 AND step=$2`, enrollment, index, status, nextIndex)
	if e != nil {
		return nil, e
	}
	if tag.RowsAffected() > 0 && status == "active" {
		next := workflow[nextIndex]
		state := "pending"
		if str(campaignData, "autonomy") == "manual" && (str(next, "type") == "invitation" || str(next, "type") == "linkedin_message" || str(next, "type") == "email") {
			state = "awaiting_approval"
		}
		b, _ := json.Marshal(map[string]any{"enrollment_id": enrollment, "step": next, "anchor_at": time.Now().Format(time.RFC3339Nano)})
		_, e = tx.Exec(ctx, `INSERT INTO jobs(workspace_id,campaign_id,prospect_id,account_id,kind,payload,status,run_at,idempotency_key) VALUES($1,$2,$3,$4,'workflow.step',$5,$6,$7,$8) ON CONFLICT DO NOTHING`, ws, campaign, prospect, account, b, state, due, fmt.Sprintf("%s:%d", enrollment, nextIndex))
		if e != nil {
			return nil, e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return map[string]any{"advanced": true}, nil
}
