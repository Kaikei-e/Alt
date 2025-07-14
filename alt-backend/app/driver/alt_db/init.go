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
				// SSL接続状況の確認
				conn, connErr := pool.Acquire(ctx)
				if connErr != nil {
					logger.Logger.Warn("Could not acquire connection to check SSL status", "error", connErr)
				} else {
					defer conn.Release()
					
					var sslUsed bool
					sslErr := conn.QueryRow(ctx, "SELECT ssl_is_used()").Scan(&sslUsed)
					if sslErr != nil {
						logger.Logger.Warn("Could not check SSL status", "error", sslErr)
					} else {
						logger.Logger.Info("Database connection established",
							"ssl_enabled", sslUsed,
							"database", os.Getenv("DB_NAME"),
							"attempt", i+1,
							"max_conns", pool.Config().MaxConns,
							"min_conns", pool.Config().MinConns)
					}
				}
				
				if conn == nil {
					// SSL確認ができなかった場合の従来ログ
					logger.Logger.Info("Connected to database with connection pool",
						"database", os.Getenv("DB_NAME"),
						"attempt", i+1,
						"max_conns", pool.Config().MaxConns,
						"min_conns", pool.Config().MinConns)
				}
				
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

	// 新しい設定構造体を使用
	config := NewDatabaseConfigFromEnv()
	
	// SSL設定の検証
	if err := config.ValidateSSLConfig(); err != nil {
		logger.Logger.Error("Invalid SSL configuration", "error", err)
		return "", fmt.Errorf("invalid SSL configuration: %w", err)
	}

	// ログで設定内容を出力
	logger.Logger.Info("Database configuration",
		"host", config.Host,
		"port", config.Port,
		"database", config.DBName,
		"sslmode", config.SSL.Mode,
	)

	// 基本接続文字列を構築
	baseConn := config.BuildConnectionString()
	
	// 既存のプール設定を追加
	connectionString := baseConn +
		" pool_max_conn_lifetime=30m"+ // Maximum time connection can be reused
		" pool_max_conn_idle_time=15m"+ // Maximum time connection can be idle
		" pool_health_check_period=1m" // How often to check connection health

	return connectionString, nil
}

func envChecker(env string, variable string) (string, error) {
	if env == "" {
		logger.Logger.Error("Environment variable is not set", "variable", variable)
		return "", fmt.Errorf("environment variable is not set: %s", variable)
	}
	return env, nil
}
