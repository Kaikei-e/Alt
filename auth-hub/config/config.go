package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds the application configuration
type Config struct {
	KratosURL  string        // Kratos internal URL
	Port       string        // Service port
	CacheTTL   time.Duration // Session cache TTL
	CSRFSecret string        // CSRF secret for token generation
}

// Load reads configuration from environment variables with sensible defaults
func Load() (*Config, error) {
	config := &Config{
		KratosURL:  getEnv("KRATOS_URL", "http://kratos:4433"),
		Port:       getEnv("PORT", "8888"),
		CacheTTL:   5 * time.Minute, // Default 5 minutes
		CSRFSecret: getEnv("CSRF_SECRET", ""),
	}

	// Parse CACHE_TTL if provided
	if cacheTTLStr := os.Getenv("CACHE_TTL"); cacheTTLStr != "" {
		duration, err := time.ParseDuration(cacheTTLStr)
		if err != nil {
			return nil, fmt.Errorf("invalid CACHE_TTL format: %w", err)
		}
		config.CacheTTL = duration
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
