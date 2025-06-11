package alt_db

import (
	"context"
	"fmt"
	"os"
	"time"

	"alt/utils/logger"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

func InitDBConnection(ctx context.Context) (*pgx.Conn, error) {
	const maxRetries = 10
	const retryInterval = 2 * time.Second

	var db *pgx.Conn
	var err error

	for i := 0; i < maxRetries; i++ {
		db, err = pgx.Connect(ctx, getDBConnectionString())
		if err == nil {
			// Test the connection
			err = db.Ping(ctx)
			if err == nil {
				logger.Logger.Info("Connected to database", "database", os.Getenv("DB_NAME"), "attempt", i+1)
				return db, nil
			}
			// Close the connection if ping failed
			db.Close(ctx)
		}

		if i < maxRetries-1 {
			logger.Logger.Warn("Database connection failed, retrying...",
				"error", err,
				"attempt", i+1,
				"max_retries", maxRetries,
				"retry_in", retryInterval)
			time.Sleep(retryInterval)
		}
	}

	logger.Logger.Error("Failed to connect to database after all retries",
		"error", err,
		"max_retries", maxRetries)
	return nil, fmt.Errorf("failed to connect to database after %d retries: %w", maxRetries, err)
}

func getDBConnectionString() string {
	err := godotenv.Load()
	if err != nil {
		logger.Logger.Error("Failed to load .env file", "error", err)
		os.Exit(1)
	}

	host := envChecker(os.Getenv("DB_HOST"), "DB_HOST")
	port := envChecker(os.Getenv("DB_PORT"), "DB_PORT")
	user := envChecker(os.Getenv("DB_USER"), "DB_USER")
	password := envChecker(os.Getenv("DB_PASSWORD"), "DB_PASSWORD")
	dbname := envChecker(os.Getenv("DB_NAME"), "DB_NAME")

	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname)
}

func envChecker(env string, variable string) string {
	if env == "" {
		logger.Logger.Error("Environment variable is not set", "variable", variable)
		os.Exit(1)
	}
	return env
}
