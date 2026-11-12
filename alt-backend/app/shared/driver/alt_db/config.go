package alt_db

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
	// 接続プール設定追加
	MaxConns    int
	MinConns    int
	MaxConnLife string
}

// LoadDatabaseConfigFromEnv loads database configuration from environment
// variables, failing fast when password or TLS mode is missing or invalid.
func LoadDatabaseConfigFromEnv() (*DatabaseConfig, error) {
	password, err := loadRequiredPassword()
	if err != nil {
		return nil, err
	}
	sslMode, err := loadRequiredSSLMode()
	if err != nil {
		return nil, err
	}
	maxConns, err := parseEnvInt("DB_MAX_CONNS", 20)
	if err != nil {
		return nil, err
	}
	minConns, err := parseEnvInt("DB_MIN_CONNS", 5)
	if err != nil {
		return nil, err
	}

	return &DatabaseConfig{
		Host:        getEnvOrDefault("DB_HOST", "localhost"),
		Port:        getEnvOrDefault("DB_PORT", "5432"),
		User:        getEnvOrDefault("DB_USER", "devuser"),
		Password:    password,
		DBName:      getEnvOrDefault("DB_NAME", "devdb"),
		SSLMode:     sslMode,
		MaxConns:    maxConns,
		MinConns:    minConns,
		MaxConnLife: getEnvOrDefault("DB_MAX_CONN_LIFE", "30m"),
	}, nil
}

func (dc *DatabaseConfig) BuildConnectionString() string {
	connectTimeout := getEnvIntOrDefault("DB_CONNECT_TIMEOUT_SECONDS", 90)
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s search_path=public connect_timeout=%d",
		dc.Host, dc.Port, dc.User, dc.Password, dc.DBName, dc.SSLMode, connectTimeout,
	)
}

func loadRequiredPassword() (string, error) {
	if fileValue := os.Getenv("DB_PASSWORD_FILE"); fileValue != "" {
		content, err := os.ReadFile(filepath.Clean(fileValue)) //#nosec G304,G703 -- path from DB_PASSWORD_FILE env var (Docker Secrets)
		if err != nil {
			return "", fmt.Errorf("read DB_PASSWORD_FILE %q: %w", fileValue, err)
		}
		trimmed := strings.TrimSpace(string(content))
		if trimmed == "" {
			return "", fmt.Errorf("DB_PASSWORD_FILE %q is empty", fileValue)
		}
		return trimmed, nil
	}
	if value := os.Getenv("DB_PASSWORD"); value != "" {
		return value, nil
	}
	return "", errors.New("database password is required: set DB_PASSWORD or DB_PASSWORD_FILE")
}

func loadRequiredSSLMode() (string, error) {
	sslMode := strings.TrimSpace(os.Getenv("DB_SSL_MODE"))
	if sslMode == "" {
		return "", errors.New("DB_SSL_MODE is required: must be one of disable, require, verify-ca, verify-full")
	}
	switch sslMode {
	case "disable", "require", "verify-ca", "verify-full":
		return sslMode, nil
	default:
		return "", fmt.Errorf("invalid DB_SSL_MODE=%q: must be one of disable, require, verify-ca, verify-full", sslMode)
	}
}

func parseEnvInt(key string, defaultValue int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue, nil
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s=%q: must be an integer in [%d, %d]: %w", key, value, math.MinInt, math.MaxInt, err)
	}
	return intValue, nil
}

func (dc *DatabaseConfig) BuildDirectConnectionString(hostOverride string, portOverride string) string {
	direct := *dc
	if strings.TrimSpace(hostOverride) != "" {
		direct.Host = hostOverride
	} else if direct.Host == "pgbouncer" {
		direct.Host = "db"
	}
	if strings.TrimSpace(portOverride) != "" {
		direct.Port = portOverride
	} else if direct.Port == "6432" && direct.Host == "db" {
		direct.Port = "5432"
	}
	return direct.BuildConnectionString()
}

func getEnvOrDefault(key, defaultValue string) string {
	// Check for _FILE suffix (Docker Secrets support).
	// The path is operator-supplied via env var, not user input.
	if fileValue := os.Getenv(key + "_FILE"); fileValue != "" {
		if content, err := os.ReadFile(filepath.Clean(fileValue)); err == nil { //#nosec G304,G703 -- path from *_FILE env var (Docker Secrets)
			return strings.TrimSpace(string(content))
		}
	}
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvIntOrDefault は key を int として読む。defaultValue に落ちるのは
// 変数が未設定のときだけで、設定されているのに使えない値（非数値 / int 範囲外）は
// 起動失敗にする。黙って既定値に落とすと、起動ログは実効値しか出さないので
// compose の DB_MAX_CONNS=40 が 20 で動いていることが運用から見えない
// （CLAUDE.md rule 9: fail-fast startup config）。
func getEnvIntOrDefault(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	// Atoi は bitSize=0（= int）でパースするので、プラットフォーム依存の
	// int 範囲外は ErrRange として返ってくる。
	intValue, err := strconv.Atoi(value)
	if err != nil {
		panic(fmt.Errorf("invalid %s=%q: must be an integer in [%d, %d]: %w", key, value, math.MinInt, math.MaxInt, err))
	}
	return intValue
}

// parseEnvInt32 reads key as a base-10 int32 via strconv.ParseInt bitSize 32.
// Unset → defaultValue. Non-integer or out of int32 range → error.
// Never silent-clamps (CLAUDE.md rule 9: fail-fast startup config).
func parseEnvInt32(key string, defaultValue int32) (int32, error) {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue, nil
	}
	n, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid %s=%q: must be an integer in [%d, %d]: %w", key, value, math.MinInt32, math.MaxInt32, err)
	}
	return int32(n), nil
}

// poolConnsFromEnv loads DB_MAX_CONNS / DB_MIN_CONNS as int32 pool sizes.
// Values that fit a platform int but not int32, or that are invalid for a
// pgx pool (MaxConns <= 0, MinConns < 0), fail instead of being clamped.
func poolConnsFromEnv() (maxConns, minConns int32, err error) {
	maxConns, err = parseEnvInt32("DB_MAX_CONNS", 20)
	if err != nil {
		return 0, 0, fmt.Errorf("read DB_MAX_CONNS: %w", err)
	}
	if maxConns <= 0 {
		return 0, 0, fmt.Errorf("invalid DB_MAX_CONNS=%d: must be a positive int32", maxConns)
	}
	minConns, err = parseEnvInt32("DB_MIN_CONNS", 5)
	if err != nil {
		return 0, 0, fmt.Errorf("read DB_MIN_CONNS: %w", err)
	}
	if minConns < 0 {
		return 0, 0, fmt.Errorf("invalid DB_MIN_CONNS=%d: must be a non-negative int32", minConns)
	}
	return maxConns, minConns, nil
}
