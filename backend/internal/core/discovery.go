package core

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

func (s *Server) DiscoverCampaigns(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ctx.Err() != nil {
				return
			}
			if s.Maintenance(ctx) {
				continue
			}
			work, cancel := context.WithTimeout(context.WithoutCancel(ctx), 4*time.Minute)
			s.scheduleDiscovery(work)
			cancel()
		}
	}
}
func (s *Server) scheduleDiscovery(ctx context.Context) {
	rows, e := s.DB.Query(ctx, `SELECT id,workspace_id,data FROM records WHERE kind='campaigns' AND data->>'status'='active'`)
	if e != nil {
		return
	}
	type item struct {
		id, ws string
		raw    []byte
	}
	var all []item
	for rows.Next() {
		var v item
		rows.Scan(&v.id, &v.ws, &v.raw)
		all = append(all, v)
	}
	rows.Close()
	for _, v := range all {
		var campaign map[string]any
		json.Unmarshal(v.raw, &campaign)
		s.EnrollCampaign(ctx, v.ws, v.id, false)
		target := number(campaign, "target_count", 100)
		var count, pending, done int
		s.DB.QueryRow(ctx, `SELECT count(*) FROM records WHERE workspace_id=$1 AND kind='prospects' AND data->>'campaign_id'=$2`, v.ws, v.id).Scan(&count)
		if count >= target {
			continue
		}
		s.DB.QueryRow(ctx, `SELECT count(*) FILTER(WHERE status IN ('pending','running','blocked')),count(*) FILTER(WHERE status='completed') FROM jobs WHERE workspace_id=$1 AND kind='linkedin.search' AND payload->>'campaign_id'=$2`, v.ws, v.id).Scan(&pending, &done)
		if pending > 0 {
			continue
		}
		offset := done * 10
		if done > 0 {
			var found int
			var completed time.Time
			if s.DB.QueryRow(ctx, `SELECT coalesce((payload->'result'->>'found')::int,0),updated_at FROM jobs WHERE workspace_id=$1 AND kind='linkedin.search' AND payload->>'campaign_id'=$2 AND status='completed' ORDER BY updated_at DESC LIMIT 1`, v.ws, v.id).Scan(&found, &completed) == nil && found == 0 {
				if str(campaign, "search_mode") == "once" || time.Since(completed) < 24*time.Hour {
					continue
				}
				offset = 0
			}
		}
		ids, ok := campaign["account_ids"].([]any)
		if !ok || len(ids) == 0 {
			continue
		}
		account, _ := ids[done%len(ids)].(string)
		criteria, _ := campaign["criteria"].(map[string]any)
		terms := []string{}
		for _, k := range []string{"roles", "geography", "industry", "company_size", "seniority"} {
			if value := str(criteria, k); value != "" {
				terms = append(terms, value)
			}
		}
		if len(terms) == 0 {
			continue
		}
		payload, _ := json.Marshal(map[string]any{"account_id": account, "campaign_id": v.id, "keywords": strings.Join(terms, " "), "start": offset, "remaining": target - count})
		s.DB.Exec(ctx, `INSERT INTO jobs(workspace_id,campaign_id,account_id,kind,payload,idempotency_key) VALUES($1,$2,$3,'linkedin.search',$4,$5) ON CONFLICT DO NOTHING`, v.ws, v.id, account, payload, v.id+":search:"+time.Now().UTC().Format("2006-01-02T15:04"))
	}
}
