package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration for the auth service
type Config struct {
	// Server
	Port     string `env:"PORT" default:"9500"`
	Host     string `env:"HOST" default:"0.0.0.0"`
	LogLevel string `env:"LOG_LEVEL" default:"info"`

	// Database
	DatabaseURL      string `env:"DATABASE_URL" required:"true"`
	DatabaseHost     string `env:"DB_HOST" default:"auth-postgres"`
	DatabasePort     string `env:"DB_PORT" default:"5432"`
	DatabaseName     string `env:"DB_NAME" default:"auth_db"`
	DatabaseUser     string `env:"DB_USER" default:"auth_user"`
	DatabasePassword string `env:"DB_PASSWORD" required:"true"`
	DatabaseSSLMode  string `env:"DB_SSL_MODE" default:"require"`

	// Kratos
	KratosPublicURL string `env:"KRATOS_PUBLIC_URL" required:"true"`
	KratosAdminURL  string `env:"KRATOS_ADMIN_URL" required:"true"`

	// CSRF
	CSRFTokenLength int           `env:"CSRF_TOKEN_LENGTH" default:"32"`
	SessionTimeout  time.Duration `env:"SESSION_TIMEOUT" default:"24h"`

	// Features
	EnableAuditLog bool `env:"ENABLE_AUDIT_LOG" default:"true"`
	EnableMetrics  bool `env:"ENABLE_METRICS" default:"true"`
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	config := &Config{}

	// Server configuration
	config.Port = getEnvOrDefault("PORT", "9500")
	config.Host = getEnvOrDefault("HOST", "0.0.0.0")
	config.LogLevel = getEnvOrDefault("LOG_LEVEL", "info")

	// Database configuration
	config.DatabaseURL = os.Getenv("DATABASE_URL")
	if config.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	config.DatabaseHost = getEnvOrDefault("DB_HOST", "auth-postgres")
	config.DatabasePort = getEnvOrDefault("DB_PORT", "5432")
	config.DatabaseName = getEnvOrDefault("DB_NAME", "auth_db")
	config.DatabaseUser = getEnvOrDefault("DB_USER", "auth_user")
	config.DatabasePassword = os.Getenv("DB_PASSWORD")
	if config.DatabasePassword == "" {
		return nil, fmt.Errorf("DB_PASSWORD is required")
	}
	config.DatabaseSSLMode = getEnvOrDefault("DB_SSL_MODE", "require")

	// Kratos configuration
	config.KratosPublicURL = os.Getenv("KRATOS_PUBLIC_URL")
	if config.KratosPublicURL == "" {
		return nil, fmt.Errorf("KRATOS_PUBLIC_URL is required")
	}

	config.KratosAdminURL = os.Getenv("KRATOS_ADMIN_URL")
	if config.KratosAdminURL == "" {
		return nil, fmt.Errorf("KRATOS_ADMIN_URL is required")
	}

	// CSRF configuration
	var err error
	csrfLengthStr := getEnvOrDefault("CSRF_TOKEN_LENGTH", "32")
	csrfLength, err := strconv.ParseInt(csrfLengthStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid CSRF_TOKEN_LENGTH: %w", err)
	}
	// Check bounds for int type (platform-dependent)
	const maxInt64 = 1<<63 - 1
	const minInt64 = -1 << 63
	if csrfLength > maxInt64 || csrfLength < minInt64 {
		return nil, fmt.Errorf("CSRF_TOKEN_LENGTH out of range: %s (max: %d, min: %d)", csrfLengthStr, maxInt64, minInt64)
	}
	config.CSRFTokenLength = int(csrfLength)

	sessionTimeoutStr := getEnvOrDefault("SESSION_TIMEOUT", "24h")
	config.SessionTimeout, err = time.ParseDuration(sessionTimeoutStr)
	if err != nil {
		return nil, fmt.Errorf("invalid SESSION_TIMEOUT: %w", err)
	}

	// Feature flags
	config.EnableAuditLog = getBoolEnv("ENABLE_AUDIT_LOG", true)
	config.EnableMetrics = getBoolEnv("ENABLE_METRICS", true)

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate port
	port, err := strconv.ParseInt(c.Port, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid port: %s", c.Port)
	}
	// Check bounds for int type (platform-dependent)
	const maxInt64 = 1<<63 - 1
	const minInt64 = -1 << 63
	if port > maxInt64 || port < minInt64 {
		return fmt.Errorf("port out of range: %s (max: %d, min: %d)", c.Port, maxInt64, minInt64)
	}
	// Check port range (1-65535)
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535: %s", c.Port)
	}

	// Validate log level
	validLogLevels := []string{"debug", "info", "warn", "error"}
	if !contains(validLogLevels, strings.ToLower(c.LogLevel)) {
		return fmt.Errorf("invalid log level: %s (must be one of: %s)", c.LogLevel, strings.Join(validLogLevels, ", "))
	}

	// Validate CSRF token length (minimum 16 for security)
	if c.CSRFTokenLength < 16 {
		return fmt.Errorf("CSRF token length must be at least 16, got: %d", c.CSRFTokenLength)
	}

	// Validate session timeout (minimum 1 minute)
	if c.SessionTimeout < time.Minute {
		return fmt.Errorf("session timeout must be at least 1 minute, got: %v", c.SessionTimeout)
	}

	return nil
}

// Helper functions

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
