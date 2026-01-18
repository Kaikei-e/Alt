package alt_db

import (
	"context"
	"fmt"
	"math"
	"os"
	"time"

	"alt/utils/logger"

	"github.com/jackc/pgx/v5/pgxpool"
)

func InitDBConnectionPool(ctx context.Context) (*pgxpool.Pool, error) {
	connStr, err := getDBConnectionString()
	if err != nil {
		return nil, fmt.Errorf("failed to build connection string: %w", err)
	}

	// pgxpool.ParseConfig使用で接続文字列妥当性確認
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("invalid connection string: %w", err)
	}

	// プール設定を明示的に設定（bounds checking付き）
	maxConns := getEnvIntOrDefault("DB_MAX_CONNS", 20)
	if maxConns > math.MaxInt32 {
		logger.Logger.WarnContext(ctx, "DB_MAX_CONNS value too large, using maximum allowed value",
			"provided", maxConns, "max_allowed", math.MaxInt32)
		maxConns = math.MaxInt32
	}
	config.MaxConns = int32(maxConns)

	minConns := getEnvIntOrDefault("DB_MIN_CONNS", 5)
	if minConns > math.MaxInt32 {
		logger.Logger.WarnContext(ctx, "DB_MIN_CONNS value too large, using maximum allowed value",
			"provided", minConns, "max_allowed", math.MaxInt32)
		minConns = math.MaxInt32
	}
	config.MinConns = int32(minConns)

	maxConnLifetime, _ := time.ParseDuration(getEnvOrDefault("DB_MAX_CONN_LIFE", "30m"))
	config.MaxConnLifetime = maxConnLifetime
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
				time.Sleep(retryDelay * time.Duration(i+1))
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
				time.Sleep(retryDelay * time.Duration(i+1))
				continue
			}
			return nil, fmt.Errorf("failed to ping database after %d attempts: %w", maxRetries, err)
		}
		cancel()

		// 成功ログ（SSL情報削除）
		logger.Logger.InfoContext(ctx, "Database connection established via Linkerd mTLS",
			"database", os.Getenv("DB_NAME"),
			"attempt", i+1,
			"max_conns", pool.Config().MaxConns,
			"min_conns", pool.Config().MinConns,
			"transport", "HTTP-over-mTLS")

		return pool, nil
	}

	return nil, fmt.Errorf("exhausted all connection attempts")
}

func getDBConnectionString() (string, error) {
	config := NewDatabaseConfigFromEnv()

	logger.Logger.InfoContext(context.Background(), "Database configuration for Linkerd",
		"host", config.Host,
		"port", config.Port,
		"database", config.DBName,
		"ssl_mode", "disabled",
		"mtls_provider", "linkerd-proxy")

	return config.BuildConnectionString(), nil
}

func envChecker(env string, variable string) (string, error) {
	if env == "" {
		logger.Logger.ErrorContext(context.Background(), "Environment variable is not set", "variable", variable)
		return "", fmt.Errorf("environment variable is not set: %s", variable)
	}
	return env, nil
}
