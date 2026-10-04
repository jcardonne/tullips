package core

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWorkflowDatabase(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL for PostgreSQL workflow regression")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close()
	schema := "test_engine_" + HashToken(NewToken())[:12]
	if _, e = admin.Exec(ctx, `CREATE SCHEMA `+schema); e != nil {
		t.Fatal(e)
	}
	defer admin.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`)
	config, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	db, e := pgxpool.NewWithConfig(ctx, config)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	migration, e := os.ReadFile("../../migrations/001_init.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(ctx, string(migration)); e != nil {
		t.Fatal(e)
	}
	s := &Server{DB: db, InternalSecret: strings.Repeat("s", 32), EncryptionKey: strings.Repeat("e", 32)}
	var ws string
	if e = db.QueryRow(ctx, `INSERT INTO workspaces(name) VALUES('engine regression') RETURNING id`).Scan(&ws); e != nil {
		t.Fatal(e)
	}
	defer db.Exec(ctx, `DELETE FROM workspaces WHERE id=$1`, ws)
	db.Exec(ctx, `INSERT INTO memberships VALUES($1,'test-engine','owner')`, ws)
	create := func(kind string, data map[string]any) string {
		b, _ := json.Marshal(data)
		var id string
		if e := db.QueryRow(ctx, `INSERT INTO records(workspace_id,kind,data) VALUES($1,$2,$3) RETURNING id`, ws, kind, b).Scan(&id); e != nil {
			t.Fatal(e)
		}
		return id
	}
	account := create("accounts", map[string]any{"channel": "linkedin", "timezone": "UTC", "working_days": []int{0, 1, 2, 3, 4, 5, 6}, "start_hour": 0, "end_hour": 24, "status": "disconnected"})
	campaign := create("campaigns", map[string]any{"name": "Regression", "autonomy": "automatic", "status": "draft", "account_ids": []string{account}, "workflow": []map[string]any{{"type": "condition", "condition": "accepted"}, {"type": "linkedin_message", "message": "Hello {{first_name}}"}}})
	prospect := create("prospects", map[string]any{"first_name": "Ada", "campaign_id": campaign, "invited_at": time.Now().Add(-5 * 24 * time.Hour).Format(time.RFC3339)})
	r := httptest.NewRequest("POST", "/api/workspaces/"+ws+"/campaigns/"+campaign+"/launch", strings.NewReader("{}"))
	r.Header.Set("X-Tullips-User", "test-engine")
	r.Header.Set("X-Tullips-Signature", SignUser(s.InternalSecret, "test-engine"))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("launch %d %s", w.Code, w.Body.String())
	}
	dispatch := func(context.Context, string, map[string]any) (map[string]any, error) {
		t.Fatal("unexpected external dispatch")
		return nil, nil
	}
	s.runJob(ctx, dispatch)
	var step int
	var state string
	if e = db.QueryRow(ctx, `SELECT step,status FROM enrollments WHERE prospect_id=$1`, prospect).Scan(&step, &state); e != nil {
		t.Fatal(e)
	}
	if step != 0 || state != "active" {
		t.Fatal("unaccepted prospect advanced")
	}
	db.Exec(ctx, `UPDATE records SET data=data || jsonb_build_object('accepted_at',$2::text) WHERE id=$1`, prospect, time.Now().Format(time.RFC3339))
	db.Exec(ctx, `UPDATE jobs SET run_at=now() WHERE prospect_id=$1`, prospect)
	s.runJob(ctx, dispatch)
	db.QueryRow(ctx, `SELECT step FROM enrollments WHERE prospect_id=$1`, prospect).Scan(&step)
	if step != 1 {
		t.Fatal("acceptance failed to advance")
	}
	s.runJob(ctx, dispatch)
	db.QueryRow(ctx, `SELECT status FROM jobs WHERE prospect_id=$1 ORDER BY created_at DESC LIMIT 1`, prospect).Scan(&state)
	if state != "blocked" {
		t.Fatalf("disconnected delivery must block, got %s", state)
	}
	db.Exec(ctx, `UPDATE records SET data=data || '{"status":"replied"}' WHERE id=$1`, prospect)
	db.Exec(ctx, `UPDATE jobs SET status='pending',run_at=now() WHERE prospect_id=$1 AND status='blocked'`, prospect)
	s.runJob(ctx, dispatch)
	db.QueryRow(ctx, `SELECT status FROM enrollments WHERE prospect_id=$1`, prospect).Scan(&state)
	if state != "stopped" {
		t.Fatal("reply did not stop sequence")
	}
	// A delayed worker must count from the prior send, not the worker restart.
	delayCampaign := create("campaigns", map[string]any{"status": "active", "autonomy": "automatic", "account_ids": []string{account}, "workflow": []map[string]any{{"type": "delay", "days": 2}, {"type": "stop"}}})
	delayedProspect := create("prospects", map[string]any{"campaign_id": delayCampaign})
	result, err := s.EnrollCampaign(ctx, ws, delayCampaign, false)
	if err != nil || result["enrolled"] != 1 {
		t.Fatalf("delay enrollment: %v %v", result, err)
	}
	anchor := time.Date(2026, 1, 2, 17, 59, 59, 0, time.UTC)
	db.Exec(ctx, `UPDATE jobs SET payload=payload || jsonb_build_object('anchor_at',$2::text),status='pending',run_at=now() WHERE prospect_id=$1`, delayedProspect, anchor.Format(time.RFC3339Nano))
	s.runJob(ctx, dispatch)
	var runAt time.Time
	if err = db.QueryRow(ctx, `SELECT run_at FROM jobs WHERE prospect_id=$1 AND payload->'step'->>'type'='stop'`, delayedProspect).Scan(&runAt); err != nil {
		t.Fatal(err)
	}
	if !runAt.Equal(anchor.AddDate(0, 0, 2)) {
		t.Fatalf("delay shifted from anchor: %s", runAt)
	}

}
