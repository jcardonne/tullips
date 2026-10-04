package core

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *Server) access(w http.ResponseWriter, r *http.Request, user, ws, kind string, p []string) {
	id := ""
	if len(p) == 5 {
		id = p[4]
	}
	if kind == "members" {
		if !s.HasAccess(r.Context(), user, ws, true) {
			fail(w, 403, "owner permission required")
			return
		}
		switch r.Method {
		case "GET":
			rows, e := s.DB.Query(r.Context(), `SELECT jsonb_build_object('user_id',user_id,'role',role) FROM memberships WHERE workspace_id=$1`, ws)
			s.rows(w, rows, e)
		case "POST":
			d, e := decode(w, r)
			if e != nil || strings.TrimSpace(str(d, "user_id")) == "" {
				fail(w, 400, "user_id required")
				return
			}
			_, e = s.DB.Exec(r.Context(), `INSERT INTO memberships(workspace_id,user_id,role) VALUES($1,$2,'member') ON CONFLICT DO NOTHING`, ws, str(d, "user_id"))
			s.result(w, e, map[string]bool{"added": true})
		case "DELETE":
			if id == "" {
				fail(w, 400, "user ID required")
				return
			}
			_, e := s.DB.Exec(r.Context(), `DELETE FROM memberships WHERE workspace_id=$1 AND user_id=$2 AND role='member'`, ws, id)
			s.result(w, e, map[string]bool{"removed": true})
		default:
			fail(w, 405, "method not allowed")
		}
		return
	}
	switch r.Method {
	case "GET":
		rows, e := s.DB.Query(r.Context(), `SELECT jsonb_build_object('id',id,'name',name,'scopes',scopes,'created_at',created_at,'last_used_at',last_used_at) FROM api_keys WHERE workspace_id=$1 AND user_id=$2`, ws, user)
		s.rows(w, rows, e)
	case "POST":
		d, e := decode(w, r)
		if e != nil || str(d, "name") == "" {
			fail(w, 400, "name required")
			return
		}
		scopes := []string{"read"}
		if v, ok := d["scopes"]; ok {
			b, _ := json.Marshal(v)
			if json.Unmarshal(b, &scopes) != nil {
				fail(w, 400, "invalid scopes")
				return
			}
		}
		for _, v := range scopes {
			if v != "read" && v != "write" && v != "launch" && v != "send" {
				fail(w, 400, "invalid scope")
				return
			}
		}
		token := NewToken()
		var keyID string
		e = s.DB.QueryRow(r.Context(), `INSERT INTO api_keys(workspace_id,user_id,name,token_hash,scopes) VALUES($1,$2,$3,$4,$5) RETURNING id`, ws, user, str(d, "name"), HashToken(token), scopes).Scan(&keyID)
		s.result(w, e, map[string]any{"id": keyID, "name": str(d, "name"), "scopes": scopes, "token": token})
	case "DELETE":
		_, e := s.DB.Exec(r.Context(), `DELETE FROM api_keys WHERE id::text=$1 AND workspace_id=$2 AND user_id=$3`, id, ws, user)
		s.result(w, e, map[string]bool{"revoked": true})
	default:
		fail(w, 405, "method not allowed")
	}
}
