// Package config provides configuration management for alt-butterfly-facade.
package config

import (
	"errors"
	"os"
	"strings"
	"time"
)

// Config holds the configuration for the BFF service.
type Config struct {
	// Port is the port number for the BFF service
	Port string
	// BackendConnectURL is the URL of the alt-backend Connect-RPC service
	BackendConnectURL string
	// AuthHubURL is the URL of the auth-hub service
	AuthHubURL string
	// BackendTokenSecretFile is the path to the backend token secret file
	BackendTokenSecretFile string
	// BackendTokenSecret is the backend token secret (alternative to file)
	BackendTokenSecret string
	// BackendTokenIssuer is the expected issuer of backend tokens
	BackendTokenIssuer string
	// BackendTokenAudience is the expected audience of backend tokens
	BackendTokenAudience string
	// RequestTimeout is the timeout for unary RPC requests
	RequestTimeout time.Duration
	// StreamingTimeout is the timeout for streaming RPC requests
	StreamingTimeout time.Duration
}

// NewConfig creates a new Config from environment variables with defaults.
func NewConfig() *Config {
	return &Config{
		Port:                   getEnv("BFF_PORT", "9200"),
		BackendConnectURL:      getEnv("BACKEND_CONNECT_URL", "http://alt-backend:9101"),
		AuthHubURL:             getEnv("AUTH_HUB_INTERNAL_URL", "http://auth-hub:8888"),
		BackendTokenSecretFile: getEnv("BACKEND_TOKEN_SECRET_FILE", ""),
		BackendTokenSecret:     getEnv("BACKEND_TOKEN_SECRET", ""),
		BackendTokenIssuer:     getEnv("BACKEND_TOKEN_ISSUER", "auth-hub"),
		BackendTokenAudience:   getEnv("BACKEND_TOKEN_AUDIENCE", "alt-backend"),
		RequestTimeout:         getDurationEnv("BFF_REQUEST_TIMEOUT", 30*time.Second),
		StreamingTimeout:       getDurationEnv("BFF_STREAMING_TIMEOUT", 5*time.Minute),
	}
}

// LoadBackendTokenSecret loads the backend token secret from file or environment.
func (c *Config) LoadBackendTokenSecret() ([]byte, error) {
	// Try to load from file first
	if c.BackendTokenSecretFile != "" {
		data, err := os.ReadFile(c.BackendTokenSecretFile)
		if err != nil {
			return nil, err
		}
		return []byte(strings.TrimSpace(string(data))), nil
	}

	// Fall back to environment variable
	if c.BackendTokenSecret != "" {
		return []byte(c.BackendTokenSecret), nil
	}

	return nil, errors.New("backend token secret not configured: set BACKEND_TOKEN_SECRET_FILE or BACKEND_TOKEN_SECRET")
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Port == "" {
		return errors.New("BFF_PORT is required")
	}
	if c.BackendConnectURL == "" {
		return errors.New("BACKEND_CONNECT_URL is required")
	}
	return nil
}

// getEnv returns the value of an environment variable or a default value.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getDurationEnv returns the value of an environment variable as a duration or a default value.
func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
