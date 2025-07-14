package alt_db

import (
	"fmt"
	"log/slog"
	"os"
)

type SSLConfig struct {
	Mode     string // disable, allow, prefer, require, verify-ca, verify-full
	RootCert string // CA証明書のパス
	Cert     string // クライアント証明書のパス
	Key      string // クライアント秘密鍵のパス
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSL      SSLConfig
}

func NewDatabaseConfigFromEnv() *DatabaseConfig {
	return &DatabaseConfig{
		Host:     getEnvOrDefault("DB_HOST", "localhost"),
		Port:     getEnvOrDefault("DB_PORT", "5432"),
		User:     getEnvOrDefault("DB_USER", "devuser"),
		Password: getEnvOrDefault("DB_PASSWORD", "devpassword"),
		DBName:   getEnvOrDefault("DB_NAME", "devdb"),
		SSL: SSLConfig{
			Mode:     getEnvOrDefault("DB_SSL_MODE", "prefer"),
			RootCert: getEnvOrDefault("DB_SSL_ROOT_CERT", ""),
			Cert:     getEnvOrDefault("DB_SSL_CERT", ""),
			Key:      getEnvOrDefault("DB_SSL_KEY", ""),
		},
	}
}

func (dc *DatabaseConfig) BuildConnectionString() string {
	baseConn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		dc.Host, dc.Port, dc.User, dc.Password, dc.DBName, dc.SSL.Mode,
	)

	// SSL証明書パスが指定されている場合は追加
	if dc.SSL.RootCert != "" {
		baseConn += fmt.Sprintf(" sslrootcert=%s", dc.SSL.RootCert)
	}
	if dc.SSL.Cert != "" {
		baseConn += fmt.Sprintf(" sslcert=%s", dc.SSL.Cert)
	}
	if dc.SSL.Key != "" {
		baseConn += fmt.Sprintf(" sslkey=%s", dc.SSL.Key)
	}

	return baseConn + " search_path=public pool_max_conns=20 pool_min_conns=5"
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

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}