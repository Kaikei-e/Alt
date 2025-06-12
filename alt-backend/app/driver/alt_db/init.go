package alt_db

import (
	"context"
	"fmt"
	"os"
	"time"

	"alt/utils/logger"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func InitDBConnectionPool(ctx context.Context) (*pgxpool.Pool, error) {
	const maxRetries = 10
	const retryInterval = 2 * time.Second

	var pool *pgxpool.Pool
	var err error

	for i := 0; i < maxRetries; i++ {
		pool, err = pgxpool.New(ctx, getDBConnectionString())
		if err == nil {
			// Test the connection pool
			err = pool.Ping(ctx)
			if err == nil {
				logger.Logger.Info("Connected to database with connection pool",
					"database", os.Getenv("DB_NAME"),
					"attempt", i+1,
					"max_conns", pool.Config().MaxConns,
					"min_conns", pool.Config().MinConns)
				return pool, nil
			}
			// Close the pool if ping failed
			pool.Close()
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

	// Connection pool configuration with optimal settings
	connectionString := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable"+
			" pool_max_conns=25"+ // Maximum number of connections in pool
			" pool_min_conns=5"+ // Minimum number of connections in pool
			" pool_max_conn_lifetime=30m"+ // Maximum time connection can be reused
			" pool_max_conn_idle_time=15m"+ // Maximum time connection can be idle
			" pool_health_check_period=1m", // How often to check connection health
		host, port, user, password, dbname)

	return connectionString
}

func envChecker(env string, variable string) string {
	if env == "" {
		logger.Logger.Error("Environment variable is not set", "variable", variable)
		os.Exit(1)
	}
	return env
}
