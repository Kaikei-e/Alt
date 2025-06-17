package db

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Init(ctx context.Context) *pgxpool.Pool {
	dbURL := loadEnv()

	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatal(err)
	}

	return db
}

func loadEnv() string {
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		log.Fatal("DB_HOST is not set")
	}

	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		log.Fatal("DB_PORT is not set")
	}

	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		log.Fatal("DB_NAME is not set")
	}

	dbUser := os.Getenv("SEARCH_INDEXER_DB_USER")
	if dbUser == "" {
		log.Fatal("SEARCH_INDEXER_DB_USER is not set")
	}

	dbPassword := os.Getenv("SEARCH_INDEXER_DB_PASSWORD")
	if dbPassword == "" {
		log.Fatal("SEARCH_INDEXER_DB_PASSWORD is not set")
	}

	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", dbHost, dbPort, dbUser, dbPassword, dbName)
}
