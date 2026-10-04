package platform

import (
	"bytes"
	"encoding/json"
	"github.com/tullips/tullips/backend/internal/core"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
)

var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Input       map[string]any `json:"inputSchema"`
	Scope       string         `json:"-"`
	Method      string         `json:"-"`
	Path        string         `json:"-"`
}

func tools() []tool {
	list := []tool{
		{Name: "list_prospects", Description: "List workspace prospects", Scope: "read", Method: "GET", Path: "prospects"},
		{Name: "list_campaigns", Description: "List workspace campaigns", Scope: "read", Method: "GET", Path: "campaigns"},
		{Name: "list_conversations", Description: "Read the unified inbox", Scope: "read", Method: "GET", Path: "conversations"},
		{Name: "get_dashboard", Description: "Get workspace activity and totals", Scope: "read", Method: "GET", Path: "dashboard"},
		{Name: "create_prospect", Description: "Create a prospect. Pass prospect fields in data.", Scope: "write", Method: "POST", Path: "prospects"},
		{Name: "update_prospect", Description: "Update a prospect identified by id. Pass changed fields in data.", Scope: "write", Method: "PATCH", Path: "prospects/{id}"},
		{Name: "create_campaign", Description: "Prepare a draft campaign with workflow steps in data. Launch is separate.", Scope: "write", Method: "POST", Path: "campaigns"},
		{Name: "update_campaign", Description: "Edit a campaign. Cannot launch or resume it; use launch_campaign.", Scope: "write", Method: "PATCH", Path: "campaigns/{id}"},
		{Name: "launch_campaign", Description: "Launch a campaign through the normal approval and queue checks.", Scope: "launch", Method: "POST", Path: "campaigns/{id}/launch"},
		{Name: "send_reply", Description: "Queue a human-approved reply through account quotas and activity windows. Pass message text in data.", Scope: "send", Method: "POST", Path: "conversations/{id}/reply"},
	}
	for i := range list {
		props := map[string]any{"workspace_id": map[string]string{"type": "string"}, "id": map[string]string{"type": "string"}, "data": map[string]any{"type": "object", "additionalProperties": true}}
		required := []string{"workspace_id"}
		if strings.Contains(list[i].Path, "{id}") {
			required = append(required, "id")
		}
		list[i].Input = map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
	}
	return list
}
func (s *Server) mcp(w http.ResponseWriter, r *http.Request) {
	if origin := r.Header.Get("Origin"); origin != "" && origin != s.Base {
		fail(w, 403, "Origin not allowed")
		return
	}
	user, workspace, scopes, err := s.Core.Authenticate(r)
	oauth := false
	if err != nil || workspace == "" {
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		err = s.Core.DB.QueryRow(r.Context(), "SELECT user_id,workspace_id::text,scopes FROM oauth_tokens WHERE token_hash=$1 AND expires_at>now()", core.HashToken(raw)).Scan(&user, &workspace, &scopes)
		oauth = err == nil
	}
	if err != nil || workspace == "" || !s.Core.HasAccess(r.Context(), user, workspace, false) {
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+s.Base+`/.well-known/oauth-protected-resource"`)
		fail(w, 401, "A workspace API key or OAuth token is required")
		return
	}
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		fail(w, 405, "Use POST for stateless MCP requests")
		return
	}
	var req struct {
		Version string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if !decode(w, r, &req) {
		return
	}
	respond := func(result any) { write(w, 200, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result}) }
	rpcError := func(code int, message string) {
		write(w, 200, map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": code, "message": message}})
	}
	if req.Version != "2.0" {
		rpcError(-32600, "Invalid JSON-RPC version")
		return
	}
	if len(req.ID) == 0 {
		w.WriteHeader(202)
		return
	}
	switch req.Method {
	case "initialize":
		respond(map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "tullips", "version": "0.1.0"}, "instructions": "Your authorized workspace_id is " + workspace + ". Use this ID for every tool call. Writes require explicit scopes; outbound actions obey campaign approvals and account scheduling."})
	case "ping":
		respond(map[string]any{})
	case "tools/list":
		allowed := []tool{}
		for _, t := range tools() {
			if has(scopes, t.Scope) {
				allowed = append(allowed, t)
			}
		}
		respond(map[string]any{"tools": allowed})
	case "tools/call":
		var p struct {
			Name string `json:"name"`
			Args struct {
				Workspace string         `json:"workspace_id"`
				ID        string         `json:"id"`
				Data      map[string]any `json:"data"`
			} `json:"arguments"`
		}
		if json.Unmarshal(req.Params, &p) != nil {
			rpcError(-32602, "Invalid tool arguments")
			return
		}
		var chosen *tool
		for _, t := range tools() {
			if t.Name == p.Name {
				v := t
				chosen = &v
				break
			}
		}
		if chosen == nil {
			rpcError(-32602, "Unknown tool")
			return
		}
		if !has(scopes, chosen.Scope) {
			rpcError(-32602, "Insufficient permission")
			return
		}
		if p.Args.Workspace != workspace || !uuid.MatchString(workspace) {
			rpcError(-32602, "Workspace access denied")
			return
		}
		path := chosen.Path
		if strings.Contains(path, "{id}") {
			if !uuid.MatchString(p.Args.ID) {
				rpcError(-32602, "Invalid record ID")
				return
			}
			path = strings.ReplaceAll(path, "{id}", p.Args.ID)
		}
		if p.Name == "create_campaign" || p.Name == "update_campaign" {
			if state, ok := p.Args.Data["status"]; ok && state != "draft" && state != "paused" {
				rpcError(-32602, "Use launch_campaign to activate a campaign")
				return
			}
		}
		body, _ := json.Marshal(p.Args.Data)
		forward, _ := http.NewRequestWithContext(r.Context(), chosen.Method, "http://internal/api/workspaces/"+workspace+"/"+path, bytes.NewReader(body))
		forward.Header.Set("Content-Type", "application/json")
		if oauth {
			forward.Header.Set("X-Tullips-User", user)
			forward.Header.Set("X-Tullips-Signature", core.SignUser(s.Core.InternalSecret, user))
		} else {
			forward.Header.Set("Authorization", r.Header.Get("Authorization"))
		}
		recorder := httptest.NewRecorder()
		s.API.ServeHTTP(recorder, forward)
		response := recorder.Result()
		b, _ := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		response.Body.Close()
		respond(map[string]any{"content": []map[string]string{{"type": "text", "text": string(b)}}, "isError": response.StatusCode >= 400})
	default:
		rpcError(-32601, "Method not found")
	}
}
