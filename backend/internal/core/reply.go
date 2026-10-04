package core

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/tullips/tullips/backend/internal/automation"
	"github.com/tullips/tullips/backend/internal/integrations"
	"os"
	"time"
)

func (s *Server) ReplyAction(ctx context.Context, ws, job string, payload map[string]any) (map[string]any, error) {
	if !s.PaidActive(ctx, ws) {
		return blocked("Active subscription required"), nil
	}
	var raw []byte
	e := s.DB.QueryRow(ctx, `SELECT data FROM records WHERE id::text=$1 AND workspace_id=$2 AND kind='conversations'`, str(payload, "conversation_id"), ws).Scan(&raw)
	if e != nil {
		return nil, e
	}
	var conv map[string]any
	json.Unmarshal(raw, &conv)
	id := str(conv, "account_id")
	data, secret, e := s.accountData(ctx, ws, id)
	if e != nil {
		return nil, e
	}
	if os.Getenv("LIVE_SENDS") != "true" || str(data, "status") != "connected" {
		return later("blocked", time.Now()), nil
	}
	due, e := automation.NextActive(time.Now(), window(data))
	if e != nil {
		return nil, e
	}
	if due.After(time.Now().Add(time.Second)) {
		return later("pending", due), nil
	}
	var activated time.Time
	s.DB.QueryRow(ctx, `SELECT created_at FROM records WHERE id=$1`, id).Scan(&activated)
	limit, limitErr := s.accountLimit(ctx, ws, data, activated)
	if limitErr != nil {
		return nil, limitErr
	}
	var used int
	e = s.DB.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE account_id=$1 AND status='completed' AND kind IN ('workflow.step','message.send') AND (updated_at AT TIME ZONE $2)::date=(now() AT TIME ZONE $2)::date`, id, window(data).Timezone).Scan(&used)
	if e != nil {
		return nil, e
	}
	if used >= limit {
		return later("pending", time.Now().AddDate(0, 0, 1)), nil
	}
	text := str(payload, "text")
	if text == "" {
		return nil, errors.New("empty message")
	}
	if str(conv, "channel") == "email" {
		var email string
		e = s.DB.QueryRow(ctx, `SELECT data->>'email' FROM records WHERE id::text=$1 AND workspace_id=$2 AND kind='prospects'`, str(conv, "prospect_id"), ws).Scan(&email)
		if e != nil {
			return nil, e
		}
		m, e := s.mailbox(ctx, id, data, secret)
		if e != nil {
			return nil, e
		}
		e = m.Send(ctx, integrations.Email{From: m.Email, To: email, Subject: "Re: " + str(conv, "subject"), Text: text, MessageID: job + "@tullips.local", InReplyTo: str(conv, "message_id")})
		if e != nil {
			return nil, e
		}
	} else {
		client, e := integrations.NewLinkedIn()
		if e != nil {
			return nil, e
		}
		if e = client.Restore([]byte(str(secret, "session"))); e != nil {
			return nil, e
		}
		_, e = client.SendMessage(ctx, str(conv, "conversation_urn"), str(data, "mailbox_urn"), text, job, job)
		if e != nil {
			return nil, e
		}
	}
	message, _ := json.Marshal(map[string]any{"direction": "outbound", "text": text, "created_at": time.Now(), "job_id": job})
	_, e = s.DB.Exec(ctx, `UPDATE records SET data=jsonb_set(data,'{messages}',coalesce(data->'messages','[]'::jsonb)||jsonb_build_array($2::jsonb)),updated_at=now() WHERE id::text=$1`, str(payload, "conversation_id"), message)
	if e != nil {
		return nil, e
	}
	return map[string]any{"sent": true}, nil
}
