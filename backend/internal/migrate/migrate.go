// Package migrate applies ordered SQL migrations explicitly, never during API startup.
package migrate

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tullips/tullips/backend/migrations"
)

func Run(ctx context.Context, db *pgxpool.Pool) error {
	conn, err := db.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(7483100)`); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(7483100)`)
	if _, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		raw, err := migrations.Files.ReadFile(entry.Name())
		if err != nil {
			return err
		}
		checksum := fmt.Sprintf("%x", sha256.Sum256(raw))
		var existing string
		err = conn.QueryRow(ctx, `SELECT checksum FROM schema_migrations WHERE name=$1`, entry.Name()).Scan(&existing)
		if err == nil {
			if existing != checksum {
				return fmt.Errorf("migration %s changed after application", entry.Name())
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(raw)); err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO schema_migrations(name,checksum) VALUES($1,$2)`, entry.Name(), checksum)
		}
		if err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", entry.Name(), err)
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func Check(ctx context.Context, db *pgxpool.Pool) error {
	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		var checksum string
		raw, _ := migrations.Files.ReadFile(entry.Name())
		if err := db.QueryRow(ctx, `SELECT checksum FROM schema_migrations WHERE name=$1`, entry.Name()).Scan(&checksum); err != nil || checksum != fmt.Sprintf("%x", sha256.Sum256(raw)) {
			return fmt.Errorf("database migrations are missing or incompatible: run the migration command")
		}
	}
	return nil
}
