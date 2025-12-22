// TDD Phase: HealthChecker Factory Integration Tests
// ABOUTME: Tests HTTPClientFactory integration with HealthChecker service
// ABOUTME: Verifies Envoy proxy configuration and health check functionality

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"log/slog"

	"pre-processor/config"
)

func setHealthCheckerTransport(t *testing.T, service HealthCheckerService, handler http.HandlerFunc, delay time.Duration) {
	t.Helper()
	healthService, ok := service.(*healthCheckerService)
	if !ok {
		t.Fatalf("expected *healthCheckerService but got %T", service)
	}
	transport := newHandlerTransport(handler, delay)
	switch client := healthService.client.(type) {
	case *HTTPClientWrapper:
		client.Transport = transport
	case *OptimizedHTTPClientWrapper:
		client.Client.Transport = transport
	case *EnvoyHTTPClient:
		client.httpClient.Transport = transport
	}
}

// TestNewHealthCheckerServiceWithFactory tests factory-based constructor
func TestNewHealthCheckerServiceWithFactory(t *testing.T) {
	tests := map[string]struct {
		config         *config.Config
		newsCreatorURL string
		expectEnvoy    bool
		description    string
	}{
		"factory_envoy_health_check": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy:  true,
					EnvoyProxyURL:  "http://test-envoy:8080",
					EnvoyProxyPath: "/proxy/https://",
					EnvoyTimeout:   30 * time.Second,
					UserAgent:      "test-health-checker-envoy",
				},
			},
			newsCreatorURL: "http://news-creator:11434",
			expectEnvoy:    true,
			description:    "Factory should create Envoy-enabled health checker",
		},
		"factory_direct_health_check": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: false,
					Timeout:       30 * time.Second,
					UserAgent:     "test-health-checker-direct",
				},
			},
			newsCreatorURL: "http://news-creator:11434",
			expectEnvoy:    false,
			description:    "Factory should create direct HTTP health checker",
		},
	}

	logger := slog.Default()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create service using factory constructor
			service := NewHealthCheckerServiceWithFactory(tc.config, tc.newsCreatorURL, logger)

			if service == nil {
				t.Errorf("%s: expected service but got nil", tc.description)
				return
			}

			// Verify service type
			healthService, ok := service.(*healthCheckerService)
			if !ok {
				t.Errorf("%s: expected *healthCheckerService but got different type", tc.description)
				return
			}

			// Verify HTTP client is set
			if healthService.client == nil {
				t.Errorf("%s: expected httpClient to be set but got nil", tc.description)
				return
			}

			// Verify news creator URL is set
			if healthService.newsCreatorURL != tc.newsCreatorURL {
				t.Errorf("%s: expected newsCreatorURL=%s but got %s",
					tc.description, tc.newsCreatorURL, healthService.newsCreatorURL)
			}

			// Verify client type matches expectation
			clientType := getClientTypeName(healthService.client)
			if tc.expectEnvoy && clientType != "EnvoyHTTPClient" {
				t.Errorf("%s: expected EnvoyHTTPClient but got %s", tc.description, clientType)
			}
			if !tc.expectEnvoy && clientType == "EnvoyHTTPClient" {
				t.Errorf("%s: expected non-Envoy client but got EnvoyHTTPClient", tc.description)
			}

			t.Logf("%s: created health checker with client type: %s", tc.description, clientType)
		})
	}
}

// TestHealthCheckerFactory_Integration tests end-to-end health check functionality
func TestHealthCheckerFactory_Integration(t *testing.T) {
	mockNewsCreatorHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Health checker calls /health endpoint
		if r.URL.Path == "/health" {
			// Mock healthy response with models
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"models": [
					{"name": "gemma3:4b"},
					{"name": "llama2:7b"}
				]
			}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	})

	tests := map[string]struct {
		config         *config.Config
		newsCreatorURL string
		expectHealthy  bool
		expectError    bool
		description    string
	}{
		"direct_http_healthy": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: false,
					Timeout:       30 * time.Second,
					UserAgent:     "test-health-integration",
				},
			},
			newsCreatorURL: "http://news-creator.test",
			expectHealthy:  true,
			expectError:    false,
			description:    "Direct HTTP health check should succeed with mock server",
		},
		"envoy_invalid_config": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy:  true,
					EnvoyProxyURL:  "", // Invalid empty URL
					EnvoyProxyPath: "/proxy/https://",
					EnvoyTimeout:   30 * time.Second,
				},
			},
			newsCreatorURL: "http://news-creator:11434",
			expectHealthy:  false,
			expectError:    true,
			description:    "Invalid Envoy config should cause health check to fail",
		},
		"internal_service_allowed": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: false,
					Timeout:       30 * time.Second,
					UserAgent:     "test-health-integration",
				},
			},
			newsCreatorURL: "http://news-creator.test",
			expectHealthy:  true,
			expectError:    false,
			description:    "Internal service communication should be allowed for health checks",
		},
	}

	logger := slog.Default()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create service using factory
			service := NewHealthCheckerServiceWithFactory(tc.config, tc.newsCreatorURL, logger)
			if !tc.expectError {
				setHealthCheckerTransport(t, service, mockNewsCreatorHandler, 0)
			}

			// Test health check
			ctx := context.Background()
			err := service.CheckNewsCreatorHealth(ctx)

			if tc.expectError {
				if err == nil {
					t.Errorf("%s: expected error but got none", tc.description)
				} else {
					t.Logf("%s: got expected error: %v", tc.description, err)
				}
				return
			}

			if tc.expectHealthy {
				if err != nil {
					t.Errorf("%s: expected healthy but got error: %v", tc.description, err)
				} else {
					t.Logf("%s: health check succeeded as expected", tc.description)
				}
			} else {
				if err == nil {
					t.Errorf("%s: expected unhealthy but got success", tc.description)
				} else {
					t.Logf("%s: got expected error: %v", tc.description, err)
				}
			}
		})
	}
}

// TestHealthCheckerFactory_WaitForHealthy tests wait functionality
func TestHealthCheckerFactory_WaitForHealthy(t *testing.T) {
	callCount := 0
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// Health checker calls /health endpoint
		if r.URL.Path == "/health" {
			if callCount >= 2 { // Become healthy after second call
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"models": [{"name": "test-model"}]}`))
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	})

	tests := map[string]struct {
		config         *config.Config
		newsCreatorURL string
		expectSuccess  bool
		description    string
	}{
		"wait_for_internal_service": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: false,
					Timeout:       30 * time.Second,
					UserAgent:     "test-wait-health",
				},
			},
			newsCreatorURL: "http://news-creator.test",
			expectSuccess:  true,
			description:    "Wait should succeed for internal service communication",
		},
	}

	logger := slog.Default()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create service using factory
			service := NewHealthCheckerServiceWithFactory(tc.config, tc.newsCreatorURL, logger)
			setHealthCheckerTransport(t, service, mockHandler, 0)

			// Test wait for healthy with timeout longer than polling interval (10s)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			start := time.Now()
			err := service.WaitForHealthy(ctx)
			duration := time.Since(start)

			if tc.expectSuccess {
				if err != nil {
					t.Errorf("%s: expected success but got error: %v", tc.description, err)
				} else {
					t.Logf("%s: wait completed successfully after %v", tc.description, duration)
				}
			} else {
				if err == nil {
					t.Errorf("%s: expected failure but got success", tc.description)
				} else {
					t.Logf("%s: got expected error after %v: %v", tc.description, duration, err)
				}
			}
		})
	}
}

// TestHealthCheckerFactory_BackwardsCompatibility tests that old constructor still works
func TestHealthCheckerFactory_BackwardsCompatibility(t *testing.T) {
	logger := slog.Default()
	newsCreatorURL := "http://news-creator:11434"

	// Test old constructor
	service := NewHealthCheckerService(newsCreatorURL, logger)

	if service == nil {
		t.Errorf("Expected service but got nil")
		return
	}

	// Verify service type
	healthService, ok := service.(*healthCheckerService)
	if !ok {
		t.Errorf("Expected *healthCheckerService but got different type")
		return
	}

	// Verify HTTP client is set
	if healthService.client == nil {
		t.Errorf("Expected httpClient to be set but got nil")
		return
	}

	// Verify client type (should be HTTPClientWrapper for backwards compatibility)
	clientType := getClientTypeName(healthService.client)
	if clientType != "HTTPClientWrapper" {
		t.Errorf("Expected HTTPClientWrapper for backwards compatibility but got %s", clientType)
	}

	t.Logf("Backwards compatibility: created health checker with client type: %s", clientType)
}
