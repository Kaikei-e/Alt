package alt_db

import (
	"fmt"
	"os"
	"strconv"
)

// SSL設定構造体完全削除
type DatabaseConfig struct {
	Host        string
	Port        string
	User        string
	Password    string
	DBName      string
	// 接続プール設定追加
	MaxConns    int
	MinConns    int
	MaxConnLife string
}

func NewDatabaseConfigFromEnv() *DatabaseConfig {
	return &DatabaseConfig{
		Host:        getEnvOrDefault("DB_HOST", "localhost"),
		Port:        getEnvOrDefault("DB_PORT", "5432"),
		User:        getEnvOrDefault("DB_USER", "devuser"),
		Password:    getEnvOrDefault("DB_PASSWORD", "devpassword"),
		DBName:      getEnvOrDefault("DB_NAME", "devdb"),
		MaxConns:    getEnvIntOrDefault("DB_MAX_CONNS", 20),
		MinConns:    getEnvIntOrDefault("DB_MIN_CONNS", 5),
		MaxConnLife: getEnvOrDefault("DB_MAX_CONN_LIFE", "30m"),
	}
}

// Linkerd環境最適化接続文字列 - 不正パラメータ削除
func (dc *DatabaseConfig) BuildConnectionString() string {
	// sslmode=disable固定 + 基本PostgreSQL接続パラメータのみ
	connectTimeout := getEnvIntOrDefault("DB_CONNECT_TIMEOUT_SECONDS", 90)
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable search_path=public connect_timeout=%d",
		dc.Host, dc.Port, dc.User, dc.Password, dc.DBName, connectTimeout,
	)
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
