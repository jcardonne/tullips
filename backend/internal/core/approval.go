package core

import (
	"net/http"
	"os"
)

func (s *Server) approve(w http.ResponseWriter, r *http.Request, ws, id string, scopes []string) {
	if r.Method != "POST" {
		fail(w, 405, "method not allowed")
		return
	}
	if !has(scopes, "send") {
		fail(w, 403, "send permission required")
		return
	}
	state := "blocked"
	if os.Getenv("LIVE_SENDS") == "true" {
		state = "pending"
	}
	tag, e := s.DB.Exec(r.Context(), `UPDATE jobs SET status=$3,updated_at=now() WHERE id::text=$1 AND workspace_id=$2 AND status IN ('awaiting_approval','blocked')`, id, ws, state)
	if e != nil {
		s.result(w, e, nil)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 404, "no job awaiting approval")
		return
	}
	JSON(w, 200, map[string]string{"id": id, "status": state})
}
