package platform

import (
	"encoding/json"
	"github.com/tullips/tullips/backend/internal/core"
	"github.com/tullips/tullips/backend/internal/integrations"
	"golang.org/x/oauth2"
	"net/http"
	"os"
	"strings"
	"time"
)

func (s *Server) mailConfig(provider string) (*oauth2.Config, error) {
	prefix := strings.ToUpper(provider)
	return integrations.MailOAuth(provider, os.Getenv(prefix+"_CLIENT_ID"), os.Getenv(prefix+"_CLIENT_SECRET"), s.Base+"/oauth/mail/callback")
}
func (s *Server) mailAuthorize(w http.ResponseWriter, r *http.Request) {
	user, restricted, _, e := s.Core.Authenticate(r)
	ws, id := r.PathValue("workspace"), r.PathValue("account")
	if e != nil || restricted != "" || !s.Core.HasAccess(r.Context(), user, ws, false) {
		fail(w, 403, "Workspace session required")
		return
	}
	var in struct {
		Provider string `json:"provider"`
	}
	if !decode(w, r, &in) {
		return
	}
	config, e := s.mailConfig(in.Provider)
	if e != nil {
		fail(w, 503, e.Error())
		return
	}
	var channel string
	if s.Core.DB.QueryRow(r.Context(), "SELECT data->>'channel' FROM records WHERE workspace_id=$1 AND id::text=$2 AND kind='accounts'", ws, id).Scan(&channel) != nil || channel != "email" {
		fail(w, 400, "Select an email sender account")
		return
	}
	state, verifier := core.NewToken(), oauth2.GenerateVerifier()
	encrypted, e := core.Encrypt(s.Core.EncryptionKey, verifier)
	if e != nil {
		fail(w, 500, "Credential encryption unavailable")
		return
	}
	_, e = s.Core.DB.Exec(r.Context(), "INSERT INTO mail_oauth_states(state_hash,user_id,workspace_id,account_id,provider,verifier,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)", core.HashToken(state), user, ws, id, in.Provider, encrypted, time.Now().Add(10*time.Minute))
	if e != nil {
		fail(w, 500, "Could not start authorization")
		return
	}
	write(w, 200, map[string]string{"url": config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("prompt", "consent"))})
}
func (s *Server) mailCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		fail(w, 400, "Missing OAuth state")
		return
	}
	var user, ws, id, provider, encrypted string
	e := s.Core.DB.QueryRow(r.Context(), "DELETE FROM mail_oauth_states WHERE state_hash=$1 AND expires_at>now() RETURNING user_id,workspace_id::text,account_id::text,provider,verifier", core.HashToken(state)).Scan(&user, &ws, &id, &provider, &encrypted)
	if e != nil || !s.Core.HasAccess(r.Context(), user, ws, false) {
		fail(w, 400, "Invalid or expired authorization")
		return
	}
	if r.URL.Query().Get("error") != "" {
		http.Redirect(w, r, s.Base+"/app?tab=settings&mail=cancelled", http.StatusSeeOther)
		return
	}
	verifier, e := core.Decrypt(s.Core.EncryptionKey, encrypted)
	if e != nil {
		fail(w, 500, "Could not read authorization")
		return
	}
	config, e := s.mailConfig(provider)
	if e != nil {
		fail(w, 503, "Provider not configured")
		return
	}
	token, e := config.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(verifier))
	if e != nil {
		fail(w, 400, "Mailbox authorization failed")
		return
	}
	var raw []byte
	if s.Core.DB.QueryRow(r.Context(), "SELECT data FROM records WHERE id=$1 AND workspace_id=$2 AND kind='accounts'", id, ws).Scan(&raw) != nil {
		fail(w, 404, "Sender no longer exists")
		return
	}
	var data map[string]any
	if json.Unmarshal(raw, &data) != nil {
		fail(w, 500, "Invalid sender record")
		return
	}
	email, _ := data["email"].(string)
	mailbox := integrations.ProviderMailbox(provider, email)
	mailbox.AccessToken = token.AccessToken
	if e = mailbox.Check(r.Context()); e != nil {
		fail(w, 400, "Mailbox authentication failed. Check the selected email address and SMTP/IMAP access in your provider settings.")
		return
	}
	secret := map[string]any{}
	if stored, ok := data["secrets"].(string); ok && stored != "" {
		if plaintext, err := core.Decrypt(s.Core.EncryptionKey, stored); err == nil {
			json.Unmarshal([]byte(plaintext), &secret)
		}
	}
	secret["access_token"] = token.AccessToken
	secret["refresh_token"] = token.RefreshToken
	secret["token_expiry"] = token.Expiry.Format(time.RFC3339)
	secret["oauth_provider"] = provider
	b, _ := json.Marshal(secret)
	cipher, e := core.Encrypt(s.Core.EncryptionKey, string(b))
	if e != nil {
		fail(w, 500, "Could not encrypt credentials")
		return
	}
	patch, _ := json.Marshal(map[string]any{"provider": provider, "status": "connected", "secrets": cipher, "smtp_host": mailbox.SMTPHost, "smtp_port": mailbox.SMTPPort, "imap_host": mailbox.IMAPHost, "imap_port": mailbox.IMAPPort})
	_, e = s.Core.DB.Exec(r.Context(), "UPDATE records SET data=data || $3::jsonb,updated_at=now() WHERE id=$1 AND workspace_id=$2", id, ws, patch)
	if e != nil {
		fail(w, 500, "Could not save mailbox")
		return
	}
	http.Redirect(w, r, s.Base+"/app?tab=settings&mail=connected", http.StatusSeeOther)
}
