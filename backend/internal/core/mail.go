package core

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tullips/tullips/backend/internal/integrations"
	"golang.org/x/oauth2"
	"os"
	"strings"
	"time"
)

func (s *Server) mailbox(ctx context.Context, id string, data, secret map[string]any) (integrations.Mailbox, error) {
	m := integrations.Mailbox{Email: str(data, "email"), Username: str(data, "username"), Password: str(secret, "password"), AccessToken: str(secret, "access_token"), SMTPHost: str(data, "smtp_host"), SMTPPort: number(data, "smtp_port", 587), IMAPHost: str(data, "imap_host"), IMAPPort: number(data, "imap_port", 993)}
	if m.Username == "" {
		m.Username = m.Email
	}
	provider := str(secret, "oauth_provider")
	if provider != "" {
		prefix := strings.ToUpper(provider)
		cfg, e := integrations.MailOAuth(provider, os.Getenv(prefix+"_CLIENT_ID"), os.Getenv(prefix+"_CLIENT_SECRET"), os.Getenv("APP_URL")+"/oauth/mail/callback")
		if e != nil {
			return m, e
		}
		expiry, _ := time.Parse(time.RFC3339, str(secret, "token_expiry"))
		token, e := cfg.TokenSource(ctx, &oauth2.Token{AccessToken: m.AccessToken, RefreshToken: str(secret, "refresh_token"), Expiry: expiry}).Token()
		if e != nil {
			return m, e
		}
		if token.AccessToken != m.AccessToken {
			secret["access_token"] = token.AccessToken
			secret["refresh_token"] = token.RefreshToken
			secret["token_expiry"] = token.Expiry.Format(time.RFC3339)
			if e = s.saveAccount(ctx, id, data, secret); e != nil {
				return m, e
			}
		}
		m.AccessToken = token.AccessToken
	}
	return m, nil
}
func (s *Server) SyncMail(ctx context.Context, ws, id string) (int, error) {
	data, secret, e := s.accountData(ctx, ws, id)
	if e != nil {
		return 0, e
	}
	m, e := s.mailbox(ctx, id, data, secret)
	if e != nil {
		return 0, e
	}
	batch, e := m.Inbox(ctx, uint32(number(data, "last_uid", 0)))
	if e != nil {
		return 0, e
	}
	resetUID := false
	if previous := number(data, "uid_validity", 0); previous != 0 && uint32(previous) != batch.UIDValidity {
		resetUID = true
		batch, e = m.Inbox(ctx, 0)
		if e != nil {
			return 0, e
		}
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback(ctx)
	n := 0
	last := number(data, "last_uid", 0)
	if resetUID {
		last = 0
	}
	for _, msg := range batch.Messages {
		if int(msg.UID) > last {
			last = int(msg.UID)
		}
		var prospect string
		e = tx.QueryRow(ctx, `SELECT id FROM records WHERE workspace_id=$1 AND kind='prospects' AND lower(data->>'email')=lower($2)`, ws, msg.From).Scan(&prospect)
		if e != nil {
			continue
		}
		key := id + ":" + msg.MessageID
		if msg.MessageID == "" {
			key = fmt.Sprintf("%s:%d:%d", id, batch.UIDValidity, msg.UID)
		}
		b, _ := json.Marshal(map[string]any{"prospect_id": prospect, "account_id": id, "channel": "email", "subject": msg.Subject, "external_id": key, "messages": []map[string]any{{"direction": "inbound", "text": msg.Text, "created_at": msg.Date, "message_id": msg.MessageID}}})
		_, e = tx.Exec(ctx, `INSERT INTO records(workspace_id,kind,data) SELECT $1,'conversations',$2 WHERE NOT EXISTS(SELECT 1 FROM records WHERE workspace_id=$1 AND kind='conversations' AND data->>'external_id'=$3)`, ws, b, key)
		if e != nil {
			return n, e
		}
		_, e = tx.Exec(ctx, `UPDATE records SET data=data || jsonb_build_object('status','replied','replied_at',$2::text) WHERE id=$1`, prospect, msg.Date.Format(time.RFC3339))
		if e != nil {
			return n, e
		}
		_, e = tx.Exec(ctx, `UPDATE enrollments SET status='stopped' WHERE prospect_id=$1 AND status='active'`, prospect)
		if e != nil {
			return n, e
		}
		_, e = tx.Exec(ctx, `UPDATE jobs SET status='cancelled' WHERE prospect_id=$1 AND status IN ('pending','blocked','awaiting_approval')`, prospect)
		if e != nil {
			return n, e
		}
		n++
	}
	if e = tx.Commit(ctx); e != nil {
		return n, e
	}
	data["last_uid"] = last
	data["uid_validity"] = batch.UIDValidity
	return n, s.saveAccount(ctx, id, data, secret)
}
