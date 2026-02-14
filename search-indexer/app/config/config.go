package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

type Config struct {
	Database    DatabaseConfig
	Meilisearch MeilisearchConfig
	Indexer     IndexerConfig
	HTTP        HTTPConfig
	BackendAPI  BackendAPIConfig
}

// BackendAPIConfig holds configuration for connecting to alt-backend's internal API.
// When URL is set, the search-indexer uses the API instead of direct DB access.
type BackendAPIConfig struct {
	// URL is the Connect-RPC URL for alt-backend's internal API.
	URL string
	// ServiceToken is the shared secret for service authentication.
	ServiceToken string
}

// Enhanced DatabaseConfig with SSL support
type DatabaseConfig struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	Timeout  time.Duration
	SSL      SSLConfig
}

type MeilisearchConfig struct {
	Host    string
	APIKey  string
	Timeout time.Duration
}

type IndexerConfig struct {
	Interval     time.Duration
	BatchSize    int
	RetryDelay   time.Duration
	MaxRetries   int
	RetryTimeout time.Duration
}

type HTTPConfig struct {
	Addr              string
	ReadHeaderTimeout time.Duration
}

func Load() (*Config, error) {
	ctx := context.Background()

	// Load Backend API configuration (when set, DB config is not required)
	backendAPIURL := getEnvOrDefault("BACKEND_API_URL", "")
	backendAPIConfig := BackendAPIConfig{
		URL:          backendAPIURL,
		ServiceToken: getEnvOrDefault("SERVICE_TOKEN", ""),
	}

	cfg := &Config{
		BackendAPI: backendAPIConfig,
		Meilisearch: MeilisearchConfig{
			Host:    getEnvOrDefault("MEILISEARCH_HOST", ""),
			APIKey:  getEnvOrDefault("MEILISEARCH_API_KEY", ""),
			Timeout: 15 * time.Second,
		},
		Indexer: IndexerConfig{
			Interval:     1 * time.Minute,
			BatchSize:    200,
			RetryDelay:   1 * time.Minute,
			MaxRetries:   5,
			RetryTimeout: 1 * time.Minute,
		},
		HTTP: HTTPConfig{
			Addr:              ":9300",
			ReadHeaderTimeout: 5 * time.Second,
		},
	}

	// Validate Meilisearch config (always required)
	if cfg.Meilisearch.Host == "" {
		return nil, fmt.Errorf("meilisearch configuration error: required environment variable MEILISEARCH_HOST is not set")
	}

	// When BACKEND_API_URL is set, skip DB configuration entirely
	if backendAPIURL != "" {
		slog.InfoContext(ctx, "configuration loaded (API mode)",
			"backend_api_url", backendAPIURL,
			"meilisearch_host", cfg.Meilisearch.Host,
		)
		return cfg, nil
	}

	// Load required database environment variables (legacy DB mode)
	dbHost, err := getEnvRequired("DB_HOST")
	if err != nil {
		return nil, fmt.Errorf("database configuration error: %w", err)
	}
	dbPort, err := getEnvRequired("DB_PORT")
	if err != nil {
		return nil, fmt.Errorf("database configuration error: %w", err)
	}
	dbName, err := getEnvRequired("DB_NAME")
	if err != nil {
		return nil, fmt.Errorf("database configuration error: %w", err)
	}
	dbUser, err := getEnvRequired("SEARCH_INDEXER_DB_USER")
	if err != nil {
		return nil, fmt.Errorf("database configuration error: %w", err)
	}
	dbPassword, err := getEnvRequired("SEARCH_INDEXER_DB_PASSWORD")
	if err != nil {
		return nil, fmt.Errorf("database configuration error: %w", err)
	}

	// Create database config with SSL support
	dbConfig := &DatabaseConfig{
		Host:     dbHost,
		Port:     dbPort,
		Name:     dbName,
		User:     dbUser,
		Password: dbPassword,
		Timeout:  10 * time.Second,
		SSL: SSLConfig{
			Mode:     getEnvOrDefault("DB_SSL_MODE", "prefer"),
			RootCert: getEnvOrDefault("DB_SSL_ROOT_CERT", ""),
			Cert:     getEnvOrDefault("DB_SSL_CERT", ""),
			Key:      getEnvOrDefault("DB_SSL_KEY", ""),
		},
	}

	// SSL設定の検証
	if err := dbConfig.ValidateSSLConfig(); err != nil {
		slog.ErrorContext(ctx, "invalid SSL configuration", "error", err)
		return nil, fmt.Errorf("SSL configuration error: %w", err)
	}

	cfg.Database = *dbConfig

	slog.InfoContext(ctx, "configuration loaded (DB mode)",
		"db_host", cfg.Database.Host,
		"db_sslmode", cfg.Database.SSL.Mode,
		"meilisearch_host", cfg.Meilisearch.Host,
	)

	return cfg, nil
}

// UseBackendAPI returns true when the search-indexer should use the backend API
// instead of direct database access.
func (c *Config) UseBackendAPI() bool {
	return c.BackendAPI.URL != ""
}

// 後方互換性のためのメソッド（deprecated）
func (c *DatabaseConfig) ConnectionString() string {
	slog.WarnContext(context.Background(), "ConnectionString is deprecated, use GetDatabaseConnectionString()")
	return c.GetDatabaseConnectionString()
}

// 新しいメソッド
func (c *DatabaseConfig) GetDatabaseConnectionString() string {
	baseConn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSL.Mode,
	)

	if c.SSL.RootCert != "" {
		baseConn += fmt.Sprintf(" sslrootcert=%s", c.SSL.RootCert)
	}
	if c.SSL.Cert != "" {
		baseConn += fmt.Sprintf(" sslcert=%s", c.SSL.Cert)
	}
	if c.SSL.Key != "" {
		baseConn += fmt.Sprintf(" sslkey=%s", c.SSL.Key)
	}

	return baseConn
}

func (c *DatabaseConfig) GetDatabaseURL() string {
	baseURL := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s",
		c.User, c.Password, c.Host, c.Port, c.Name,
	)

	// SSLパラメータをクエリ文字列として追加
	params := fmt.Sprintf("?sslmode=%s", c.SSL.Mode)

	if c.SSL.RootCert != "" {
		params += fmt.Sprintf("&sslrootcert=%s", c.SSL.RootCert)
	}
	if c.SSL.Cert != "" {
		params += fmt.Sprintf("&sslcert=%s", c.SSL.Cert)
	}
	if c.SSL.Key != "" {
		params += fmt.Sprintf("&sslkey=%s", c.SSL.Key)
	}

	return baseURL + params
}

func (c *DatabaseConfig) ValidateSSLConfig() error {
	switch c.SSL.Mode {
	case "disable":
		return fmt.Errorf("SSL disable mode is not allowed")
	case "allow", "prefer":
		// 警告はログに出力（ここでは省略）
		return nil
	case "require":
		return nil
	case "verify-ca", "verify-full":
		if c.SSL.RootCert == "" {
			return fmt.Errorf("SSL root certificate required for mode %s", c.SSL.Mode)
		}
		return nil
	default:
		return fmt.Errorf("invalid SSL mode: %s", c.SSL.Mode)
	}
}

func getEnvRequired(key string) (string, error) {
	// Check for _FILE suffix
	if fileValue := os.Getenv(key + "_FILE"); fileValue != "" {
		content, err := os.ReadFile(fileValue)
		if err == nil {
			return strings.TrimSpace(string(content)), nil
		}
	}

	value := os.Getenv(key)
	if value == "" {
		return "", fmt.Errorf("required environment variable %s is not set", key)
	}
	return value, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	// Check for _FILE suffix
	if fileValue := os.Getenv(key + "_FILE"); fileValue != "" {
		content, err := os.ReadFile(fileValue)
		if err == nil {
			return strings.TrimSpace(string(content))
		}
	}

	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
