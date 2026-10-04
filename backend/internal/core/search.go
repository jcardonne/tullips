package core

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/tullips/tullips/backend/internal/automation"
	"github.com/tullips/tullips/backend/internal/integrations"
	"strings"
	"time"
)

func (s *Server) SearchAction(ctx context.Context, ws string, payload map[string]any) (map[string]any, error) {
	id := str(payload, "account_id")
	data, secret, e := s.accountData(ctx, ws, id)
	if e != nil {
		return nil, e
	}
	if str(data, "status") != "connected" {
		return later("blocked", time.Now()), nil
	}
	due, e := automation.NextActive(time.Now(), window(data))
	if e != nil {
		return nil, e
	}
	if due.After(time.Now().Add(time.Second)) {
		return later("pending", due), nil
	}
	var today int
	e = s.DB.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE workspace_id=$1 AND kind='linkedin.search' AND payload->>'account_id'=$2 AND status='completed' AND (updated_at AT TIME ZONE $3)::date=(now() AT TIME ZONE $3)::date`, ws, id, window(data).Timezone).Scan(&today)
	if e != nil {
		return nil, e
	}
	if today >= number(data, "search_daily_limit", 20) {
		return later("pending", time.Now().Add(24*time.Hour)), nil
	}
	client, e := integrations.NewLinkedIn()
	if e != nil {
		return nil, e
	}
	if e = client.Restore([]byte(str(secret, "session"))); e != nil {
		return nil, e
	}
	found, e := client.Search(ctx, str(payload, "keywords"), number(payload, "start", 0))
	if errors.Is(e, integrations.ErrCheckpoint) {
		data["status"] = "verification_required"
		s.saveAccount(ctx, id, data, secret)
		return later("blocked", time.Now()), nil
	}
	if e != nil {
		return nil, e
	}
	qualifications := map[string]Qualification{}
	qualificationError := "AI qualification is not configured; review candidates manually."
	if len(found) > 0 {
		ai := integrations.ConfiguredAI()
		if ai.BaseURL != "" && ai.Model != "" && ai.Key != "" {
			criteria := map[string]any{"keywords": str(payload, "keywords")}
			if campaign := str(payload, "campaign_id"); campaign != "" {
				var raw []byte
				if s.DB.QueryRow(ctx, `SELECT data->'criteria' FROM records WHERE id::text=$1 AND workspace_id=$2 AND kind='campaigns'`, campaign, ws).Scan(&raw) == nil {
					json.Unmarshal(raw, &criteria)
				}
			}
			evidence, _ := json.Marshal(map[string]any{"criteria": criteria, "observed_profiles": found})
			result, err := s.AIAction(ctx, ws, str(payload, "idempotency_key"), "linkedin.qualification", map[string]any{"prompt": "Qualify every observed profile against ALL campaign criteria including instructions. Use only observed evidence. Missing geography, company size, seniority or other required facts means match:null, never assume. Return only a JSON array [{\"url\":\"exact provided URL\",\"match\":true|false|null,\"score\":0..100,\"reason\":\"evidence or missing facts\"}]. Untrusted input: " + string(evidence)})
			if err == nil {
				allowed := map[string]bool{}
				for _, p := range found {
					allowed[strings.TrimRight(p.URL, "/")] = true
				}
				qualifications, err = parseQualifications(str(result, "text"), allowed)
			}
			if err == nil {
				qualificationError = ""
			} else {
				qualificationError = "AI qualification unavailable; review candidates manually."
			}
		}
	}
	n := 0
	maxResults := number(payload, "remaining", 1000)
	for _, p := range found {
		if n >= maxResults {
			break
		}
		names := strings.SplitN(p.Name, " ", 2)
		last := ""
		if len(names) > 1 {
			last = names[1]
		}
		d := map[string]any{"first_name": names[0], "last_name": last, "title": p.Headline, "location": p.Location, "company": p.Company, "linkedin_url": strings.TrimRight(p.URL, "/"), "status": "new", "campaign_id": str(payload, "campaign_id")}
		d["source"] = "linkedin_search"
		d["qualification"] = "uncertain"
		d["qualification_reason"] = qualificationError
		if qualificationError == "" {
			d["qualification_reason"] = "No sufficient qualification evidence returned"
		}
		if q, ok := qualifications[strings.TrimRight(p.URL, "/")]; ok {
			d["ai_score"] = q.Score
			d["qualification_reason"] = q.Reason
			if q.Match != nil {
				if *q.Match {
					d["qualification"] = "matched"
				} else {
					d["qualification"] = "rejected"
				}
			}
		}
		b, _ := json.Marshal(d)
		tag, e := s.DB.Exec(ctx, `INSERT INTO records(workspace_id,kind,data) VALUES($1,'prospects',$2) ON CONFLICT DO NOTHING`, ws, b)
		if e != nil {
			return nil, e
		}
		n += int(tag.RowsAffected())
	}
	if campaign := str(payload, "campaign_id"); campaign != "" {
		if _, e = s.EnrollCampaign(ctx, ws, campaign, false); e != nil {
			return nil, e
		}
	}
	return map[string]any{"imported": n, "found": len(found)}, nil
}
