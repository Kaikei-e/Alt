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

func parseFloatEnv(key string, defaultValue float64) (float64, error) {
	v, err := getEnvOrDefault(key, "")
	if err != nil {
		return 0, err
	}
	if v == "" {
		return defaultValue, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 {
		return defaultValue, nil
	}
	return f, nil
}

func parseIntEnv(key string, defaultValue int) (int, error) {
	v, err := getEnvOrDefault(key, "")
	if err != nil {
		return 0, err
	}
	if v == "" {
		return defaultValue, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultValue, nil
	}
	return n, nil
}

type Config struct {
	Meilisearch MeilisearchConfig
	BackendAPI  BackendAPIConfig
	RateLimit   RateLimitConfig
}

// RateLimitConfig bounds incoming REST and Connect-RPC request throughput.
// Global bucket because callers are already authenticated via TLS peer identity;
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
}

type MeilisearchConfig struct {
	Host    string
	APIKey  string
	Timeout time.Duration
}

func Load() (*Config, error) {
	ctx := context.Background()

	backendAPIURL, err := getEnvOrDefault("BACKEND_API_URL", "")
	if err != nil {
		return nil, err
	}
	if backendAPIURL == "" {
		return nil, fmt.Errorf("required environment variable BACKEND_API_URL is not set")
	}

	meiliHost, err := getEnvOrDefault("MEILISEARCH_HOST", "")
	if err != nil {
		return nil, err
	}
	meiliAPIKey, err := getEnvOrDefault("MEILISEARCH_API_KEY", "")
	if err != nil {
		return nil, err
	}
	rps, err := parseFloatEnv("SEARCH_RATE_LIMIT_RPS", 100)
	if err != nil {
		return nil, err
	}
	burst, err := parseIntEnv("SEARCH_RATE_LIMIT_BURST", 200)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		BackendAPI: BackendAPIConfig{
			URL: backendAPIURL,
		},
		Meilisearch: MeilisearchConfig{
			Host:    meiliHost,
			APIKey:  meiliAPIKey,
			Timeout: 15 * time.Second,
		},
		RateLimit: RateLimitConfig{
			RequestsPerSecond: rps,
			Burst:             burst,
		},
	}

	// Validate Meilisearch config (always required)
	if cfg.Meilisearch.Host == "" {
		return nil, fmt.Errorf("meilisearch configuration error: required environment variable MEILISEARCH_HOST is not set")
	}

	// The API key is not hard-required so a keyless local Meilisearch still
	// boots, but an empty key must be an explicit, logged decision rather than
	// a silent default (Rule 9): in compose/workers.yaml the key arrives via
	// MEILISEARCH_API_KEY_FILE and an empty result there means the write path
	// will be silently unauthenticated. A broken _FILE mount is caught above
	// (getEnvOrDefault now fails closed); this covers "no key configured".
	if cfg.Meilisearch.APIKey == "" {
		slog.WarnContext(ctx, "MEILISEARCH_API_KEY is empty: connecting to Meilisearch WITHOUT authentication (no-auth mode)",
			"meilisearch_host", cfg.Meilisearch.Host,
		)
	}

	slog.InfoContext(ctx, "configuration loaded",
		"backend_api_url", backendAPIURL,
		"meilisearch_host", cfg.Meilisearch.Host,
		"meilisearch_auth", cfg.Meilisearch.APIKey != "",
	)

	return cfg, nil
}

// getEnvOrDefault resolves a config value, preferring a Docker-secret style
// "<key>_FILE" path over the plain "<key>" env var. When <key>_FILE is set but
// the file cannot be read it returns an error so startup fails fast (Rule 9):
// silently falling back to the plain var or the default would let a broken
// secret mount degrade into an empty credential without any signal.
func getEnvOrDefault(key, defaultValue string) (string, error) {
	// Check for _FILE suffix
	if fileValue := os.Getenv(key + "_FILE"); fileValue != "" {
		content, err := os.ReadFile(fileValue)
		if err != nil {
			return "", fmt.Errorf("read secret file for %s (%s_FILE=%q): %w", key, key, fileValue, err)
		}
		return strings.TrimSpace(string(content)), nil
	}

	if value := os.Getenv(key); value != "" {
		return value, nil
	}
	return defaultValue, nil
}
