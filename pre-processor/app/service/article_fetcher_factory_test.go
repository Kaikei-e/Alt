// TDD Phase: ArticleFetcher Factory Integration Tests
// ABOUTME: Tests HTTPClientFactory integration with ArticleFetcher service
// ABOUTME: Verifies Envoy proxy configuration and automatic client selection

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"log/slog"

	"pre-processor/config"
	"pre-processor/retry"
)

// TestNewArticleFetcherServiceWithFactory tests factory-based constructor
func TestNewArticleFetcherServiceWithFactory(t *testing.T) {
	tests := map[string]struct {
		config      *config.Config
		expectEnvoy bool
		description string
	}{
		"factory_envoy_enabled": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy:  true,
					EnvoyProxyURL:  "http://test-envoy:8080",
					EnvoyProxyPath: "/proxy/https://",
					EnvoyTimeout:   30 * time.Second,
					UserAgent:      "test-factory-envoy",
				},
			},
			expectEnvoy: true,
			description: "Factory should create Envoy-enabled fetcher",
		},
		"factory_direct_http": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: false,
					Timeout:       30 * time.Second,
					UserAgent:     "test-factory-direct",
				},
			},
			expectEnvoy: false,
			description: "Factory should create direct HTTP fetcher",
		},
	}

	logger := slog.Default()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create service using factory constructor
			service := NewArticleFetcherServiceWithFactory(tc.config, logger)

			if service == nil {
				t.Errorf("%s: expected service but got nil", tc.description)
				return
			}

			// Verify service type
			fetcherService, ok := service.(*articleFetcherService)
			if !ok {
				t.Errorf("%s: expected *articleFetcherService but got different type", tc.description)
				return
			}

			// Verify HTTP client is set
			if fetcherService.httpClient == nil {
				t.Errorf("%s: expected httpClient to be set but got nil", tc.description)
				return
			}

			// Verify client type matches expectation
			clientType := getClientTypeName(fetcherService.httpClient)
			if tc.expectEnvoy && clientType != "EnvoyHTTPClient" {
				t.Errorf("%s: expected EnvoyHTTPClient but got %s", tc.description, clientType)
			}
			if !tc.expectEnvoy && clientType == "EnvoyHTTPClient" {
				t.Errorf("%s: expected non-Envoy client but got EnvoyHTTPClient", tc.description)
			}

			t.Logf("%s: created fetcher with client type: %s", tc.description, clientType)
		})
	}
}

// TestNewArticleFetcherServiceWithFactoryAndDLQ tests factory + DLQ constructor
func TestNewArticleFetcherServiceWithFactoryAndDLQ(t *testing.T) {
	tests := map[string]struct {
		config      *config.Config
		retrier     *retry.Retrier
		dlqEnabled  bool
		expectEnvoy bool
		description string
	}{
		"factory_dlq_envoy": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy:  true,
					EnvoyProxyURL:  "http://test-envoy:8080",
					EnvoyProxyPath: "/proxy/https://",
					EnvoyTimeout:   60 * time.Second,
					UserAgent:      "test-factory-dlq-envoy",
				},
			},
			retrier:     nil, // Will create default
			dlqEnabled:  true,
			expectEnvoy: true,
			description: "Factory with DLQ should create Envoy-enabled fetcher",
		},
		"factory_dlq_direct": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: false,
					Timeout:       30 * time.Second,
					UserAgent:     "test-factory-dlq-direct",
				},
			},
			retrier:     nil,
			dlqEnabled:  true,
			expectEnvoy: false,
			description: "Factory with DLQ should create direct HTTP fetcher",
		},
	}

	logger := slog.Default()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create mock DLQ publisher
			var dlqPublisher DLQPublisher
			if tc.dlqEnabled {
				dlqPublisher = &mockDLQPublisher{}
			}

			// Create service using factory + DLQ constructor
			service := NewArticleFetcherServiceWithFactoryAndDLQ(tc.config, logger, tc.retrier, dlqPublisher)

			if service == nil {
				t.Errorf("%s: expected service but got nil", tc.description)
				return
			}

			// Verify service type and configuration
			fetcherService, ok := service.(*articleFetcherService)
			if !ok {
				t.Errorf("%s: expected *articleFetcherService but got different type", tc.description)
				return
			}

			// Verify HTTP client is set
			if fetcherService.httpClient == nil {
				t.Errorf("%s: expected httpClient to be set but got nil", tc.description)
				return
			}

			// Verify retry and DLQ components
			if tc.dlqEnabled && fetcherService.dlqPublisher == nil {
				t.Errorf("%s: expected dlqPublisher to be set", tc.description)
			}

			if fetcherService.retrier == nil {
				t.Errorf("%s: expected retrier to be set (default should be created)", tc.description)
			}

			// Verify client type matches expectation
			clientType := getClientTypeName(fetcherService.httpClient)
			if tc.expectEnvoy && clientType != "EnvoyHTTPClient" {
				t.Errorf("%s: expected EnvoyHTTPClient but got %s", tc.description, clientType)
			}

			t.Logf("%s: created fetcher with client type: %s, DLQ: %v, Retrier: %v",
				tc.description, clientType, fetcherService.dlqPublisher != nil, fetcherService.retrier != nil)
		})
	}
}

// TestArticleFetcherFactory_Integration tests end-to-end factory integration
func TestArticleFetcherFactory_Integration(t *testing.T) {
	// Create mock server for testing
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock response content
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`
			<!DOCTYPE html>
			<html>
			<head><title>Test Article</title></head>
			<body>
				<article>
					<h1>Test Article Title</h1>
					<p>This is test article content for factory integration testing.</p>
				</article>
			</body>
			</html>
		`))
	}))
	defer mockServer.Close()

	tests := map[string]struct {
		config      *config.Config
		targetURL   string
		expectError bool
		description string
	}{
		"private_network_blocked": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: false,
					Timeout:       30 * time.Second,
					UserAgent:     "test-integration",
				},
			},
			targetURL:   mockServer.URL, // This will be 127.0.0.1
			expectError: false,          // Article fetching is disabled, returns nil,nil
			description: "Article fetching is disabled for ethical compliance",
		},
		"envoy_config_error": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy:  true,
					EnvoyProxyURL:  "", // Invalid empty URL
					EnvoyProxyPath: "/proxy/https://",
				},
			},
			targetURL:   "https://example.com",
			expectError: false, // Article fetching is disabled, returns nil,nil
			description: "Article fetching is disabled for ethical compliance",
		},
		"invalid_url": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: false,
					Timeout:       30 * time.Second,
					UserAgent:     "test-integration",
				},
			},
			targetURL:   "invalid-url-format",
			expectError: false, // Article fetching is disabled, returns nil,nil
			description: "Article fetching is disabled for ethical compliance",
		},
	}

	logger := slog.Default()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create service using factory
			service := NewArticleFetcherServiceWithFactory(tc.config, logger)

			// Test article fetching
			ctx := context.Background()
			article, err := service.FetchArticle(ctx, tc.targetURL)

			// Article fetching is currently disabled for ethical compliance
			// All requests should return nil, nil regardless of configuration
			if err != nil {
				t.Errorf("%s: unexpected error (article fetching disabled): %v", tc.description, err)
				return
			}

			if article != nil {
				t.Errorf("%s: expected nil article (fetching disabled) but got: %+v", tc.description, article)
				return
			}

			t.Logf("%s: article fetching disabled, returned nil as expected", tc.description)
		})
	}
}

// mockDLQPublisher implements DLQPublisher interface for testing
type mockDLQPublisher struct{}

func (m *mockDLQPublisher) PublishFailedArticle(ctx context.Context, url string, attempts int, lastError error) error {
	// Mock implementation - just log the failure
	return nil
}
