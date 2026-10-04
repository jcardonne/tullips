package main

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tullips/tullips/backend/internal/core"
	"github.com/tullips/tullips/backend/internal/migrate"
	"github.com/tullips/tullips/backend/internal/platform"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	if e = migrate.Check(ctx, db); e != nil {
		log.Fatal(e)
	}
	secret := os.Getenv("INTERNAL_API_SECRET")
	if len(secret) < 32 {
		log.Fatal("INTERNAL_API_SECRET must contain at least 32 characters")
	}
	s := &core.Server{DB: db, InternalSecret: secret, EncryptionKey: os.Getenv("ENCRYPTION_KEY")}
	p, e := platform.New(s, os.Getenv("APP_URL"))
	if e != nil {
		log.Fatal(e)
	}
	bind := os.Getenv("API_BIND")
	if bind == "" {
		bind = ":8080"
	}
	srv := &http.Server{Addr: bind, Handler: p.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 90 * time.Second}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 70*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	log.Print("Tullips API listening on :8080")
	if e = srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Fatal(e)
	}
	if ctx.Err() != nil {
		<-shutdownDone
	}
}
