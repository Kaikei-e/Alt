package config

import (
	"fmt"
	"os"
)

// Config holds the service configuration.
type Config struct {
	DatabaseURL string
	ListenAddr  string
	MetricsAddr string
	// AdminToken, if set, is required as a Bearer token on the mutating
	// /admin/* endpoints (snapshots/retention/storage) served on MetricsAddr.
	// Empty means admin auth is explicitly disabled — main.go logs this
	// loudly at startup so "forgot to set it" and "intentionally open" are
	// never indistinguishable (Rule 8).
	AdminToken string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":9500"
	}

	metricsAddr := os.Getenv("METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = ":9501"
	}

	return &Config{
		DatabaseURL: dbURL,
		ListenAddr:  listenAddr,
		MetricsAddr: metricsAddr,
		AdminToken:  os.Getenv("ADMIN_TOKEN"),
	}, nil
}
