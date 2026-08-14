package config

import (
	"fmt"
	"os"
	"strconv"
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
	BackendTokenSecret   string        // Secret for signing backend JWT tokens
	BackendTokenIssuer   string        // JWT issuer claim
	BackendTokenAudience string        // JWT audience claim
	BackendTokenTTL      time.Duration // JWT token TTL
	InternalAuthSecret   string        // Shared bearer the /internal group compares X-Internal-Auth against
	ValidateRateLimit    float64       // Validate endpoint: requests per second (default: 100/60 ≈ 1.67)
	SessionRateLimit     float64       // Session endpoint: requests per second (default: 30/60 = 0.5)
	CSRFRateLimit        float64       // CSRF endpoint: requests per second (default: 100)
}

// Load reads configuration from environment variables with sensible defaults
func Load() (*Config, error) {
	config := &Config{
		KratosURL:            getEnv("KRATOS_URL", "http://kratos:4433"),
		KratosAdminURL:       getEnv("KRATOS_ADMIN_URL", "http://kratos:4434"),
		Port:                 getEnv("PORT", "8888"),
		CacheTTL:             5 * time.Minute, // Default 5 minutes
		CSRFSecret:           getEnv("CSRF_SECRET", ""),
		BackendTokenSecret:   getEnv("BACKEND_TOKEN_SECRET", ""),
		BackendTokenIssuer:   getEnv("BACKEND_TOKEN_ISSUER", "auth-hub"),
		BackendTokenAudience: getEnv("BACKEND_TOKEN_AUDIENCE", "alt-backend"),
		BackendTokenTTL:      5 * time.Minute, // Default 5 minutes
		InternalAuthSecret:   getEnv("INTERNAL_AUTH_SECRET", ""),
		ValidateRateLimit:    100.0 / 60.0, // Default: ~1.67 req/s (100 req/min)
		SessionRateLimit:     30.0 / 60.0,  // Default: 0.5 req/s (30 req/min)
		CSRFRateLimit:        100.0,        // Default: 100 req/s
	}

	// Parse CACHE_TTL if provided
	if cacheTTLStr := os.Getenv("CACHE_TTL"); cacheTTLStr != "" {
		duration, err := time.ParseDuration(cacheTTLStr)
		if err != nil {
			return nil, fmt.Errorf("invalid CACHE_TTL format: %w", err)
		}
		config.CacheTTL = duration
	}

	// Parse VALIDATE_RATE_LIMIT if provided (requests per second)
	if v := os.Getenv("VALIDATE_RATE_LIMIT"); v != "" {
		r, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid VALIDATE_RATE_LIMIT: %w", err)
		}
		config.ValidateRateLimit = r
	}

	// Parse SESSION_RATE_LIMIT if provided (requests per second)
	if v := os.Getenv("SESSION_RATE_LIMIT"); v != "" {
		r, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid SESSION_RATE_LIMIT: %w", err)
		}
		config.SessionRateLimit = r
	}

	// Parse CSRF_RATE_LIMIT if provided (requests per second)
	if v := os.Getenv("CSRF_RATE_LIMIT"); v != "" {
		r, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid CSRF_RATE_LIMIT: %w", err)
		}
		config.CSRFRateLimit = r
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

	// CSRF_SECRET is required for security - no fallback to hardcoded values
	if c.CSRFSecret == "" {
		return fmt.Errorf("CSRF_SECRET is required")
	}
	if len(c.CSRFSecret) < 32 {
		return fmt.Errorf("CSRF_SECRET must be at least 32 characters")
	}

	if c.BackendTokenSecret == "" {
		return fmt.Errorf("BACKEND_TOKEN_SECRET is required")
	}
	if len(c.BackendTokenSecret) < 32 {
		return fmt.Errorf("BACKEND_TOKEN_SECRET must be at least 32 characters")
	}

	// INTERNAL_AUTH_SECRET is the shared bearer the /internal group compares
	// X-Internal-Auth against. It is a separate secret from the HS256 signing
	// key on purpose: alt-backend sends this value in a plaintext header on
	// every GetSystemUser call, so it lands in nginx access logs and OTel span
	// attributes. The signing key must never reach that wire, and reusing one
	// value for both jobs is what put it there. No default and no fallback to
	// BACKEND_TOKEN_SECRET — an unset value exits non-zero (CLAUDE.md rule 9).
	if c.InternalAuthSecret == "" {
		return fmt.Errorf("INTERNAL_AUTH_SECRET is required")
	}
	if len(c.InternalAuthSecret) < 32 {
		return fmt.Errorf("INTERNAL_AUTH_SECRET must be at least 32 characters")
	}
	if c.InternalAuthSecret == c.BackendTokenSecret {
		return fmt.Errorf("INTERNAL_AUTH_SECRET must not equal BACKEND_TOKEN_SECRET: " +
			"the JWT signing key would travel in plaintext X-Internal-Auth headers")
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
