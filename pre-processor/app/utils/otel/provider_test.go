package otel

import (
	"context"
	"testing"
)

func TestConfigFromEnv(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		t.Setenv("OTEL_SERVICE_NAME", "")
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
		t.Setenv("OTEL_ENABLED", "")

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
		t.Setenv("OTEL_SERVICE_NAME", "test-service")
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://otel:4318")
		t.Setenv("OTEL_ENABLED", "false")

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
