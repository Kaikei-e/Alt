package config

import (
	"fmt"
	"os"
)

// Config holds the service configuration.
type Config struct {
	DatabaseURL   string
	ListenAddr    string
	MetricsAddr   string
	ServiceSecret string
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

	serviceSecret := os.Getenv("SERVICE_SECRET")
	if serviceSecret == "" {
		secretFile := os.Getenv("SERVICE_SECRET_FILE")
		if secretFile != "" {
			data, err := os.ReadFile(secretFile)
			if err != nil {
				return nil, fmt.Errorf("read service secret file: %w", err)
			}
			serviceSecret = string(data)
		}
	}

	return &Config{
		DatabaseURL:   dbURL,
		ListenAddr:    listenAddr,
		MetricsAddr:   metricsAddr,
		ServiceSecret: serviceSecret,
	}, nil
}
