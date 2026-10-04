package core

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/tullips/tullips/backend/internal/integrations"
	"strings"
	"time"
)

func (s *Server) SyncLinkedIn(ctx context.Context, ws, id string) (int, error) {
	data, secret, e := s.accountData(ctx, ws, id)
	if e != nil {
		return 0, e
	}
	client, e := integrations.NewLinkedIn()
	if e != nil {
		return 0, e
	}
	if e = client.Restore([]byte(str(secret, "session"))); e != nil {
		return 0, e
	}
	mailbox := str(data, "mailbox_urn")
	if mailbox == "" {
		return 0, errors.New("mailbox identity missing; reconnect LinkedIn")
	}
	raw, e := client.Conversations(ctx, mailbox)
	if errors.Is(e, integrations.ErrCheckpoint) {
		if e = s.reconnect(ctx, id, data, secret, client); e == nil {
			raw, e = client.Conversations(ctx, mailbox)
		}
	}
	if e != nil {
		return 0, e
	}
	connections, _ := integrations.ParseConnections(raw)
	for _, connection := range connections {
		s.DB.Exec(ctx, `UPDATE records SET data=data || jsonb_build_object('accepted_at',$3::text,'conversation_urn',$4::text,'profile_urn',$5::text) WHERE workspace_id=$1 AND kind='prospects' AND data->>'linkedin_url'=$2 AND coalesce(data->>'accepted_at','')=''`, ws, strings.TrimRight(connection.ProfileURL, "/"), time.Now().Format(time.RFC3339), connection.ConversationURN, connection.ProfileURN)
	}
	urns, e := integrations.ConversationURNs(raw)
	if e != nil {
		return 0, e
	}
	n := 0
	for _, urn := range urns {
		raw, e = client.Messages(ctx, urn)
		if e != nil {
			return n, e
		}
		messages, e := integrations.ParseMessages(raw)
		if e != nil {
			return n, e
		}
		for _, m := range messages {
			if m.SenderURN == mailbox || m.SenderProfile == "" || m.ID == "" {
				continue
			}
			profile := strings.TrimRight(strings.Split(m.SenderProfile, "?")[0], "/")
			var prospect string
			e = s.DB.QueryRow(ctx, `SELECT id FROM records WHERE workspace_id=$1 AND kind='prospects' AND (data->>'linkedin_url'=$2 OR data->>'conversation_urn'=$3)`, ws, profile, urn).Scan(&prospect)
			if e != nil {
				continue
			}
			tx, e := s.DB.Begin(ctx)
			if e != nil {
				return n, e
			}
			key := id + ":" + m.ID
			when := time.UnixMilli(m.DeliveredAt)
			b, _ := json.Marshal(map[string]any{"prospect_id": prospect, "account_id": id, "channel": "linkedin", "conversation_urn": urn, "external_id": key, "messages": []map[string]any{{"direction": "inbound", "text": m.Text, "created_at": when, "message_id": m.ID}}})
			tag, e := tx.Exec(ctx, `INSERT INTO records(workspace_id,kind,data) SELECT $1,'conversations',$2 WHERE NOT EXISTS(SELECT 1 FROM records WHERE workspace_id=$1 AND kind='conversations' AND data->>'external_id'=$3)`, ws, b, key)
			if e == nil {
				_, e = tx.Exec(ctx, `UPDATE records SET data=data || jsonb_build_object('status','replied','replied_at',$2::text,'conversation_urn',$3::text) WHERE id=$1`, prospect, when.Format(time.RFC3339), urn)
			}
			if e == nil {
				_, e = tx.Exec(ctx, `UPDATE enrollments SET status='stopped' WHERE prospect_id=$1 AND status='active'`, prospect)
			}
			if e == nil {
				_, e = tx.Exec(ctx, `UPDATE jobs SET status='cancelled' WHERE prospect_id=$1 AND status IN ('pending','blocked','awaiting_approval')`, prospect)
			}
			if e != nil {
				tx.Rollback(ctx)
				return n, e
			}
			if e = tx.Commit(ctx); e != nil {
				return n, e
			}
			n += int(tag.RowsAffected())
		}
	}
	return n, nil
}
