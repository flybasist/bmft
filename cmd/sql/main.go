package main

import (
	"context"
	"database/sql"
	"log"
	"os"

	"github.com/flybasist/bmft/internal/postgresql"
	_ "github.com/lib/pq"
)

func main() {
	pgURL := os.Getenv("POSTGRES_DSN")
	kafkaAddr := os.Getenv("KAFKA_BROKERS")
	if pgURL == "" || kafkaAddr == "" {
		log.Fatal("POSTGRES_DSN or KAFKA_BROKERS not set")
	}

	ctx := context.Background()
	postgresql.EnsureDatabaseExists(pgURL)

	db, err := sql.Open("postgres", pgURL)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	defer db.Close()

	postgresql.StartKafkaToPostgres(ctx, kafkaAddr, db)
}
