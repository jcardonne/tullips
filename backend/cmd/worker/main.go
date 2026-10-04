package main

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tullips/tullips/backend/internal/core"
	"github.com/tullips/tullips/backend/internal/integrations"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, e := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if e != nil {
		log.Fatal(e)
	}
	defer db.Close()
	if e = db.Ping(ctx); e != nil {
		log.Fatal(e)
	}
	s := &core.Server{DB: db, EncryptionKey: os.Getenv("ENCRYPTION_KEY"), InternalSecret: os.Getenv("INTERNAL_API_SECRET")}
	var wg sync.WaitGroup
	for _, run := range []func(context.Context){s.SyncAccounts, s.DiscoverCampaigns, func(c context.Context) { s.RunJobs(c, integrations.Dispatch) }} {
		wg.Add(1)
		go func(run func(context.Context)) { defer wg.Done(); run(ctx) }(run)
	}
	bind := os.Getenv("WORKER_BIND")
	if bind == "" {
		bind = ":8081"
	}
	health := &http.Server{Addr: bind, ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ctx.Err() != nil || db.Ping(r.Context()) != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		w.Write([]byte("ok"))
	})}
	go health.ListenAndServe()
	wg.Wait()
	health.Close()
}
