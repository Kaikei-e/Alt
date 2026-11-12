package alt_db

import (
	"context"
	"fmt"
	"time"

	"alt/utils/logger"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func InitDBConnectionPool(ctx context.Context) (*pgxpool.Pool, error) {
	// Fail-fast on int32 pool bounds before DSN construction.
	maxConns, minConns, err := poolConnsFromEnv()
	if err != nil {
		return nil, fmt.Errorf("invalid pool size config: %w", err)
	}

	dbConfig, connStr, err := loadDBConnectionConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to build connection string: %w", err)
	}

	// pgxpool.ParseConfig使用で接続文字列妥当性確認
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("invalid connection string: %w", err)
	}

	config.MaxConns = maxConns
	config.MinConns = minConns

	// PgBouncer transaction pooling互換: simple protocolを使用
	// 拡張プロトコルのprepared statementキャッシュがtransaction poolingで問題になるのを回避
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	maxConnLifetime, err := time.ParseDuration(getEnvOrDefault("DB_MAX_CONN_LIFE", "1h"))
	if err != nil {
		return nil, fmt.Errorf("invalid DB_MAX_CONN_LIFE: %w", err)
	}
	config.MaxConnLifetime = maxConnLifetime
	config.MaxConnIdleTime = 30 * time.Minute
	config.HealthCheckPeriod = time.Minute

	// Linkerd環境での接続リトライ設定
	const maxRetries = 5
	const retryDelay = 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		pool, err := pgxpool.NewWithConfig(ctx, config)
		if err != nil {
			logger.Logger.WarnContext(ctx, "Database connection pool creation failed",
				"attempt", i+1, "error", err, "connection_string_valid", true)
			if i < maxRetries-1 {
				if waitErr := waitOrCancel(ctx, retryDelay*time.Duration(i+1)); waitErr != nil {
					return nil, waitErr
				}
				continue
			}
			return nil, fmt.Errorf("failed to create connection pool after %d attempts: %w", maxRetries, err)
		}

		// 接続テスト
		pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err = pool.Ping(pingCtx); err != nil {
			cancel()
			pool.Close()
			logger.Logger.WarnContext(ctx, "Database ping failed",
				"attempt", i+1, "error", err)
			if i < maxRetries-1 {
				if waitErr := waitOrCancel(ctx, retryDelay*time.Duration(i+1)); waitErr != nil {
					return nil, waitErr
				}
				continue
			}
			return nil, fmt.Errorf("failed to ping database after %d attempts: %w", maxRetries, err)
		}
		cancel()

		logger.Logger.InfoContext(ctx, "Database connection pool established",
			"database", dbConfig.DBName,
			"ssl_mode", dbConfig.SSLMode,
			"attempt", i+1,
			"max_conns", pool.Config().MaxConns,
			"min_conns", pool.Config().MinConns)

		return pool, nil
	}

	return nil, fmt.Errorf("exhausted all connection attempts")
}

// waitOrCancel blocks for delay unless ctx is done first, in which case it
// returns ctx.Err() immediately instead of letting a bare time.Sleep hold up
// shutdown for the full backoff window.
func waitOrCancel(ctx context.Context, delay time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

func getDBConnectionString() (string, error) {
	_, connStr, err := loadDBConnectionConfig(context.Background())
	return connStr, err
}

func loadDBConnectionConfig(ctx context.Context) (*DatabaseConfig, string, error) {
	config, err := LoadDatabaseConfigFromEnv()
	if err != nil {
		return nil, "", err
	}

	logger.Logger.InfoContext(ctx, "Database configuration",
		"host", config.Host,
		"port", config.Port,
		"database", config.DBName,
		"ssl_mode", config.SSLMode)

	return config, config.BuildConnectionString(), nil
}

func envChecker(env string, variable string) (string, error) {
	if env == "" {
		logger.Logger.ErrorContext(context.Background(), "Environment variable is not set", "variable", variable)
		return "", fmt.Errorf("environment variable is not set: %s", variable)
	}
	return env, nil
}
