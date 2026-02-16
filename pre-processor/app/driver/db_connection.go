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
			logger.Logger.WarnContext(ctx, "Database connection busy, retrying",
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

// Init initializes a new database connection pool.
func Init(ctx context.Context) (*pgxpool.Pool, error) {
	// 新しい設定構造体を使用
	dbConfig := NewDatabaseConfig()

	// 追加: SSL関連の環境変数をログ出力
	logger.Logger.InfoContext(ctx, "SSL ENV",
		"DB_SSL_MODE", dbConfig.SSL.Mode,
		"DB_SSL_ROOT_CERT", dbConfig.SSL.RootCert,
		"DB_SSL_CERT", dbConfig.SSL.Cert,
		"DB_SSL_KEY", dbConfig.SSL.Key,
	)

	// 追加: 証明書ファイルの存在チェック
	for _, path := range []string{dbConfig.SSL.RootCert, dbConfig.SSL.Cert, dbConfig.SSL.Key} {
		if path != "" {
			if _, err := os.Stat(path); err != nil {
				logger.Logger.ErrorContext(ctx, "SSL cert file not found", "path", path, "error", err)
			} else {
				logger.Logger.InfoContext(ctx, "SSL cert file found", "path", path)
			}
		}
	}

	// 追加: 実際の接続文字列をログ出力
	connString := dbConfig.BuildConnectionString()
	logger.Logger.InfoContext(ctx, "DB connection string", "conn", connString)

	// SSL設定の検証
	if err := dbConfig.ValidateSSLConfig(); err != nil {
		logger.Logger.ErrorContext(ctx, "Invalid SSL configuration", "error", err)
		return nil, fmt.Errorf("invalid SSL configuration: %w", err)
	}

	// ログで設定内容を出力
	logger.Logger.InfoContext(ctx, "Database configuration",
		"host", dbConfig.Host,
		"port", dbConfig.Port,
		"database", dbConfig.DBName,
		"sslmode", dbConfig.SSL.Mode,
		"max_conns", dbConfig.MaxConns,
	)

	// Parse the connection string to create pool config
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to parse database config", "error", err)
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
		logger.Logger.ErrorContext(ctx, "Failed to connect to database", "error", err)
		return nil, err
	}

	// Test the connection
	err = dbPool.Ping(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to ping database",
			"error", err,
			"sslmode", dbConfig.SSL.Mode)
		dbPool.Close()
		return nil, err
	}

	// SSL接続状況確認
	conn, err := dbPool.Acquire(ctx)
	if err != nil {
		logger.Logger.WarnContext(ctx, "Could not acquire connection to check SSL status", "error", err)
	} else {
		defer conn.Release()

		var sslUsed bool
		err := conn.QueryRow(ctx, "SELECT ssl_is_used()").Scan(&sslUsed)
		if err != nil {
			logger.Logger.WarnContext(ctx, "Could not check SSL status", "error", err)
		} else {
			logger.Logger.InfoContext(ctx, "Database connection established",
				"ssl_enabled", sslUsed,
				"sslmode", dbConfig.SSL.Mode,
			)
		}
	}

	logger.Logger.InfoContext(ctx, "Connected to database pool", "max_conns", config.MaxConns, "min_conns", config.MinConns)

	return dbPool, nil
}
