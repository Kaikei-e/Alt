package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds the application configuration
type Config struct {
	KratosURL            string        // Kratos internal URL (Frontend API - port 4433)
	KratosAdminURL       string        // Kratos Admin API URL (port 4434)
	Port                 string        // Service port
	CacheTTL             time.Duration // Session cache TTL
	CSRFSecret           string        // CSRF secret for token generation
	AuthSharedSecret     string        // Shared secret for backend authentication
	BackendTokenSecret   string        // Secret for signing backend JWT tokens
	BackendTokenIssuer   string        // JWT issuer claim
	BackendTokenAudience string        // JWT audience claim
	BackendTokenTTL      time.Duration // JWT token TTL
}

// Load reads configuration from environment variables with sensible defaults
func Load() (*Config, error) {
	config := &Config{
		KratosURL:            getEnv("KRATOS_URL", "http://kratos:4433"),
		KratosAdminURL:       getEnv("KRATOS_ADMIN_URL", "http://kratos:4434"),
		Port:                 getEnv("PORT", "8888"),
		CacheTTL:             5 * time.Minute, // Default 5 minutes
		CSRFSecret:           getEnv("CSRF_SECRET", ""),
		AuthSharedSecret:     getEnv("AUTH_SHARED_SECRET", ""),
		BackendTokenSecret:   getEnv("BACKEND_TOKEN_SECRET", ""),
		BackendTokenIssuer:   getEnv("BACKEND_TOKEN_ISSUER", "auth-hub"),
		BackendTokenAudience: getEnv("BACKEND_TOKEN_AUDIENCE", "alt-backend"),
		BackendTokenTTL:      5 * time.Minute, // Default 5 minutes
	}

	// Parse CACHE_TTL if provided
	if cacheTTLStr := os.Getenv("CACHE_TTL"); cacheTTLStr != "" {
		duration, err := time.ParseDuration(cacheTTLStr)
		if err != nil {
			return nil, fmt.Errorf("invalid CACHE_TTL format: %w", err)
		}
		config.CacheTTL = duration
	}

	// Parse BACKEND_TOKEN_TTL if provided
	if ttlStr := os.Getenv("BACKEND_TOKEN_TTL"); ttlStr != "" {
		duration, err := time.ParseDuration(ttlStr)
		if err != nil {
			return nil, fmt.Errorf("invalid BACKEND_TOKEN_TTL format: %w", err)
		}
		config.BackendTokenTTL = duration
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.KratosURL == "" {
		return fmt.Errorf("KRATOS_URL cannot be empty")
	}

	if c.Port == "" {
		return fmt.Errorf("PORT cannot be empty")
	}

	if c.CacheTTL <= 0 {
		return fmt.Errorf("CACHE_TTL must be positive")
	}

	return nil
}

// getEnv retrieves an environment variable or returns a fallback value
func getEnv(key, fallback string) string {
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
	return fallback
}
