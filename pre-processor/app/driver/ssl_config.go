package driver

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
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
	MaxConns        int
	MinConns        int
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
		MaxConns:        getEnvAsIntOrDefault("DB_MAX_CONNS", 20),
		MinConns:        getEnvAsIntOrDefault("DB_MIN_CONNS", 5),
		MaxConnLifetime: getEnvOrDefault("DB_MAX_CONN_LIFETIME", "1h"),
		MaxConnIdleTime: getEnvOrDefault("DB_MAX_CONN_IDLE_TIME", "30m"),
	}
}

func (dc *DatabaseConfig) BuildConnectionString() string {
	baseConn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		dc.Host, dc.Port, dc.User, dc.Password, dc.DBName, dc.SSL.Mode,
	)

	// SSL証明書設定
	if dc.SSL.RootCert != "" {
		baseConn += fmt.Sprintf(" sslrootcert=%s", dc.SSL.RootCert)
	}
	if dc.SSL.Cert != "" {
		baseConn += fmt.Sprintf(" sslcert=%s", dc.SSL.Cert)
	}
	if dc.SSL.Key != "" {
		baseConn += fmt.Sprintf(" sslkey=%s", dc.SSL.Key)
	}

	// 接続プール設定
	poolSettings := fmt.Sprintf(
		" pool_max_conns=%d pool_min_conns=%d pool_max_conn_lifetime=%s pool_max_conn_idle_time=%s",
		dc.MaxConns, dc.MinConns, dc.MaxConnLifetime, dc.MaxConnIdleTime,
	)

	return baseConn + poolSettings
}

func (dc *DatabaseConfig) ValidateSSLConfig() error {
	switch dc.SSL.Mode {
	case "disable":
		slog.Warn("SSL is disabled - this is not recommended for production")
	case "allow", "prefer":
		slog.Info("SSL mode allows fallback to non-encrypted connections")
	case "require":
		slog.Info("SSL required but certificate validation disabled")
	case "verify-ca", "verify-full":
		if dc.SSL.RootCert == "" {
			return fmt.Errorf("SSL root certificate required for mode %s", dc.SSL.Mode)
		}
		slog.Info("SSL with certificate validation enabled")
	default:
		return fmt.Errorf("invalid SSL mode: %s", dc.SSL.Mode)
	}
	return nil
}

func getEnvAsIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}