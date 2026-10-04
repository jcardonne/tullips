package core

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/tullips/tullips/backend/internal/automation"
	"net/http"
	"os"
	"time"
)

func (s *Server) sequenceAction(w http.ResponseWriter, r *http.Request, user, ws, kind, id, action string, scopes []string) {
	if r.Method != "POST" {
		fail(w, 405, "method not allowed")
		return
	}
	if kind == "campaigns" && action == "launch" {
		if !has(scopes, "launch") {
			fail(w, 403, "launch scope required")
			return
		}
		s.launch(w, r, ws, id)
		return
	}
	if kind == "campaigns" && action == "approve_sample" {
		if !has(scopes, "send") {
			fail(w, 403, "send scope required")
			return
		}
		tx, e := s.DB.Begin(r.Context())
		if e != nil {
			s.result(w, e, nil)
			return
		}
		defer tx.Rollback(r.Context())
		tag, e := tx.Exec(r.Context(), `UPDATE records SET data=data || '{"review_approved":true}'::jsonb WHERE id::text=$1 AND workspace_id=$2 AND kind='campaigns' AND data->>'autonomy'='review'`, id, ws)
		if e == nil && tag.RowsAffected() == 0 {
			fail(w, 400, "sample approval applies to review campaigns only")
			return
		}
		if e == nil {
			_, e = tx.Exec(r.Context(), `UPDATE jobs SET status='pending',updated_at=now() WHERE campaign_id::text=$1 AND workspace_id=$2 AND status='awaiting_approval'`, id, ws)
		}
		if e == nil {
			e = tx.Commit(r.Context())
		}
		s.result(w, e, map[string]bool{"approved": true})
		return
	}
	if kind == "conversations" && action == "reply" {
		if !has(scopes, "send") {
			fail(w, 403, "send scope required")
			return
		}
		d, e := decode(w, r)
		if e != nil || str(d, "text") == "" {
			fail(w, 400, "text required")
			return
		}
		var raw []byte
		e = s.DB.QueryRow(r.Context(), `SELECT data FROM records WHERE id::text=$1 AND workspace_id=$2 AND kind='conversations'`, id, ws).Scan(&raw)
		if e != nil {
			s.result(w, e, nil)
			return
		}
		var conv map[string]any
		json.Unmarshal(raw, &conv)
		payload, _ := json.Marshal(map[string]any{"text": str(d, "text"), "conversation_id": id, "account_id": conv["account_id"], "prospect_id": conv["prospect_id"]})
		state := "blocked"
		if os.Getenv("LIVE_SENDS") == "true" {
			state = "pending"
		}
		var job string
		e = s.DB.QueryRow(r.Context(), `INSERT INTO jobs(workspace_id,kind,payload,status,last_error,account_id) VALUES($1,'message.send',$2,$3,$4,$5) RETURNING id`, ws, payload, state, "Provider must be connected and verified before delivery", str(conv, "account_id")).Scan(&job)
		s.result(w, e, map[string]string{"id": job, "status": state})
		return
	}
	fail(w, 404, "not found")
}
func (s *Server) launch(w http.ResponseWriter, r *http.Request, ws, id string) {
	result, e := s.EnrollCampaign(r.Context(), ws, id, true)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	JSON(w, 200, result)
}
func (s *Server) EnrollCampaign(ctx context.Context, ws, id string, activate bool) (map[string]any, error) {
	if !s.PaidActive(ctx, ws) {
		return nil, errors.New("active subscription required to launch campaigns")
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var raw []byte
	e = tx.QueryRow(ctx, `SELECT data FROM records WHERE id::text=$1 AND workspace_id=$2 AND kind='campaigns' FOR UPDATE`, id, ws).Scan(&raw)
	if e != nil {
		return nil, e
	}
	var campaign map[string]any
	json.Unmarshal(raw, &campaign)
	if !activate && str(campaign, "status") != "active" {
		return map[string]any{"enrolled": 0}, nil
	}
	accounts, ok := campaign["account_ids"].([]any)
	if !ok || len(accounts) == 0 {
		return nil, errors.New("select at least one sender account")
	}
	workflow, ok := campaign["workflow"].([]any)
	if !ok || len(workflow) == 0 {
		return nil, errors.New("add workflow steps before launching")
	}
	flow, _ := json.Marshal(workflow)
	var senderIDs []string
	capacity := map[string]int{}
	windows := map[string]automation.Window{}
	for _, v := range accounts {
		aid, ok := v.(string)
		if !ok {
			continue
		}
		var found string
		if e = tx.QueryRow(ctx, `SELECT id FROM records WHERE id::text=$1 AND workspace_id=$2 AND kind='accounts'`, aid, ws).Scan(&found); e != nil {
			return nil, errors.New("invalid sender account")
		}
		var created time.Time
		var accountRaw []byte
		tx.QueryRow(ctx, `SELECT created_at,data FROM records WHERE id=$1`, found).Scan(&created, &accountRaw)
		var accountData map[string]any
		json.Unmarshal(accountRaw, &accountData)
		windows[found] = window(accountData)
		limit, limitErr := s.accountLimit(ctx, ws, accountData, created)
		if limitErr != nil {
			return nil, limitErr
		}
		var used int
		tx.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE account_id=$1 AND (status IN ('pending','running','awaiting_approval','blocked') OR (status='completed' AND updated_at::date=current_date))`, found).Scan(&used)
		capacity[found] = limit - used
		senderIDs = append(senderIDs, found)
	}
	if len(senderIDs) == 0 {
		return nil, errors.New("select sender accounts")
	}
	rows, e := tx.Query(ctx, `SELECT id FROM records WHERE workspace_id=$1 AND kind='prospects' AND data->>'campaign_id'=$2 AND coalesce(data->>'status','') NOT IN ('replied','unsubscribed','do_not_contact') AND (coalesce(data->>'source','')<>'linkedin_search' OR data->>'qualification'='matched' OR data->>'status'='qualified' OR data->>'qualification'='manual') AND NOT EXISTS(SELECT 1 FROM enrollments e WHERE e.campaign_id::text=$2 AND e.prospect_id=records.id)`, ws, id)
	if e != nil {
		return nil, e
	}
	var prospects []string
	for rows.Next() {
		var p string
		rows.Scan(&p)
		prospects = append(prospects, p)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return nil, e
	}
	n := 0
	for _, p := range prospects {
		account := senderIDs[0]
		for _, candidate := range senderIDs {
			if capacity[candidate] > capacity[account] {
				account = candidate
			}
		}
		capacity[account]--
		prospectFlow := flow
		var custom []byte
		if tx.QueryRow(ctx, `SELECT data->'workflow' FROM records WHERE id=$1`, p).Scan(&custom) == nil && len(custom) > 2 && string(custom) != "null" {
			prospectFlow = custom
		}
		var enrollment string
		e = tx.QueryRow(ctx, `INSERT INTO enrollments(workspace_id,campaign_id,prospect_id,account_id,workflow) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING RETURNING id`, ws, id, p, account, prospectFlow).Scan(&enrollment)
		if e != nil {
			continue
		}
		state := "awaiting_approval"
		reviewApproved, _ := campaign["review_approved"].(bool)
		if str(campaign, "autonomy") == "automatic" || (str(campaign, "autonomy") == "review" && reviewApproved) {
			state = "pending"
		}
		due, _ := automation.NextActive(time.Now(), windows[account])
		payload, _ := json.Marshal(map[string]any{"enrollment_id": enrollment, "step": workflow[0]})
		_, e = tx.Exec(ctx, `INSERT INTO jobs(workspace_id,campaign_id,prospect_id,account_id,kind,payload,status,run_at,idempotency_key,last_error) VALUES($1,$2,$3,$4,'workflow.step',$5,$6,$7,$8,'Provider execution requires a connected and verified account')`, ws, id, p, account, payload, state, due, enrollment+":0")
		if e != nil {
			return nil, e
		}
		n++
	}
	_, e = tx.Exec(ctx, `UPDATE records SET data=data || '{"status":"active"}'::jsonb,updated_at=now() WHERE id=$1`, id)
	if e == nil {
		e = tx.Commit(ctx)
	}
	return map[string]any{"enrolled": n, "status": "active"}, e
}
