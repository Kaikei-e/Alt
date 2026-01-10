package otel

import (
	"context"
	"os"
	"testing"
)

func TestConfigFromEnv(t *testing.T) {
	originalServiceName := os.Getenv("OTEL_SERVICE_NAME")
	originalEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	originalEnabled := os.Getenv("OTEL_ENABLED")
	defer func() {
		os.Setenv("OTEL_SERVICE_NAME", originalServiceName)
		os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", originalEndpoint)
		os.Setenv("OTEL_ENABLED", originalEnabled)
	}()

	t.Run("default values", func(t *testing.T) {
		os.Unsetenv("OTEL_SERVICE_NAME")
		os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		os.Unsetenv("OTEL_ENABLED")

		cfg := ConfigFromEnv()

		if cfg.ServiceName != "pre-processor" {
			t.Errorf("expected ServiceName 'pre-processor', got %s", cfg.ServiceName)
		}
		if cfg.OTLPEndpoint != "http://localhost:4318" {
			t.Errorf("expected OTLPEndpoint 'http://localhost:4318', got %s", cfg.OTLPEndpoint)
		}
		if !cfg.Enabled {
			t.Error("expected Enabled to be true by default")
		}
	})

	t.Run("custom values", func(t *testing.T) {
		os.Setenv("OTEL_SERVICE_NAME", "test-service")
		os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://otel:4318")
		os.Setenv("OTEL_ENABLED", "false")

		cfg := ConfigFromEnv()

		if cfg.ServiceName != "test-service" {
			t.Errorf("expected ServiceName 'test-service', got %s", cfg.ServiceName)
		}
		if cfg.OTLPEndpoint != "http://otel:4318" {
			t.Errorf("expected OTLPEndpoint 'http://otel:4318', got %s", cfg.OTLPEndpoint)
		}
		if cfg.Enabled {
			t.Error("expected Enabled to be false")
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
