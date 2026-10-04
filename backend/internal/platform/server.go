package platform

import (
	"encoding/json"
	"github.com/tullips/tullips/backend/internal/core"
	"net/http"
	"strings"
)

type Server struct {
	Core *core.Server
	Base string
	API  http.Handler
}

func New(c *core.Server, base string) (*Server, error) {
	s := &Server{Core: c, Base: strings.TrimRight(base, "/"), API: c.Handler()}
	return s, nil
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-protected-resource", s.resourceMetadata)
	mux.HandleFunc("/.well-known/oauth-authorization-server", s.authMetadata)
	mux.HandleFunc("POST /oauth/register", s.register)
	mux.HandleFunc("POST /oauth/token", s.token)
	mux.HandleFunc("POST /oauth/revoke", s.revoke)
	mux.HandleFunc("POST /api/oauth/authorize", s.authorize)
	mux.HandleFunc("GET /api/oauth/grants", s.grants)
	mux.HandleFunc("DELETE /api/oauth/grants/{client}", s.grants)
	mux.HandleFunc("/mcp", s.mcp)
	mux.HandleFunc("POST /api/workspaces/{workspace}/accounts/{account}/oauth", s.mailAuthorize)
	mux.HandleFunc("GET /oauth/mail/callback", s.mailCallback)
	mux.HandleFunc("POST /api/stripe/webhook", s.webhook)
	mux.HandleFunc("POST /api/workspaces/{workspace}/billing/{action}", s.billing)
	mux.HandleFunc("GET /api/workspaces/{workspace}/billing", s.billingStatus)
	mux.Handle("/", s.API)
	mux.HandleFunc("/api/installation", s.Core.Installation)
	mux.HandleFunc("/api/installation/", s.Core.Installation)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" && !strings.HasPrefix(r.URL.Path, "/api/installation") && s.Core.Maintenance(r.Context()) {
			w.Header().Set("Retry-After", "30")
			fail(w, 503, "The installation is being updated. Please try again shortly.")
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, message string) {
	write(w, status, map[string]string{"error": message})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if json.NewDecoder(r.Body).Decode(v) != nil {
		fail(w, 400, "Invalid request body")
		return false
	}
	return true
}
func has(scopes []string, scope string) bool {
	for _, v := range scopes {
		if v == scope {
			return true
		}
	}
	return false
}
