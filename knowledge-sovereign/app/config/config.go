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
	}, nil
}
