package otel

import (
	"context"
	"os"
	"testing"
)

func TestConfigFromEnv(t *testing.T) {
	originalServiceName := os.Getenv("OTEL_SERVICE_NAME")
	originalEnabled := os.Getenv("OTEL_ENABLED")
	defer func() {
		os.Setenv("OTEL_SERVICE_NAME", originalServiceName)
		os.Setenv("OTEL_ENABLED", originalEnabled)
	}()

	t.Run("default values", func(t *testing.T) {
		os.Unsetenv("OTEL_SERVICE_NAME")
		os.Unsetenv("OTEL_ENABLED")

		cfg := ConfigFromEnv()

		if cfg.ServiceName != "auth-hub" {
			t.Errorf("expected ServiceName 'auth-hub', got %s", cfg.ServiceName)
		}
		if !cfg.Enabled {
			t.Error("expected Enabled to be true by default")
		}
	})
}

func TestInitProvider_Disabled(t *testing.T) {
	cfg := Config{
		ServiceName:  "test",
		Enabled:      false,
		OTLPEndpoint: "http://localhost:4318",
	}

	shutdown, err := InitProvider(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown returned error: %v", err)
	}
}
