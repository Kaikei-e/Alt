package alt_db

import (
	"context"
	"fmt"
	"os"

	"alt/utils/logger"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

func InitDBConnection(ctx context.Context) (*pgx.Conn, error) {

	db, err := pgx.Connect(ctx, getDBConnectionString())
	if err != nil {
		logger.Logger.Error("Failed to connect to database", "error", err)
		return nil, err
	}

	err = db.Ping(ctx)
	if err != nil {
		logger.Logger.Error("Failed to ping database", "error", err)
		return nil, err
	}

	logger.Logger.Info("Connected to database", "database", os.Getenv("DB_NAME"))

	return db, nil
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
