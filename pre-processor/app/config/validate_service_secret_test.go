package config

import (
	"strings"
	"testing"
	"time"
)

// LOW-2: pre-processor must refuse to start in production without a
// service secret. Silent startup would yield HTTP 500 for every internal
// request, which is strictly worse than crashing at boot.

func validBaseConfig() *Config {
	return &Config{
		Server:      ServerConfig{Port: 9200},
		HTTP:        HTTPConfig{Timeout: time.Second, MinContentLength: 0, MaxRedirects: 0},
		Retry:       RetryConfig{MaxAttempts: 1, BackoffFactor: 2.0},
		RateLimit:   RateLimitConfig{DefaultInterval: 5 * time.Second},
		Metrics:     MetricsConfig{Port: 9201},
		NewsCreator: NewsCreatorConfig{Host: "http://news-creator:11434", Timeout: time.Second},
		SummarizeQueue: SummarizeQueueConfig{
			WorkerInterval:  time.Second,
			MaxRetries:      0,
			PollingInterval: time.Second,
			Concurrency:     1,
		},
	}
}

func TestValidate_ProductionRequiresServiceSecret(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SERVICE_SECRET", "")
	t.Setenv("SERVICE_SECRET_FILE", "")

	err := validateConfig(validBaseConfig())
	if err == nil || !strings.Contains(err.Error(), "SERVICE_SECRET") {
		t.Fatalf("expected SERVICE_SECRET error in production, got %v", err)
	}
}

func TestValidate_ProductionWithSecretPasses(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SERVICE_SECRET", "some-secret")
	t.Setenv("SERVICE_SECRET_FILE", "")

	if err := validateConfig(validBaseConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_NonProductionToleratesEmptySecret(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("SERVICE_SECRET", "")
	t.Setenv("SERVICE_SECRET_FILE", "")

	if err := validateConfig(validBaseConfig()); err != nil {
		t.Fatalf("non-production must tolerate empty secret, got %v", err)
	}
}
