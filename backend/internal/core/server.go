package core

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tullips/tullips/backend/internal/automation"
)

type Server struct {
	DB                            *pgxpool.Pool
	InternalSecret, EncryptionKey string
}

func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.serve) }
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}
func decode(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var d map[string]any
	err := json.NewDecoder(r.Body).Decode(&d)
	if err == nil && d == nil {
		err = errors.New("object required")
	}
	return d, err
}
func (s *Server) Authenticate(r *http.Request) (user, workspace string, scopes []string, err error) {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		err = s.DB.QueryRow(r.Context(), `SELECT k.user_id,k.workspace_id,k.scopes FROM api_keys k JOIN memberships m ON m.workspace_id=k.workspace_id AND m.user_id=k.user_id WHERE token_hash=$1`, HashToken(token)).Scan(&user, &workspace, &scopes)
		return
	}
	scopes = []string{"read", "write", "launch", "send"}
	user = r.Header.Get("X-Tullips-User")
	if !validSignature(s.InternalSecret, user, r.Header.Get("X-Tullips-Signature")) {
		err = errors.New("unauthorized")
	}
	return
}
func (s *Server) HasAccess(ctx context.Context, user, workspace string, owner bool) bool {
	var role string
	err := s.DB.QueryRow(ctx, `SELECT role FROM memberships WHERE workspace_id=$1 AND user_id=$2`, workspace, user).Scan(&role)
	return err == nil && (!owner || role == "owner")
}
func has(scopes []string, scope string) bool {
	if scopes == nil {
		return false
	}
	for _, v := range scopes {
		if v == scope {
			return true
		}
	}
	return false
}
func str(d map[string]any, k string) string { v, _ := d[k].(string); return v }
func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		if err := s.DB.Ping(r.Context()); err != nil {
			fail(w, 503, "database unavailable")
			return
		}
		JSON(w, 200, map[string]string{"status": "ok"})
		return
	}
	user, keyWS, scopes, err := s.Authenticate(r)
	if err != nil {
		fail(w, 401, "authentication required")
		return
	}
	p := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(p) < 2 || p[0] != "api" || p[1] != "workspaces" {
		fail(w, 404, "not found")
		return
	}
	if r.Method == "GET" && !has(scopes, "read") {
		fail(w, 403, "read permission required")
		return
	}
	requiredScope := "write"
	if len(p) == 6 && p[5] == "launch" {
		requiredScope = "launch"
	}
	if len(p) == 6 && (p[5] == "reply" || p[5] == "approve" || p[5] == "approve_sample") {
		requiredScope = "send"
	}
	if r.Method != "GET" && !has(scopes, requiredScope) {
		fail(w, 403, requiredScope+" permission required")
		return
	}
	if len(p) == 2 {
		s.workspaces(w, r, user, keyWS)
		return
	}
	ws := p[2]
	if keyWS != "" && keyWS != ws || !s.HasAccess(r.Context(), user, ws, false) {
		fail(w, 403, "workspace access denied")
		return
	}
	if len(p) == 3 {
		if r.Method == "DELETE" && s.HasAccess(r.Context(), user, ws, true) && keyWS == "" {
			_, err = s.DB.Exec(r.Context(), `DELETE FROM workspaces WHERE id=$1`, ws)
			s.result(w, err, map[string]bool{"deleted": true})
			return
		}
		fail(w, 405, "method not allowed")
		return
	}
	kind := p[3]
	if kind == "actions" && len(p) == 6 && p[5] == "approve" {
		s.approve(w, r, ws, p[4], scopes)
		return
	}
	if kind == "actions" {
		s.actions(w, r, ws)
		return
	}
	if kind == "keys" || kind == "members" {
		if keyWS != "" {
			fail(w, 403, "use a user session to manage access")
			return
		}
		s.access(w, r, user, ws, kind, p)
		return
	}
	if kind == "settings" {
		s.settings(w, r, user, ws)
		return
	}
	if kind == "dashboard" {
		s.dashboard(w, r, ws)
		return
	}
	if kind == "events" {
		s.events(w, r, ws)
		return
	}
	if kind == "inbox" {
		kind = "conversations"
	}
	if kind != "campaigns" && kind != "prospects" && kind != "accounts" && kind != "conversations" {
		fail(w, 404, "not found")
		return
	}
	if kind == "prospects" && len(p) == 5 && (p[4] == "export" || p[4] == "import") {
		s.csv(w, r, ws, p[4])
		return
	}
	if len(p) == 6 && kind == "accounts" && (p[5] == "connect" || p[5] == "verify" || p[5] == "sync") {
		s.connectAccount(w, r, ws, p[4], p[5])
		return
	}
	if len(p) == 6 {
		s.sequenceAction(w, r, user, ws, kind, p[4], p[5], scopes)
		return
	}
	if len(p) > 5 {
		fail(w, 404, "not found")
		return
	}
	id := ""
	if len(p) == 5 {
		id = p[4]
	}
	switch r.Method {
	case "GET":
		s.list(w, r, ws, kind, id)
	case "POST", "PATCH":
		if r.Method == "POST" && id != "" || r.Method == "PATCH" && id == "" {
			fail(w, 405, "method not allowed")
			return
		}
		d, e := decode(w, r)
		if e != nil {
			fail(w, 400, "invalid JSON object")
			return
		}
		if kind == "campaigns" && str(d, "status") == "active" {
			fail(w, 400, "use the campaign launch endpoint to activate and enroll prospects")
			return
		}
		if kind == "conversations" {
			if _, ok := d["messages"]; ok {
				fail(w, 400, "messages are append-only; use the send endpoint")
				return
			}
		}
		if e = s.validate(r.Context(), ws, kind, d); e != nil {
			fail(w, 400, e.Error())
			return
		}
		delete(d, "review_approved")
		if kind == "campaigns" && id != "" && !has(scopes, "launch") {
			var state string
			s.DB.QueryRow(r.Context(), `SELECT data->>'status' FROM records WHERE id::text=$1 AND workspace_id=$2`, id, ws).Scan(&state)
			if state == "active" {
				for _, key := range []string{"workflow", "autonomy", "account_ids", "criteria", "search_mode", "target_count"} {
					if _, ok := d[key]; ok {
						fail(w, 403, "launch scope required to change an active campaign")
						return
					}
				}
			}
		}
		applyExisting, _ := d["apply_existing"].(bool)
		delete(d, "apply_existing")
		delete(d, "id")
		delete(d, "workspace_id")
		delete(d, "created_at")
		delete(d, "updated_at")
		delete(d, "secrets")
		if kind == "accounts" {
			secret := map[string]any{}
			if id != "" {
				_, existing, loadErr := s.accountData(r.Context(), ws, id)
				if loadErr != nil {
					s.result(w, loadErr, nil)
					return
				}
				secret = existing
				if channel := str(d, "channel"); channel != "" {
					account, _, _ := s.accountData(r.Context(), ws, id)
					if channel != str(account, "channel") {
						fail(w, 400, "account channel cannot be changed")
						return
					}
				}
			}
			for _, k := range []string{"password", "cookies", "session", "checkpoint", "access_token", "refresh_token"} {
				if v, ok := d[k]; ok {
					secret[k] = v
					delete(d, k)
				}
			}
			if len(secret) > 0 {
				b, _ := json.Marshal(secret)
				sealed, e := Encrypt(s.EncryptionKey, string(b))
				if e != nil {
					fail(w, 503, "encryption is not configured")
					return
				}
				d["secrets"] = sealed
			}
		}
		b, _ := json.Marshal(d)
		var raw []byte
		tx, e := s.DB.Begin(r.Context())
		if e != nil {
			s.result(w, e, nil)
			return
		}
		defer tx.Rollback(r.Context())
		if id == "" && kind == "accounts" && str(d, "channel") == "linkedin" && os.Getenv("DEPLOYMENT_MODE") == "saas" {
			var limit, count int
			if e = tx.QueryRow(r.Context(), `SELECT account_limit FROM billing_accounts WHERE workspace_id=$1 FOR UPDATE`, ws).Scan(&limit); e != nil {
				fail(w, 402, "activate a subscription before connecting LinkedIn accounts")
				return
			}
			e = tx.QueryRow(r.Context(), `SELECT count(*) FROM records WHERE workspace_id=$1 AND kind='accounts' AND data->>'channel'='linkedin'`, ws).Scan(&count)
			if e != nil {
				s.result(w, e, nil)
				return
			}
			if count >= limit {
				fail(w, 402, "purchase an additional LinkedIn account slot")
				return
			}
		}
		if id == "" {
			e = tx.QueryRow(r.Context(), `INSERT INTO records(workspace_id,kind,data) VALUES($1,$2,$3) RETURNING id`, ws, kind, b).Scan(&id)
		} else {
			var found string
			e = tx.QueryRow(r.Context(), `UPDATE records SET data=data || $4::jsonb,updated_at=now() WHERE id=$1 AND workspace_id=$2 AND kind=$3 RETURNING id`, id, ws, kind, b).Scan(&found)
		}
		if e == nil && kind == "prospects" {
			if workflow, ok := d["workflow"]; ok {
				e = updateProspectWorkflow(r.Context(), tx, id, workflow)
			}
		}
		if e == nil && kind == "campaigns" {
			if workflow, ok := d["workflow"]; ok {
				v, _ := json.Marshal(workflow)
				_, e = tx.Exec(r.Context(), `INSERT INTO workflow_versions(campaign_id,workflow) VALUES($1,$2)`, id, v)
				if e == nil && applyExisting {
					var rows pgx.Rows
					rows, e = tx.Query(r.Context(), `SELECT prospect_id FROM enrollments WHERE campaign_id=$1 AND status='active'`, id)
					if e == nil {
						var prospects []string
						for rows.Next() {
							var prospect string
							if e = rows.Scan(&prospect); e != nil {
								break
							}
							prospects = append(prospects, prospect)
						}
						rows.Close()
						for _, prospect := range prospects {
							if e = updateProspectWorkflow(r.Context(), tx, prospect, workflow); e != nil {
								break
							}
						}
					}
				}
			}
		}
		if e == nil {
			_, e = tx.Exec(r.Context(), `INSERT INTO events(workspace_id,actor,action,record_id) VALUES($1,$2,$3,$4)`, ws, user, kind+"."+strings.ToLower(r.Method), id)
		}
		if e == nil {
			e = tx.QueryRow(r.Context(), `SELECT (data - 'secrets') || jsonb_build_object('id',id,'workspace_id',workspace_id,'created_at',created_at,'updated_at',updated_at) FROM records WHERE id=$1`, id).Scan(&raw)
		}
		if e == nil {
			e = tx.Commit(r.Context())
		}
		s.result(w, e, json.RawMessage(raw))
	case "DELETE":
		if id == "" {
			fail(w, 405, "record ID required")
			return
		}
		tag, e := s.DB.Exec(r.Context(), `DELETE FROM records WHERE id=$1 AND workspace_id=$2 AND kind=$3`, id, ws, kind)
		if e == nil && tag.RowsAffected() == 0 {
			e = pgx.ErrNoRows
		}
		s.result(w, e, map[string]bool{"deleted": true})
	default:
		fail(w, 405, "method not allowed")
	}
}
func (s *Server) result(w http.ResponseWriter, err error, v any) {
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "not found")
		return
	}
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			fail(w, 409, "record already exists")
		} else {
			fail(w, 500, "database operation failed")
		}
		return
	}
	JSON(w, 200, v)
}
func (s *Server) workspaces(w http.ResponseWriter, r *http.Request, user, keyWS string) {
	if r.Method == "GET" {
		rows, e := s.DB.Query(r.Context(), `SELECT jsonb_build_object('id',w.id,'name',w.name,'role',m.role,'settings',w.settings,'created_at',w.created_at) FROM workspaces w JOIN memberships m ON m.workspace_id=w.id WHERE m.user_id=$1 AND ($2='' OR w.id::text=$2) ORDER BY w.created_at`, user, keyWS)
		s.rows(w, rows, e)
		return
	}
	if r.Method != "POST" || keyWS != "" {
		fail(w, 405, "method not allowed")
		return
	}
	d, e := decode(w, r)
	if e != nil || strings.TrimSpace(str(d, "name")) == "" {
		fail(w, 400, "name is required")
		return
	}
	tx, e := s.DB.Begin(r.Context())
	if e != nil {
		s.result(w, e, nil)
		return
	}
	defer tx.Rollback(r.Context())
	var id string
	e = tx.QueryRow(r.Context(), `INSERT INTO workspaces(name,settings) VALUES($1,$2) RETURNING id`, str(d, "name"), `{"working_days":[1,2,3,4,5],"start_hour":9,"end_hour":18,"ramp_days":[0,14,60],"ramp_limits":[40,60,100]}`).Scan(&id)
	if e == nil {
		_, e = tx.Exec(r.Context(), `INSERT INTO memberships VALUES($1,$2,'owner')`, id, user)
	}
	if e == nil {
		e = tx.Commit(r.Context())
	}
	s.result(w, e, map[string]any{"id": id, "name": str(d, "name"), "role": "owner"})
}
func (s *Server) rows(w http.ResponseWriter, rows pgx.Rows, err error) {
	if err != nil {
		s.result(w, err, nil)
		return
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if e := rows.Scan(&b); e != nil {
			s.result(w, e, nil)
			return
		}
		out = append(out, b)
	}
	s.result(w, rows.Err(), out)
}
func (s *Server) list(w http.ResponseWriter, r *http.Request, ws, kind, id string) {
	q := `SELECT (data - 'secrets') || jsonb_build_object('id',id,'workspace_id',workspace_id,'created_at',created_at,'updated_at',updated_at) FROM records WHERE workspace_id=$1 AND kind=$2 AND ($3='' OR id::text=$3) AND ($4='' OR data::text ILIKE '%' || $4 || '%') ORDER BY created_at DESC LIMIT 1000`
	if id != "" {
		var b []byte
		e := s.DB.QueryRow(r.Context(), q, ws, kind, id, r.URL.Query().Get("q")).Scan(&b)
		s.result(w, e, json.RawMessage(b))
		return
	}
	rows, e := s.DB.Query(r.Context(), q, ws, kind, id, r.URL.Query().Get("q"))
	s.rows(w, rows, e)
}
func (s *Server) validate(ctx context.Context, ws, kind string, d map[string]any) error {
	if flow, ok := d["workflow"]; ok {
		steps, ok := flow.([]any)
		if !ok || len(steps) == 0 || len(steps) > 100 {
			return errors.New("workflow must contain 1 to 100 steps")
		}
		for _, item := range steps {
			step, ok := item.(map[string]any)
			if !ok {
				return errors.New("invalid workflow step")
			}
			switch str(step, "type") {
			case "invitation", "linkedin_message", "email", "delay", "condition", "tag", "stop":
			default:
				return errors.New("unknown workflow step")
			}
			if number(step, "days", 0) < 0 || number(step, "days", 0) > 365 {
				return errors.New("delay must be 0 to 365 business days")
			}
		}
	}
	if kind == "campaigns" {
		if v, ok := d["status"]; ok && v != "draft" && v != "active" && v != "paused" && v != "completed" {
			return errors.New("invalid campaign status")
		}
		if v, ok := d["autonomy"]; ok && v != "manual" && v != "review" && v != "automatic" {
			return errors.New("invalid autonomy")
		}
	}
	if kind == "accounts" {
		if v, ok := d["channel"]; ok && v != "linkedin" && v != "email" {
			return errors.New("invalid account channel")
		}
		if value, ok := d["daily_limit"]; ok && value != nil && number(d, "daily_limit", 0) < 1 {
			return errors.New("daily_limit must be positive")
		}
		if _, ok := d["working_days"]; ok {
			if _, err := automation.NextActive(time.Now(), window(d)); err != nil {
				return err
			}
		}
	}
	if kind == "prospects" {
		if str(d, "status") == "qualified" {
			d["qualification"] = "manual"
			d["qualification_reason"] = "Approved by a workspace member"
		}
		if u := str(d, "linkedin_url"); u != "" {
			d["linkedin_url"] = strings.TrimRight(strings.Split(u, "?")[0], "/")
		}
		if email := str(d, "email"); email != "" {
			d["email"] = strings.ToLower(strings.TrimSpace(email))
		}
	}
	for _, key := range []string{"campaign_id", "prospect_id", "email_account_id"} {
		if id := str(d, key); id != "" {
			var exists bool
			expected := "campaigns"
			if key == "prospect_id" {
				expected = "prospects"
			}
			if key == "email_account_id" {
				expected = "accounts"
			}
			e := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM records WHERE id::text=$1 AND workspace_id=$2 AND kind=$3)`, id, ws, expected).Scan(&exists)
			if e != nil || !exists {
				return fmt.Errorf("invalid %s", key)
			}
		}
	}
	if ids, ok := d["account_ids"].([]any); ok {
		for _, id := range ids {
			var exists bool
			e := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM records WHERE id::text=$1 AND workspace_id=$2 AND kind='accounts')`, id, ws).Scan(&exists)
			if e != nil || !exists {
				return errors.New("invalid account_ids")
			}
		}
	}
	if tz := str(d, "timezone"); tz != "" {
		if _, e := time.LoadLocation(tz); e != nil {
			return errors.New("invalid timezone")
		}
	}
	return nil
}
func (s *Server) settings(w http.ResponseWriter, r *http.Request, user, ws string) {
	if r.Method == "GET" {
		var b []byte
		e := s.DB.QueryRow(r.Context(), `SELECT settings FROM workspaces WHERE id=$1`, ws).Scan(&b)
		s.result(w, e, json.RawMessage(b))
		return
	}
	if r.Method != "PATCH" || !s.HasAccess(r.Context(), user, ws, true) {
		fail(w, 403, "owner permission required")
		return
	}
	d, e := decode(w, r)
	if e != nil {
		fail(w, 400, "invalid JSON")
		return
	}
	if e = validateRamp(d); e != nil {
		fail(w, 400, e.Error())
		return
	}
	b, _ := json.Marshal(d)
	var out []byte
	e = s.DB.QueryRow(r.Context(), `UPDATE workspaces SET settings=settings || $2::jsonb WHERE id=$1 RETURNING settings`, ws, b).Scan(&out)
	s.result(w, e, json.RawMessage(out))
}
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request, ws string) {
	var b []byte
	e := s.DB.QueryRow(r.Context(), `SELECT jsonb_build_object('prospects',count(*) FILTER(WHERE kind='prospects'),'campaigns',count(*) FILTER(WHERE kind='campaigns'),'active_campaigns',count(*) FILTER(WHERE kind='campaigns' AND data->>'status'='active'),'accounts',count(*) FILTER(WHERE kind='accounts'),'conversations',count(*) FILTER(WHERE kind='conversations'),'replies',count(*) FILTER(WHERE kind='prospects' AND data->>'status'='replied')) FROM records WHERE workspace_id=$1`, ws).Scan(&b)
	s.result(w, e, json.RawMessage(b))
}
func (s *Server) events(w http.ResponseWriter, r *http.Request, ws string) {
	rows, e := s.DB.Query(r.Context(), `SELECT to_jsonb(e) FROM events e WHERE workspace_id=$1 ORDER BY created_at DESC LIMIT 100`, ws)
	s.rows(w, rows, e)
}
func (s *Server) csv(w http.ResponseWriter, r *http.Request, ws, action string) {
	if action == "export" && r.Method == "GET" {
		rows, e := s.DB.Query(r.Context(), `SELECT data FROM records WHERE workspace_id=$1 AND kind='prospects' ORDER BY created_at`, ws)
		if e != nil {
			s.result(w, e, nil)
			return
		}
		defer rows.Close()
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="prospects.csv"`)
		out := csv.NewWriter(w)
		defer out.Flush()
		cols := []string{"first_name", "last_name", "title", "company", "linkedin_url", "email", "status", "notes"}
		out.Write(cols)
		for rows.Next() {
			var b []byte
			rows.Scan(&b)
			var d map[string]any
			json.Unmarshal(b, &d)
			v := []string{}
			for _, k := range cols {
				value := str(d, k)
				if strings.HasPrefix(value, "=") || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "@") {
					value = "'" + value
				}
				v = append(v, value)
			}
			out.Write(v)
		}
		return
	}
	if action != "import" || r.Method != "POST" {
		fail(w, 405, "method not allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	reader := csv.NewReader(r.Body)
	cols, e := reader.Read()
	if e != nil {
		fail(w, 400, "invalid CSV")
		return
	}
	tx, e := s.DB.Begin(r.Context())
	if e != nil {
		s.result(w, e, nil)
		return
	}
	defer tx.Rollback(r.Context())
	n := 0
	for {
		row, e := reader.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			fail(w, 400, "invalid CSV row")
			return
		}
		d := map[string]any{}
		for i, k := range cols {
			switch k {
			case "first_name", "last_name", "title", "company", "linkedin_url", "email", "status", "notes":
				d[k] = row[i]
			}
		}
		if e = s.validate(r.Context(), ws, "prospects", d); e != nil {
			fail(w, 400, e.Error())
			return
		}
		b, _ := json.Marshal(d)
		tag, e := tx.Exec(r.Context(), `INSERT INTO records(workspace_id,kind,data) VALUES($1,'prospects',$2) ON CONFLICT DO NOTHING`, ws, b)
		if e != nil {
			s.result(w, e, nil)
			return
		}
		n += int(tag.RowsAffected())
	}
	e = tx.Commit(r.Context())
	s.result(w, e, map[string]int{"imported": n})
}
