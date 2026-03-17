package driver

import (
	"context"
	"fmt"
	"time"

	logger "pre-processor/utils/logger"

	"github.com/jackc/pgx/v5/pgxpool"
)

// InitPreProcessorDB initializes a connection pool to the pre-processor dedicated database.
// Reads PP_DB_* environment variables for connection configuration.
func InitPreProcessorDB(ctx context.Context) (*pgxpool.Pool, error) {
	dbConfig := NewDatabaseConfigWithPrefix("PP_")

	connString := dbConfig.BuildConnectionString()
	logger.Logger.InfoContext(ctx, "Pre-processor DB connection",
		"host", dbConfig.Host,
		"port", dbConfig.Port,
		"database", dbConfig.DBName,
	)

	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to parse pre-processor DB config", "error", err)
		return nil, fmt.Errorf("failed to parse pre-processor DB config: %w", err)
	}

	config.MaxConns = dbConfig.MaxConns
	config.MinConns = dbConfig.MinConns
	config.MaxConnLifetime = 1 * time.Hour
	config.MaxConnIdleTime = 30 * time.Minute
	config.ConnConfig.Tracer = &QueryTracer{}

	dbPool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to connect to pre-processor DB", "error", err)
		return nil, fmt.Errorf("failed to connect to pre-processor DB: %w", err)
	}

	if err := dbPool.Ping(ctx); err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to ping pre-processor DB", "error", err)
		dbPool.Close()
		return nil, fmt.Errorf("failed to ping pre-processor DB: %w", err)
	}

	logger.Logger.InfoContext(ctx, "Connected to pre-processor DB",
		"max_conns", config.MaxConns,
		"min_conns", config.MinConns,
	)

	return dbPool, nil
}
