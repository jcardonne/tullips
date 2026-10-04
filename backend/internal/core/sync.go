package core

import (
	"context"
	"time"
)

func (s *Server) SyncAccounts(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
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
			rows, e := s.DB.Query(ctx, `SELECT id,workspace_id,data->>'channel' FROM records WHERE kind='accounts' AND data->>'status'='connected'`)
			if e != nil {
				continue
			}
			type item struct{ id, ws, channel string }
			var accounts []item
			for rows.Next() {
				var a item
				rows.Scan(&a.id, &a.ws, &a.channel)
				accounts = append(accounts, a)
			}
			rows.Close()
			for _, a := range accounts {
				if ctx.Err() != nil || s.Maintenance(ctx) {
					return
				}
				jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
				if a.channel == "email" {
					s.SyncMail(jobCtx, a.ws, a.id)
				} else {
					s.SyncLinkedIn(jobCtx, a.ws, a.id)
				}
				cancel()
			}
		}
	}
}
