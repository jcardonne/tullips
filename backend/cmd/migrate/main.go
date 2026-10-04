package main

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tullips/tullips/backend/internal/migrate"
	"log"
	"os"
)

func main() {
	db, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err = migrate.Run(context.Background(), db); err != nil {
		log.Fatal(err)
	}
}
