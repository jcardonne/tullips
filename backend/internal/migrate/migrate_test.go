package migrate

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
)

func TestMigrationsAreSerializedAndChecked(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable database")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// A dedicated schema isolates this check from the workflow tests.
	if _, err = db.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS test_updater_migrations`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DROP SCHEMA test_updater_migrations CASCADE`)
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = "test_updater_migrations"
	isolated, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer isolated.Close()
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- Run(ctx, isolated) }()
	}
	for range 2 {
		if err = <-results; err != nil {
			t.Fatal(err)
		}
	}
	if err = Check(ctx, isolated); err != nil {
		t.Fatal(err)
	}
	if _, err = isolated.Exec(ctx, `UPDATE schema_migrations SET checksum='tampered' WHERE name='001_init.sql'`); err != nil {
		t.Fatal(err)
	}
	if err = Run(ctx, isolated); err == nil {
		t.Fatal("modified migration accepted")
	}
	if err = Check(ctx, isolated); err == nil {
		t.Fatal("incompatible database accepted")
	}
}
