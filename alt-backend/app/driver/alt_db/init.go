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
		connStr, err := getDBConnectionString()
		if err != nil {
			logger.Logger.Error("Failed to get database connection string", "error", err)
			return nil, fmt.Errorf("failed to get database connection string: %w", err)
		}

		pool, err = pgxpool.New(ctx, connStr)
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

func getDBConnectionString() (string, error) {
	err := godotenv.Load()
	if err != nil {
		logger.Logger.Error("Failed to load .env file", "error", err)
		return "", fmt.Errorf("failed to load .env file: %w", err)
	}

	host, err := envChecker(os.Getenv("DB_HOST"), "DB_HOST")
	if err != nil {
		return "", err
	}
	port, err := envChecker(os.Getenv("DB_PORT"), "DB_PORT")
	if err != nil {
		return "", err
	}
	user, err := envChecker(os.Getenv("DB_USER"), "DB_USER")
	if err != nil {
		return "", err
	}
	password, err := envChecker(os.Getenv("DB_PASSWORD"), "DB_PASSWORD")
	if err != nil {
		return "", err
	}
	dbname, err := envChecker(os.Getenv("DB_NAME"), "DB_NAME")
	if err != nil {
		return "", err
	}

	// Connection pool configuration with optimal settings
	connectionString := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable"+
			" pool_max_conns=25"+ // Maximum number of connections in pool
			" pool_min_conns=5"+ // Minimum number of connections in pool
			" pool_max_conn_lifetime=30m"+ // Maximum time connection can be reused
			" pool_max_conn_idle_time=15m"+ // Maximum time connection can be idle
			" pool_health_check_period=1m", // How often to check connection health
		host, port, user, password, dbname)

	return connectionString, nil
}

func envChecker(env string, variable string) (string, error) {
	if env == "" {
		logger.Logger.Error("Environment variable is not set", "variable", variable)
		return "", fmt.Errorf("environment variable is not set: %s", variable)
	}
	return env, nil
}
