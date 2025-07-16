package domain

import (
	"fmt"
)

// SecretType represents the type of secret
type SecretType string

const (
	DatabaseSecret SecretType = "database"
	SSLSecret      SecretType = "ssl"
	APISecret      SecretType = "api"
)

// Secret represents a Kubernetes secret
type Secret struct {
	Name      string
	Namespace string
	Type      SecretType
	Data      map[string]string
}

// NewSecret creates a new secret
func NewSecret(name, namespace string, secretType SecretType) *Secret {
	return &Secret{
		Name:      name,
		Namespace: namespace,
		Type:      secretType,
		Data:      make(map[string]string),
	}
}

// AddData adds data to the secret
func (s *Secret) AddData(key, value string) {
	s.Data[key] = value
}

// GetData returns the data for the given key
func (s *Secret) GetData(key string) (string, bool) {
	value, exists := s.Data[key]
	return value, exists
}

// DatabaseSecretConfig represents database secret configuration
type DatabaseSecretConfig struct {
	Name         string
	Username     string
	Password     string
	KeyName      string
	Namespace    string
}

// NewDatabaseSecretConfig creates a new database secret configuration
func NewDatabaseSecretConfig(name, username, password, keyName, namespace string) *DatabaseSecretConfig {
	return &DatabaseSecretConfig{
		Name:      name,
		Username:  username,
		Password:  password,
		KeyName:   keyName,
		Namespace: namespace,
	}
}

// GetDefaultDatabaseConfigs returns default database secret configurations
func GetDefaultDatabaseConfigs() []DatabaseSecretConfig {
	return []DatabaseSecretConfig{
		{
			Name:      "postgres-secrets",
			Username:  "alt_db_user",
			Password:  "ProductionPassword123",
			KeyName:   "postgres-password",
			Namespace: "alt-database",
		},
		{
			Name:      "auth-postgres-secrets",
			Username:  "auth_db_user",
			Password:  "AuthProdPassword456",
			KeyName:   "postgres-password",
			Namespace: "alt-database",
		},
		{
			Name:      "kratos-postgres-secrets",
			Username:  "kratos_db_user",
			Password:  "KratosProdPassword789",
			KeyName:   "postgres-password",
			Namespace: "alt-database",
		},
		{
			Name:      "clickhouse-secrets",
			Username:  "clickhouse_user",
			Password:  "ClickHouseProdPassword012",
			KeyName:   "password",
			Namespace: "alt-database",
		},
	}
}

// GetMeiliSearchSecretConfig returns MeiliSearch secret configuration
func GetMeiliSearchSecretConfig() DatabaseSecretConfig {
	return DatabaseSecretConfig{
		Name:      "meilisearch-secrets",
		Username:  "",
		Password:  "",
		KeyName:   "master-key",
		Namespace: "alt-search",
	}
}

// SSLSecretConfig represents SSL secret configuration
type SSLSecretConfig struct {
	Name       string
	Namespace  string
	CertFile   string
	KeyFile    string
	CAFile     string
	ServiceName string
}

// NewSSLSecretConfig creates a new SSL secret configuration
func NewSSLSecretConfig(serviceName, namespace, certFile, keyFile, caFile string) *SSLSecretConfig {
	return &SSLSecretConfig{
		Name:        fmt.Sprintf("%s-ssl-certs-prod", serviceName),
		Namespace:   namespace,
		CertFile:    certFile,
		KeyFile:     keyFile,
		CAFile:      caFile,
		ServiceName: serviceName,
	}
}

// GetDefaultSSLConfigs returns default SSL secret configurations
func GetDefaultSSLConfigs(sslDir string) []SSLSecretConfig {
	return []SSLSecretConfig{
		{
			Name:        "postgres-ssl-certs-prod",
			Namespace:   "alt-database",
			CertFile:    fmt.Sprintf("%s/postgres.crt", sslDir),
			KeyFile:     fmt.Sprintf("%s/postgres.key", sslDir),
			CAFile:      fmt.Sprintf("%s/ca.crt", sslDir),
			ServiceName: "postgres",
		},
		{
			Name:        "auth-postgres-ssl-certs-prod",
			Namespace:   "alt-database",
			CertFile:    fmt.Sprintf("%s/auth-postgres.crt", sslDir),
			KeyFile:     fmt.Sprintf("%s/auth-postgres.key", sslDir),
			CAFile:      fmt.Sprintf("%s/ca.crt", sslDir),
			ServiceName: "auth-postgres",
		},
		{
			Name:        "kratos-postgres-ssl-certs-prod",
			Namespace:   "alt-database",
			CertFile:    fmt.Sprintf("%s/kratos-postgres.crt", sslDir),
			KeyFile:     fmt.Sprintf("%s/kratos-postgres.key", sslDir),
			CAFile:      fmt.Sprintf("%s/ca.crt", sslDir),
			ServiceName: "kratos-postgres",
		},
		{
			Name:        "clickhouse-ssl-certs-prod",
			Namespace:   "alt-database",
			CertFile:    fmt.Sprintf("%s/clickhouse.crt", sslDir),
			KeyFile:     fmt.Sprintf("%s/clickhouse.key", sslDir),
			CAFile:      fmt.Sprintf("%s/ca.crt", sslDir),
			ServiceName: "clickhouse",
		},
		{
			Name:        "meilisearch-ssl-certs-prod",
			Namespace:   "alt-search",
			CertFile:    fmt.Sprintf("%s/meilisearch.crt", sslDir),
			KeyFile:     fmt.Sprintf("%s/meilisearch.key", sslDir),
			CAFile:      fmt.Sprintf("%s/ca.crt", sslDir),
			ServiceName: "meilisearch",
		},
		{
			Name:        "alt-backend-ssl-certs-prod",
			Namespace:   "alt-apps",
			CertFile:    fmt.Sprintf("%s/alt-backend.crt", sslDir),
			KeyFile:     fmt.Sprintf("%s/alt-backend.key", sslDir),
			CAFile:      fmt.Sprintf("%s/ca.crt", sslDir),
			ServiceName: "alt-backend",
		},
		{
			Name:        "alt-frontend-ssl-certs-prod",
			Namespace:   "alt-apps",
			CertFile:    fmt.Sprintf("%s/alt-frontend.crt", sslDir),
			KeyFile:     fmt.Sprintf("%s/alt-frontend.key", sslDir),
			CAFile:      fmt.Sprintf("%s/ca.crt", sslDir),
			ServiceName: "alt-frontend",
		},
		{
			Name:        "kratos-ssl-certs-prod",
			Namespace:   "alt-auth",
			CertFile:    fmt.Sprintf("%s/kratos.crt", sslDir),
			KeyFile:     fmt.Sprintf("%s/kratos.key", sslDir),
			CAFile:      fmt.Sprintf("%s/ca.crt", sslDir),
			ServiceName: "kratos",
		},
		{
			Name:        "nginx-ssl-certs-prod",
			Namespace:   "alt-ingress",
			CertFile:    fmt.Sprintf("%s/nginx.crt", sslDir),
			KeyFile:     fmt.Sprintf("%s/nginx.key", sslDir),
			CAFile:      fmt.Sprintf("%s/ca.crt", sslDir),
			ServiceName: "nginx",
		},
	}
}