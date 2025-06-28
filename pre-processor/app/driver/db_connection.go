package driver

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	logger "pre-processor/utils/logger"

	"github.com/jackc/pgx/v5/pgxpool"
)

// retryDBOperation retries database operations that fail with "conn busy" errors.
func retryDBOperation(ctx context.Context, operation func() error, operationName string) error {
	maxRetries := 3
	baseDelay := 100 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}

		// Check if this is a conn busy error
		if strings.Contains(err.Error(), "conn busy") && attempt < maxRetries-1 {
			delay := baseDelay * time.Duration(1<<attempt) // Exponential backoff
			logger.Logger.Warn("Database connection busy, retrying",
				"operation", operationName,
				"attempt", attempt+1,
				"max_retries", maxRetries,
				"retry_delay", delay,
				"error", err)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		// If it's not a conn busy error or we've exhausted retries, return the error
		return err
	}

	return fmt.Errorf("operation %s failed after %d retries", operationName, maxRetries)
}

// Init initializes a new database connection pool.
func Init(ctx context.Context) (*pgxpool.Pool, error) {
	// Build connection string
	connString := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable pool_max_conns=20 pool_min_conns=5 pool_max_conn_lifetime=1h pool_max_conn_idle_time=30m",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("PRE_PROCESSOR_DB_USER"),
		os.Getenv("PRE_PROCESSOR_DB_PASSWORD"),
		os.Getenv("DB_NAME"))

	// Parse the connection string to create pool config
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		logger.Logger.Error("Failed to parse database config", "error", err)
		return nil, err
	}

	// Additional pool configuration
	config.MaxConns = 20
	config.MinConns = 5
	config.MaxConnLifetime = 1 * time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

	// Add tracer
	config.ConnConfig.Tracer = &QueryTracer{}

	// Create the pool with the configuration
	dbPool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		logger.Logger.Error("Failed to connect to database", "error", err)
		return nil, err
	}

	// Test the connection
	err = dbPool.Ping(ctx)
	if err != nil {
		logger.Logger.Error("Failed to ping database", "error", err)
		dbPool.Close()

		return nil, err
	}

	logger.Logger.Info("Connected to database pool", "max_conns", config.MaxConns, "min_conns", config.MinConns)

	return dbPool, nil
}
