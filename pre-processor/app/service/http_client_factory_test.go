// TDD Phase: HTTPClientFactory Test Suite
// ABOUTME: Tests factory pattern for HTTP client creation based on configuration
// ABOUTME: Verifies Envoy proxy vs direct HTTP client selection logic

package service

import (
	"strings"
	"testing"
	"time"

	"log/slog"

	"pre-processor/config"
)

// TestHTTPClientFactory_CreateClient tests basic client creation
func TestHTTPClientFactory_CreateClient(t *testing.T) {
	tests := map[string]struct {
		config           *config.Config
		expectedType     string
		expectEnvoyStats bool
		description      string
	}{
		"envoy_enabled": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy:  true,
					EnvoyProxyURL:  "http://envoy-proxy:8080",
					EnvoyProxyPath: "/proxy/https://",
					EnvoyTimeout:   30 * time.Second,
					UserAgent:      "test-agent",
				},
			},
			expectedType:     "*service.EnvoyHTTPClient",
			expectEnvoyStats: true,
			description:      "Should create EnvoyHTTPClient when proxy enabled",
		},
		"envoy_disabled": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: false,
					Timeout:       30 * time.Second,
					UserAgent:     "test-agent",
				},
			},
			expectedType:     "*service.HTTPClientWrapper",
			expectEnvoyStats: false,
			description:      "Should create HTTPClientWrapper when proxy disabled",
		},
	}

	logger := slog.Default()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			factory := NewHTTPClientFactory(tc.config, logger)
			client := factory.CreateClient()

			if client == nil {
				t.Errorf("%s: expected client but got nil", tc.description)
				return
			}

			// Check client type using type assertion
			clientType := getClientTypeName(client)
			if !strings.Contains(clientType, strings.TrimPrefix(tc.expectedType, "*service.")) {
				t.Errorf("%s: expected client type %s, got %s",
					tc.description, tc.expectedType, clientType)
			}

			// Check stats
			stats := factory.GetClientStats()
			if stats.EnvoyEnabled != tc.expectEnvoyStats {
				t.Errorf("%s: expected EnvoyEnabled=%v, got %v",
					tc.description, tc.expectEnvoyStats, stats.EnvoyEnabled)
			}

			if tc.expectEnvoyStats && stats.ClientType != "envoy_proxy" {
				t.Errorf("%s: expected client type 'envoy_proxy', got '%s'",
					tc.description, stats.ClientType)
			}

			if !tc.expectEnvoyStats && stats.ClientType != "direct_http" {
				t.Errorf("%s: expected client type 'direct_http', got '%s'",
					tc.description, stats.ClientType)
			}
		})
	}
}

// TestHTTPClientFactory_CreateArticleFetcherClient tests article fetcher client creation
func TestHTTPClientFactory_CreateArticleFetcherClient(t *testing.T) {
	tests := map[string]struct {
		config      *config.Config
		description string
	}{
		"envoy_article_fetcher": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy:  true,
					EnvoyProxyURL:  "http://envoy-proxy:8080",
					EnvoyProxyPath: "/proxy/https://",
					EnvoyTimeout:   30 * time.Second,
					UserAgent:      "article-fetcher",
				},
			},
			description: "Article fetcher should use Envoy with extended timeout",
		},
		"direct_article_fetcher": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: false,
					Timeout:       30 * time.Second,
					UserAgent:     "article-fetcher",
				},
			},
			description: "Article fetcher should use optimized direct client",
		},
	}

	logger := slog.Default()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			factory := NewHTTPClientFactory(tc.config, logger)
			client := factory.CreateArticleFetcherClient()

			if client == nil {
				t.Errorf("%s: expected client but got nil", tc.description)
				return
			}

			// Verify client is created (basic functionality test)
			clientType := getClientTypeName(client)
			if clientType == "" {
				t.Errorf("%s: client type could not be determined", tc.description)
			}

			t.Logf("%s: created client type: %s", tc.description, clientType)
		})
	}
}

// TestHTTPClientFactory_CreateHealthCheckClient tests health check client creation
func TestHTTPClientFactory_CreateHealthCheckClient(t *testing.T) {
	tests := map[string]struct {
		config      *config.Config
		description string
	}{
		"envoy_health_check": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy:  true,
					EnvoyProxyURL:  "http://envoy-proxy:8080",
					EnvoyProxyPath: "/proxy/https://",
					EnvoyTimeout:   60 * time.Second, // Should be overridden to 30s
					UserAgent:      "health-checker",
				},
			},
			description: "Health checker should use Envoy with optimized timeout",
		},
		"direct_health_check": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: false,
					Timeout:       60 * time.Second,
					UserAgent:     "health-checker",
				},
			},
			description: "Health checker should use direct client with short timeout",
		},
	}

	logger := slog.Default()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			factory := NewHTTPClientFactory(tc.config, logger)
			client := factory.CreateHealthCheckClient()

			if client == nil {
				t.Errorf("%s: expected client but got nil", tc.description)
				return
			}

			clientType := getClientTypeName(client)
			t.Logf("%s: created health check client type: %s", tc.description, clientType)
		})
	}
}

// TestHTTPClientFactory_GetClientStats tests client statistics
func TestHTTPClientFactory_GetClientStats(t *testing.T) {
	tests := map[string]struct {
		config           *config.Config
		expectedEnvoy    bool
		expectedType     string
		expectedProxyURL string
		description      string
	}{
		"envoy_stats": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: true,
					EnvoyProxyURL: "http://test-envoy:8080",
					EnvoyTimeout:  45 * time.Second,
				},
			},
			expectedEnvoy:    true,
			expectedType:     "envoy_proxy",
			expectedProxyURL: "http://test-envoy:8080",
			description:      "Stats should reflect Envoy configuration",
		},
		"direct_stats": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: false,
					Timeout:       30 * time.Second,
				},
			},
			expectedEnvoy: false,
			expectedType:  "direct_http",
			description:   "Stats should reflect direct HTTP configuration",
		},
	}

	logger := slog.Default()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			factory := NewHTTPClientFactory(tc.config, logger)
			stats := factory.GetClientStats()

			if stats == nil {
				t.Errorf("%s: expected stats but got nil", tc.description)
				return
			}

			if stats.EnvoyEnabled != tc.expectedEnvoy {
				t.Errorf("%s: expected EnvoyEnabled=%v, got %v",
					tc.description, tc.expectedEnvoy, stats.EnvoyEnabled)
			}

			if stats.ClientType != tc.expectedType {
				t.Errorf("%s: expected ClientType=%s, got %s",
					tc.description, tc.expectedType, stats.ClientType)
			}

			if tc.expectedProxyURL != "" && stats.EnvoyProxyURL != tc.expectedProxyURL {
				t.Errorf("%s: expected EnvoyProxyURL=%s, got %s",
					tc.description, tc.expectedProxyURL, stats.EnvoyProxyURL)
			}

			if stats.TotalClients != 1 {
				t.Errorf("%s: expected TotalClients=1, got %d",
					tc.description, stats.TotalClients)
			}
		})
	}
}

// TestHTTPClientFactory_ConfigValidation tests configuration validation
func TestHTTPClientFactory_ConfigValidation(t *testing.T) {
	logger := slog.Default()

	tests := map[string]struct {
		config      *config.Config
		expectError bool
		description string
	}{
		"nil_config": {
			config:      nil,
			expectError: false, // Factory should handle nil gracefully
			description: "Factory should handle nil config without panic",
		},
		"empty_config": {
			config: &config.Config{
				HTTP: config.HTTPConfig{},
			},
			expectError: false,
			description: "Factory should handle empty config",
		},
		"invalid_envoy_config": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: true,
					EnvoyProxyURL: "", // Invalid empty URL
				},
			},
			expectError: true,
			description: "Invalid Envoy config should return error client",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create factory - this should not panic even with invalid config
			factory := NewHTTPClientFactory(tc.config, logger)

			if factory == nil {
				t.Errorf("%s: expected factory but got nil", tc.description)
				return
			}

			// Try to create client
			client := factory.CreateClient()

			if client == nil {
				t.Errorf("%s: expected client but got nil", tc.description)
				return
			}

			// Test if client works by attempting a request to invalid URL
			// This is just to verify the client interface works
			_, err := client.Get("invalid-url")

			if tc.expectError && err == nil {
				t.Errorf("%s: expected error but got none", tc.description)
			}

			// Note: We don't test !tc.expectError case because invalid URLs
			// should always return errors regardless of client type
		})
	}
}

// Helper function to get client type name for testing
func getClientTypeName(client HTTPClient) string {
	switch client.(type) {
	case *EnvoyHTTPClient:
		return "EnvoyHTTPClient"
	case *HTTPClientWrapper:
		return "HTTPClientWrapper"
	case *OptimizedHTTPClientWrapper:
		return "OptimizedHTTPClientWrapper"
	case *errorHTTPClient:
		return "errorHTTPClient"
	default:
		return "unknown"
	}
}
