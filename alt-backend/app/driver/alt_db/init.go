package alt_db

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"alt/utils/logger"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func InitDBConnection() (*sql.DB, error) {

	db, err := sql.Open("postgres", getDBConnectionString())
	if err != nil {
		logger.Logger.Error("Failed to connect to database", "error", err)
		return nil, err
	}

	db.SetMaxOpenConns(100)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Hour)

	err = db.Ping()
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

	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
	)
}
