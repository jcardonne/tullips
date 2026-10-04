package platform

import (
	"crypto/sha256"
	"encoding/base64"
	"github.com/tullips/tullips/backend/internal/core"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Server) resourceMetadata(w http.ResponseWriter, r *http.Request) {
	write(w, 200, map[string]any{"resource": s.Base + "/mcp", "authorization_servers": []string{s.Base}, "scopes_supported": []string{"read", "write", "launch", "send"}, "bearer_methods_supported": []string{"header"}})
}
func (s *Server) authMetadata(w http.ResponseWriter, r *http.Request) {
	write(w, 200, map[string]any{"issuer": s.Base, "authorization_endpoint": s.Base + "/oauth/consent", "token_endpoint": s.Base + "/oauth/token", "registration_endpoint": s.Base + "/oauth/register", "revocation_endpoint": s.Base + "/oauth/revoke", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code"}, "code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"}, "scopes_supported": []string{"read", "write", "launch", "send"}})
}
func validRedirect(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Host != "" && u.User == nil && u.Fragment == "" && (u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1")))
}
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string   `json:"client_name"`
		Redirects []string `json:"redirect_uris"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Redirects) == 0 || len(in.Redirects) > 10 || len(in.Name) > 200 {
		fail(w, 400, "invalid_client_metadata")
		return
	}
	for _, v := range in.Redirects {
		if !validRedirect(v) {
			fail(w, 400, "invalid_redirect_uri")
			return
		}
	}
	id := core.NewToken()
	if in.Name == "" {
		in.Name = "AI agent"
	}
	_, e := s.Core.DB.Exec(r.Context(), "INSERT INTO oauth_clients(client_id,name,redirect_uris) VALUES($1,$2,$3)", id, in.Name, in.Redirects)
	if e != nil {
		fail(w, 500, "Registration failed")
		return
	}
	write(w, 201, map[string]any{"client_id": id, "client_name": in.Name, "redirect_uris": in.Redirects, "token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code"}, "response_types": []string{"code"}})
}
func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	user, restricted, _, e := s.Core.Authenticate(r)
	if e != nil || restricted != "" {
		fail(w, 401, "Sign in to authorize an agent")
		return
	}
	var in struct {
		Client    string `json:"client_id"`
		Redirect  string `json:"redirect_uri"`
		Challenge string `json:"code_challenge"`
		Method    string `json:"code_challenge_method"`
		Resource  string `json:"resource"`
		Scope     string `json:"scope"`
		State     string `json:"state"`
		Workspace string `json:"workspace_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !s.Core.HasAccess(r.Context(), user, in.Workspace, false) {
		fail(w, 403, "Workspace access denied")
		return
	}
	var redirects []string
	if s.Core.DB.QueryRow(r.Context(), "SELECT redirect_uris FROM oauth_clients WHERE client_id=$1", in.Client).Scan(&redirects) != nil || !has(redirects, in.Redirect) {
		fail(w, 400, "Invalid client or redirect URI")
		return
	}
	challenge, err := base64.RawURLEncoding.DecodeString(in.Challenge)
	if err != nil || len(challenge) != 32 || in.Method != "S256" || in.Resource != s.Base+"/mcp" {
		fail(w, 400, "S256 PKCE and matching MCP resource are required")
		return
	}
	scopes := strings.Fields(in.Scope)
	if len(scopes) == 0 {
		scopes = []string{"read"}
	}
	for _, v := range scopes {
		if !has([]string{"read", "write", "launch", "send"}, v) {
			fail(w, 400, "invalid_scope")
			return
		}
	}
	code := core.NewToken()
	_, err = s.Core.DB.Exec(r.Context(), "INSERT INTO oauth_codes(code_hash,client_id,user_id,workspace_id,redirect_uri,challenge,scopes,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", core.HashToken(code), in.Client, user, in.Workspace, in.Redirect, in.Challenge, scopes, time.Now().Add(5*time.Minute))
	if err != nil {
		fail(w, 500, "Authorization failed")
		return
	}
	u, _ := url.Parse(in.Redirect)
	q := u.Query()
	q.Set("code", code)
	q.Set("state", in.State)
	u.RawQuery = q.Encode()
	write(w, 200, map[string]string{"redirect_uri": u.String()})
}
func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	if r.ParseForm() != nil || r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("resource") != s.Base+"/mcp" {
		fail(w, 400, "invalid_request")
		return
	}
	verifier := r.Form.Get("code_verifier")
	if len(verifier) < 43 || len(verifier) > 128 {
		fail(w, 400, "invalid_grant")
		return
	}
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	tx, e := s.Core.DB.Begin(r.Context())
	if e != nil {
		fail(w, 500, "Token exchange failed")
		return
	}
	defer tx.Rollback(r.Context())
	var user, workspace string
	var scopes []string
	e = tx.QueryRow(r.Context(), "DELETE FROM oauth_codes WHERE code_hash=$1 AND client_id=$2 AND redirect_uri=$3 AND challenge=$4 AND expires_at>now() RETURNING user_id,workspace_id::text,scopes", core.HashToken(r.Form.Get("code")), r.Form.Get("client_id"), r.Form.Get("redirect_uri"), challenge).Scan(&user, &workspace, &scopes)
	if e != nil || !s.Core.HasAccess(r.Context(), user, workspace, false) {
		fail(w, 400, "invalid_grant")
		return
	}
	token := core.NewToken()
	_, e = tx.Exec(r.Context(), "INSERT INTO oauth_tokens(token_hash,client_id,user_id,workspace_id,scopes,expires_at) VALUES($1,$2,$3,$4,$5,$6)", core.HashToken(token), r.Form.Get("client_id"), user, workspace, scopes, time.Now().Add(time.Hour))
	if e != nil || tx.Commit(r.Context()) != nil {
		fail(w, 500, "Token exchange failed")
		return
	}
	write(w, 200, map[string]any{"access_token": token, "token_type": "Bearer", "expires_in": 3600, "scope": strings.Join(scopes, " ")})
}
func (s *Server) revoke(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	if r.ParseForm() != nil {
		fail(w, 400, "invalid_request")
		return
	}
	_, e := s.Core.DB.Exec(r.Context(), "DELETE FROM oauth_tokens WHERE token_hash=$1 AND client_id=$2", core.HashToken(r.Form.Get("token")), r.Form.Get("client_id"))
	if e != nil {
		fail(w, 500, "Revocation failed")
		return
	}
	write(w, 200, map[string]any{})
}

func (s *Server) grants(w http.ResponseWriter, r *http.Request) {
	user, restricted, _, e := s.Core.Authenticate(r)
	if e != nil || restricted != "" {
		fail(w, 401, "User session required")
		return
	}
	if r.Method == "DELETE" {
		_, e = s.Core.DB.Exec(r.Context(), "DELETE FROM oauth_tokens WHERE user_id=$1 AND client_id=$2", user, r.PathValue("client"))
		if e != nil {
			fail(w, 500, "Could not revoke access")
			return
		}
		write(w, 200, map[string]bool{"revoked": true})
		return
	}
	rows, e := s.Core.DB.Query(r.Context(), "SELECT t.client_id,c.name,t.workspace_id::text,t.scopes,t.expires_at FROM oauth_tokens t JOIN oauth_clients c USING(client_id) WHERE t.user_id=$1 AND t.expires_at>now()", user)
	if e != nil {
		fail(w, 500, "Could not list grants")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, workspace string
		var scopes []string
		var expires time.Time
		if rows.Scan(&id, &name, &workspace, &scopes, &expires) != nil {
			fail(w, 500, "Could not read grant")
			return
		}
		out = append(out, map[string]any{"client_id": id, "name": name, "workspace_id": workspace, "scopes": scopes, "expires_at": expires})
	}
	if rows.Err() != nil {
		fail(w, 500, "Could not list grants")
		return
	}
	write(w, 200, out)
}
