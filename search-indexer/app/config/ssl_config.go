package config

import (
	"fmt"
)

type SSLConfig struct {
	Mode     string
	RootCert string
	Cert     string
	Key      string
}

func NewDatabaseConfigFromEnv() *DatabaseConfig {
	return &DatabaseConfig{
		Host:     getEnvOrDefault("DB_HOST", "localhost"),
		Port:     getEnvOrDefault("DB_PORT", "5432"),
		Name:     getEnvOrDefault("DB_NAME", "devdb"),
		User:     getEnvOrDefault("DB_USER", "devuser"),
		Password: getEnvOrDefault("DB_PASSWORD", "devpassword"),
		SSL: SSLConfig{
			Mode:     getEnvOrDefault("DB_SSL_MODE", "prefer"),
			RootCert: getEnvOrDefault("DB_SSL_ROOT_CERT", ""),
			Cert:     getEnvOrDefault("DB_SSL_CERT", ""),
			Key:      getEnvOrDefault("DB_SSL_KEY", ""),
		},
	}
}

// PostgreSQL接続文字列生成（pgx形式）
func (dc *DatabaseConfig) BuildPgxConnectionString() string {
	baseConn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		dc.Host, dc.Port, dc.User, dc.Password, dc.Name, dc.SSL.Mode,
	)

	if dc.SSL.RootCert != "" {
		baseConn += fmt.Sprintf(" sslrootcert=%s", dc.SSL.RootCert)
	}
	if dc.SSL.Cert != "" {
		baseConn += fmt.Sprintf(" sslcert=%s", dc.SSL.Cert)
	}
	if dc.SSL.Key != "" {
		baseConn += fmt.Sprintf(" sslkey=%s", dc.SSL.Key)
	}

	return baseConn
}

// PostgreSQL接続URL生成（URL形式）
func (dc *DatabaseConfig) BuildPostgresURL() string {
	baseURL := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s",
		dc.User, dc.Password, dc.Host, dc.Port, dc.Name,
	)

	// SSLパラメータをクエリ文字列として追加
	params := fmt.Sprintf("?sslmode=%s", dc.SSL.Mode)

	if dc.SSL.RootCert != "" {
		params += fmt.Sprintf("&sslrootcert=%s", dc.SSL.RootCert)
	}
	if dc.SSL.Cert != "" {
		params += fmt.Sprintf("&sslcert=%s", dc.SSL.Cert)
	}
	if dc.SSL.Key != "" {
		params += fmt.Sprintf("&sslkey=%s", dc.SSL.Key)
	}

	return baseURL + params
}