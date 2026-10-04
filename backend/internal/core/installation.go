package core

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func (s *Server) Installation(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("DEPLOYMENT_MODE") == "saas" {
		fail(w, 404, "not found")
		return
	}
	user, workspace, _, err := s.Authenticate(r)
	if err != nil {
		fail(w, 401, "authentication required")
		return
	}
	if workspace != "" || r.Header.Get("Authorization") != "" {
		fail(w, 403, "installation administrator session required")
		return
	}
	var admin bool
	if err := s.DB.QueryRow(r.Context(), `SELECT coalesce(admin_user_id=$1,false) FROM installation WHERE id`, user).Scan(&admin); err != nil {
		fail(w, 503, "installation unavailable")
		return
	}
	if r.URL.Path == "/api/installation" && r.Method == "GET" {
		JSON(w, 200, map[string]any{"administrator": admin, "version": os.Getenv("RELEASE_VERSION")})
		return
	}
	if !admin {
		fail(w, 403, "installation administrator required")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/installation/updates")
	allowed := r.Method == "GET" && path == "" || r.Method == "POST" && (path == "/check" || path == "/apply" || path == "/restore" || path == "/recover") || r.Method == "PATCH" && path == "/settings"
	if !allowed {
		fail(w, 404, "not found")
		return
	}
	endpoint, token := os.Getenv("UPDATER_URL"), os.Getenv("UPDATER_TOKEN")
	if endpoint == "" || len(token) < 32 {
		fail(w, 503, "updater is not configured; run the installation migration command")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		fail(w, 413, "request body too large")
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), r.Method, strings.TrimRight(endpoint, "/")+"/updates"+path, bytes.NewReader(body))
	if err != nil {
		fail(w, 503, "updater unavailable")
		return
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Tullips-Actor", user)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		fail(w, 503, "updater unavailable; use the server CLI for recovery")
		return
	}
	defer response.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(response.StatusCode)
	io.Copy(w, io.LimitReader(response.Body, 1<<20))
}
