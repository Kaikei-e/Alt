package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

func parseFloatEnv(key string, defaultValue float64) float64 {
	v := getEnvOrDefault(key, "")
	if v == "" {
		return defaultValue
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 {
		return defaultValue
	}
	return f
}

func parseIntEnv(key string, defaultValue int) int {
	v := getEnvOrDefault(key, "")
	if v == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultValue
	}
	return n
}

type Config struct {
	Meilisearch MeilisearchConfig
	Indexer     IndexerConfig
	HTTP        HTTPConfig
	BackendAPI  BackendAPIConfig
	RateLimit   RateLimitConfig
}

// RateLimitConfig bounds incoming REST and Connect-RPC request throughput.
// Global bucket because callers are already authenticated via X-Service-Token;
// per-caller isolation is deferred until caller identity is propagated.
type RateLimitConfig struct {
	// RequestsPerSecond is the sustained refill rate for the token bucket.
	RequestsPerSecond float64
	// Burst is the maximum request count that can arrive simultaneously.
	Burst int
}

// BackendAPIConfig holds configuration for connecting to alt-backend's internal API.
type BackendAPIConfig struct {
	// URL is the Connect-RPC URL for alt-backend's internal API.
	URL string
	// ServiceToken is the shared secret for service authentication.
	ServiceToken string
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

	backendAPIURL := getEnvOrDefault("BACKEND_API_URL", "")
	if backendAPIURL == "" {
		return nil, fmt.Errorf("required environment variable BACKEND_API_URL is not set")
	}

	cfg := &Config{
		BackendAPI: BackendAPIConfig{
			URL:          backendAPIURL,
			ServiceToken: getEnvOrDefault("SERVICE_TOKEN", ""),
		},
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
		RateLimit: RateLimitConfig{
			RequestsPerSecond: parseFloatEnv("SEARCH_RATE_LIMIT_RPS", 100),
			Burst:             parseIntEnv("SEARCH_RATE_LIMIT_BURST", 200),
		},
	}

	// Validate Meilisearch config (always required)
	if cfg.Meilisearch.Host == "" {
		return nil, fmt.Errorf("meilisearch configuration error: required environment variable MEILISEARCH_HOST is not set")
	}

	slog.InfoContext(ctx, "configuration loaded",
		"backend_api_url", backendAPIURL,
		"meilisearch_host", cfg.Meilisearch.Host,
	)

	return cfg, nil
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
