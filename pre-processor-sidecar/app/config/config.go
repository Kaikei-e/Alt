// ABOUTME: This file handles configuration management for pre-processor-sidecar
// ABOUTME: Loads environment variables and validates configuration for Inoreader API integration

package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the pre-processor-sidecar service
type Config struct {
	// Service configuration
	ServiceName string
	LogLevel    string

	// Database configuration
	Database DatabaseConfig

	// Inoreader API configuration
	Inoreader InoreaderConfig

	// Proxy configuration
	Proxy ProxyConfig

	// Rate limiting configuration
	RateLimit RateLimitConfig

	// Kubernetes configuration
	Kubernetes KubernetesConfig

	// OAuth2 configuration
	OAuth2 OAuth2Config
}

// DatabaseConfig holds database connection settings
type DatabaseConfig struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	SSLMode  string
}

// InoreaderConfig holds Inoreader API settings
type InoreaderConfig struct {
	BaseURL              string
	ClientID             string
	ClientSecret         string
	RefreshToken         string
	MaxArticlesPerRequest int
	TokenRefreshBuffer   time.Duration
}

// ProxyConfig holds proxy settings for Envoy integration
type ProxyConfig struct {
	HTTPSProxy string
	NoProxy    string
}

// RateLimitConfig holds rate limiting settings
type RateLimitConfig struct {
	DailyLimit   int
	SyncInterval time.Duration
}

// KubernetesConfig holds Kubernetes integration settings
type KubernetesConfig struct {
	InCluster       bool
	Namespace       string
	TokenSecretName string
}

// OAuth2Config holds OAuth2 token management settings
type OAuth2Config struct {
	ClientID        string
	ClientSecret    string
	RefreshToken    string
	RefreshBuffer   time.Duration
	TokenSecretName string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	cfg := &Config{
		ServiceName: getEnvOrDefault("SERVICE_NAME", "pre-processor-sidecar"),
		LogLevel:    getEnvOrDefault("LOG_LEVEL", "info"),

		Database: DatabaseConfig{
			Host:     getEnvOrDefault("DB_HOST", "postgres.alt-database.svc.cluster.local"),
			Port:     getEnvOrDefault("DB_PORT", "5432"),
			Name:     getEnvOrDefault("DB_NAME", "alt"),
			User:     getEnvOrDefault("PRE_PROCESSOR_SIDECAR_DB_USER", "pre_processor_sidecar_user"), // FIXED: Correct default user
			Password: os.Getenv("PRE_PROCESSOR_SIDECAR_DB_PASSWORD"), // Required from secret
			SSLMode:  getEnvOrDefault("DB_SSL_MODE", "disable"), // FIXED: Default to disable for Linkerd mTLS
		},

		Inoreader: InoreaderConfig{
			BaseURL:      getEnvOrDefault("INOREADER_BASE_URL", "https://www.inoreader.com/reader/api/0"),
			ClientID:     os.Getenv("INOREADER_CLIENT_ID"),     // Required from secret
			ClientSecret: os.Getenv("INOREADER_CLIENT_SECRET"), // Required from secret
			RefreshToken: os.Getenv("INOREADER_REFRESH_TOKEN"), // Required from secret
		},

		Proxy: ProxyConfig{
			HTTPSProxy: getEnvOrDefault("HTTPS_PROXY", "http://envoy-proxy.alt-apps.svc.cluster.local:8081"),
			NoProxy:    getEnvOrDefault("NO_PROXY", "localhost,127.0.0.1,.svc.cluster.local"),
		},

		RateLimit: RateLimitConfig{
			DailyLimit: 100, // Zone 1 limit
		},

		Kubernetes: KubernetesConfig{
			InCluster:       getEnvOrDefault("KUBERNETES_IN_CLUSTER", "false") == "true",
			Namespace:       getEnvOrDefault("KUBERNETES_NAMESPACE", "alt-processing"),
			TokenSecretName: getEnvOrDefault("OAUTH2_TOKEN_SECRET_NAME", "pre-processor-sidecar-oauth2-token"),
		},

		OAuth2: OAuth2Config{
			ClientID:        os.Getenv("INOREADER_CLIENT_ID"),     // Required from secret
			ClientSecret:    os.Getenv("INOREADER_CLIENT_SECRET"), // Required from secret
			RefreshToken:    os.Getenv("INOREADER_REFRESH_TOKEN"), // Required from secret
			TokenSecretName: getEnvOrDefault("OAUTH2_TOKEN_SECRET_NAME", "pre-processor-sidecar-oauth2-token"),
		},
	}

	// Parse integer configurations
	if maxArticles := os.Getenv("MAX_ARTICLES_PER_REQUEST"); maxArticles != "" {
		if val, err := strconv.Atoi(maxArticles); err == nil {
			cfg.Inoreader.MaxArticlesPerRequest = val
		} else {
			cfg.Inoreader.MaxArticlesPerRequest = 100 // Default
		}
	} else {
		cfg.Inoreader.MaxArticlesPerRequest = 100
	}

	// Parse duration configurations
	if syncInterval := os.Getenv("SYNC_INTERVAL"); syncInterval != "" {
		if duration, err := time.ParseDuration(syncInterval); err == nil {
			cfg.RateLimit.SyncInterval = duration
		} else {
			cfg.RateLimit.SyncInterval = 30 * time.Minute // Default
		}
	} else {
		cfg.RateLimit.SyncInterval = 30 * time.Minute
	}

	// Parse token refresh buffer for both Inoreader and OAuth2
	if buffer := os.Getenv("OAUTH2_TOKEN_REFRESH_BUFFER"); buffer != "" {
		if bufferSeconds, err := strconv.Atoi(buffer); err == nil {
			bufferDuration := time.Duration(bufferSeconds) * time.Second
			cfg.Inoreader.TokenRefreshBuffer = bufferDuration
			cfg.OAuth2.RefreshBuffer = bufferDuration
		} else {
			cfg.Inoreader.TokenRefreshBuffer = 5 * time.Minute // Default
			cfg.OAuth2.RefreshBuffer = 5 * time.Minute // Default
		}
	} else {
		cfg.Inoreader.TokenRefreshBuffer = 5 * time.Minute
		cfg.OAuth2.RefreshBuffer = 5 * time.Minute
	}

	// Validate required configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Database.Password == "" {
		return fmt.Errorf("PRE_PROCESSOR_SIDECAR_DB_PASSWORD is required")
	}

	if c.Inoreader.ClientID == "" {
		return fmt.Errorf("INOREADER_CLIENT_ID is required")
	}

	if c.Inoreader.ClientSecret == "" {
		return fmt.Errorf("INOREADER_CLIENT_SECRET is required")
	}

	if c.Inoreader.RefreshToken == "" {
		return fmt.Errorf("INOREADER_REFRESH_TOKEN is required")
	}

	if c.Proxy.HTTPSProxy == "" {
		return fmt.Errorf("HTTPS_PROXY is required for Envoy integration")
	}

	return nil
}

// getEnvOrDefault returns environment variable value or default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}