package core

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type Dispatch func(context.Context, string, map[string]any) (map[string]any, error)

// RunJobs leases work atomically. Expired leases retry; handlers must honor idempotency keys for external sends.
func (s *Server) RunJobs(ctx context.Context, dispatch Dispatch) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ctx.Err() != nil {
				return
			}
			if s.Maintenance(ctx) {
				continue
			}
			s.runJob(context.WithoutCancel(ctx), dispatch)
		}
	}
}
func (s *Server) runJob(ctx context.Context, dispatch Dispatch) {
	// ponytail: one global worker lease serializes quota decisions; per-account leases when throughput requires parallel delivery.
	conn, e := s.DB.Acquire(ctx)
	if e != nil {
		return
	}
	defer conn.Release()
	var locked bool
	if e = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(7438291)`).Scan(&locked); e != nil || !locked {
		return
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(7438291)`)
	s.DB.Exec(ctx, `UPDATE jobs SET status='delivery_uncertain',last_error='Worker interrupted during delivery; inspect provider before retrying' WHERE status='running' AND kind IN ('workflow.step','message.send') AND locked_at<now()-interval '5 minutes'`)
	var id, kind, workspace string
	var payload []byte
	err := s.DB.QueryRow(ctx, `UPDATE jobs SET status='running',locked_at=now(),attempts=attempts+1,updated_at=now() WHERE id=(SELECT id FROM jobs WHERE (status='pending' AND run_at<=now()) OR (status='running' AND locked_at<now()-interval '5 minutes') ORDER BY run_at FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING id,kind,payload,workspace_id`).Scan(&id, &kind, &payload, &workspace)
	if err != nil {
		return
	}
	var data map[string]any
	if json.Unmarshal(payload, &data) != nil {
		data = map[string]any{}
	}
	data["idempotency_key"] = id
	jobCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	var result map[string]any
	if kind == "website.analyze" || kind == "ai.chat" || kind == "ai.draft" {
		result, err = s.AIAction(jobCtx, workspace, id, kind, data)
	} else if kind == "message.send" {
		result, err = s.ReplyAction(jobCtx, workspace, id, data)
	} else if kind == "linkedin.search" {
		result, err = s.SearchAction(jobCtx, workspace, data)
	} else if kind == "workflow.step" {
		result, err = s.WorkflowStep(jobCtx, workspace, id, data)
	} else {
		result, err = dispatch(jobCtx, kind, data)
	}
	cancel()
	if err != nil {
		if kind == "workflow.step" || kind == "message.send" {
			s.DB.Exec(ctx, `UPDATE jobs SET status='delivery_uncertain',last_error=$2,updated_at=now() WHERE id=$1`, id, err.Error())
			return
		}
		s.DB.Exec(ctx, `UPDATE jobs SET status=CASE WHEN attempts>=3 THEN 'failed' ELSE 'pending' END,last_error=$2,run_at=now()+interval '1 minute'*attempts,updated_at=now() WHERE id=$1`, id, err.Error())
		return
	}
	if state, ok := result["__status"].(string); ok {
		s.DB.Exec(ctx, `UPDATE jobs SET status=$2,run_at=$3,last_error=$4,updated_at=now() WHERE id=$1`, id, state, result["__run_at"], result["__error"])
		return
	}
	b, _ := json.Marshal(result)
	s.DB.Exec(ctx, `UPDATE jobs SET status='completed',payload=payload || jsonb_build_object('result',$2::jsonb),updated_at=now() WHERE id=$1`, id, b)
}
func (s *Server) actions(w http.ResponseWriter, r *http.Request, ws string) {
	if r.Method == "GET" {
		rows, e := s.DB.Query(r.Context(), `SELECT jsonb_build_object('id',id,'kind',kind,'status',status,'payload',payload,'last_error',last_error,'created_at',created_at) FROM jobs WHERE workspace_id=$1 ORDER BY created_at DESC LIMIT 100`, ws)
		s.rows(w, rows, e)
		return
	}
	if r.Method != "POST" {
		fail(w, 405, "method not allowed")
		return
	}
	d, e := decode(w, r)
	if e != nil {
		fail(w, 400, "invalid JSON")
		return
	}
	kind := str(d, "kind")
	if kind != "website.analyze" && kind != "ai.chat" && kind != "ai.draft" && kind != "linkedin.search" {
		fail(w, 400, "unsupported action; live delivery requires a verified integration")
		return
	}
	payload, ok := d["payload"].(map[string]any)
	if !ok || (kind == "website.analyze" && str(payload, "url") == "") || (kind != "website.analyze" && kind != "linkedin.search" && str(payload, "prompt") == "") {
		fail(w, 400, "payload.url or payload.prompt required")
		return
	}
	if kind == "linkedin.search" {
		if str(payload, "account_id") == "" || str(payload, "keywords") == "" {
			fail(w, 400, "account_id and keywords required")
			return
		}
		if e = s.validate(r.Context(), ws, "prospects", payload); e != nil {
			fail(w, 400, e.Error())
			return
		}
		if _, _, e = s.accountData(r.Context(), ws, str(payload, "account_id")); e != nil {
			fail(w, 400, "invalid account_id")
			return
		}
	}
	b, _ := json.Marshal(payload)
	var id string
	e = s.DB.QueryRow(r.Context(), `INSERT INTO jobs(workspace_id,kind,payload,idempotency_key) VALUES($1,$2,$3,$4) ON CONFLICT(idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key WHERE jobs.workspace_id=EXCLUDED.workspace_id RETURNING id`, ws, kind, b, ws+":"+NewToken()).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		fail(w, 409, "duplicate job")
		return
	}
	s.result(w, e, map[string]string{"id": id, "status": "pending"})
}
