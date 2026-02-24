package driver

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

type DatabaseSSLConfig struct {
	Mode     string
	RootCert string
	Cert     string
	Key      string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSL      DatabaseSSLConfig

	// 接続プール設定
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime string
	MaxConnIdleTime string
}

func NewDatabaseConfig() *DatabaseConfig {
	return &DatabaseConfig{
		Host:     getEnvOrDefault("DB_HOST", "localhost"),
		Port:     getEnvOrDefault("DB_PORT", "5432"),
		User:     getEnvOrDefault("PRE_PROCESSOR_DB_USER", "devuser"),
		Password: getEnvOrDefault("PRE_PROCESSOR_DB_PASSWORD", "devpassword"),
		DBName:   getEnvOrDefault("DB_NAME", "devdb"),
		SSL: DatabaseSSLConfig{
			Mode:     getEnvOrDefault("DB_SSL_MODE", "prefer"),
			RootCert: getEnvOrDefault("DB_SSL_ROOT_CERT", ""),
			Cert:     getEnvOrDefault("DB_SSL_CERT", ""),
			Key:      getEnvOrDefault("DB_SSL_KEY", ""),
		},
		MaxConns:        getEnvAsInt32OrDefault("DB_MAX_CONNS", 20),
		MinConns:        getEnvAsInt32OrDefault("DB_MIN_CONNS", 5),
		MaxConnLifetime: getEnvOrDefault("DB_MAX_CONN_LIFETIME", "1h"),
		MaxConnIdleTime: getEnvOrDefault("DB_MAX_CONN_IDLE_TIME", "30m"),
	}
}

func (dc *DatabaseConfig) BuildConnectionString() string {
	baseConn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		dc.Host, dc.Port, dc.User, dc.Password, dc.DBName, dc.SSL.Mode,
	)

	// SSL証明書設定 - モードに応じて条件付きで設定
	switch dc.SSL.Mode {
	case "verify-ca", "verify-full":
		// 証明書検証モードでは必須
		if dc.SSL.RootCert != "" {
			baseConn += fmt.Sprintf(" sslrootcert=%s", dc.SSL.RootCert)
		}
		if dc.SSL.Cert != "" {
			baseConn += fmt.Sprintf(" sslcert=%s", dc.SSL.Cert)
		}
		if dc.SSL.Key != "" {
			baseConn += fmt.Sprintf(" sslkey=%s", dc.SSL.Key)
		}
	case "require":
		// requireモードでも証明書ファイルを指定（PostgreSQLのclientcert=verify-caに対応）
		if dc.SSL.RootCert != "" {
			baseConn += fmt.Sprintf(" sslrootcert=%s", dc.SSL.RootCert)
		}
		if dc.SSL.Cert != "" {
			baseConn += fmt.Sprintf(" sslcert=%s", dc.SSL.Cert)
		}
		if dc.SSL.Key != "" {
			baseConn += fmt.Sprintf(" sslkey=%s", dc.SSL.Key)
		}
		slog.InfoContext(context.Background(), "SSL require mode: using SSL with certificate files")
	case "prefer", "allow":
		// 任意で証明書ファイルを指定
		if dc.SSL.RootCert != "" {
			baseConn += fmt.Sprintf(" sslrootcert=%s", dc.SSL.RootCert)
		}
		if dc.SSL.Cert != "" {
			baseConn += fmt.Sprintf(" sslcert=%s", dc.SSL.Cert)
		}
		if dc.SSL.Key != "" {
			baseConn += fmt.Sprintf(" sslkey=%s", dc.SSL.Key)
		}
	}

	// 接続プール設定
	poolSettings := fmt.Sprintf(
		" pool_max_conns=%d pool_min_conns=%d pool_max_conn_lifetime=%s pool_max_conn_idle_time=%s",
		dc.MaxConns, dc.MinConns, dc.MaxConnLifetime, dc.MaxConnIdleTime,
	)

	return baseConn + poolSettings
}

func (dc *DatabaseConfig) ValidateSSLConfig() error {
	ctx := context.Background()
	switch dc.SSL.Mode {
	case "disable":
		slog.WarnContext(ctx, "SSL is disabled - this is not recommended for production")
	case "allow", "prefer":
		slog.InfoContext(ctx, "SSL mode allows fallback to non-encrypted connections")
	case "require":
		slog.InfoContext(ctx, "SSL required but certificate validation disabled")
	case "verify-ca", "verify-full":
		if dc.SSL.RootCert == "" {
			return fmt.Errorf("SSL root certificate required for mode %s", dc.SSL.Mode)
		}
		// 証明書ファイルの存在確認
		if _, err := os.Stat(dc.SSL.RootCert); err != nil {
			return fmt.Errorf("SSL root certificate file not found: %s", dc.SSL.RootCert)
		}
		slog.InfoContext(ctx, "SSL with certificate validation enabled", "mode", dc.SSL.Mode, "root_cert", dc.SSL.RootCert)
	default:
		return fmt.Errorf("invalid SSL mode: %s", dc.SSL.Mode)
	}
	return nil
}

// NewDatabaseConfigWithPrefix creates a DatabaseConfig reading env vars with the given prefix.
// For example, prefix "PP_" reads PP_DB_HOST, PP_DB_PORT, PP_DB_NAME, PP_DB_USER, PP_DB_PASSWORD.
// Supports _FILE suffix for Docker Secrets (e.g. PP_DB_PASSWORD_FILE).
func NewDatabaseConfigWithPrefix(prefix string) *DatabaseConfig {
	password := getEnvOrDefault(prefix+"DB_PASSWORD", "")
	if password == "" {
		if filePath := os.Getenv(prefix + "DB_PASSWORD_FILE"); filePath != "" {
			// #nosec G304 -- filePath comes from trusted env var for Docker Secrets
			if content, err := os.ReadFile(filePath); err == nil {
				password = strings.TrimSpace(string(content))
			}
		}
	}

	return &DatabaseConfig{
		Host:     getEnvOrDefault(prefix+"DB_HOST", "localhost"),
		Port:     getEnvOrDefault(prefix+"DB_PORT", "5432"),
		User:     getEnvOrDefault(prefix+"DB_USER", "postgres"),
		Password: password,
		DBName:   getEnvOrDefault(prefix+"DB_NAME", "postgres"),
		SSL: DatabaseSSLConfig{
			Mode: "disable",
		},
		MaxConns:        getEnvAsInt32OrDefault(prefix+"DB_MAX_CONNS", 10),
		MinConns:        getEnvAsInt32OrDefault(prefix+"DB_MIN_CONNS", 2),
		MaxConnLifetime: getEnvOrDefault(prefix+"DB_MAX_CONN_LIFETIME", "1h"),
		MaxConnIdleTime: getEnvOrDefault(prefix+"DB_MAX_CONN_IDLE_TIME", "30m"),
	}
}

func getEnvAsInt32OrDefault(key string, defaultValue int32) int32 {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.ParseInt(value, 10, 32); err == nil {
			return int32(intValue)
		}
	}
	return defaultValue
}
