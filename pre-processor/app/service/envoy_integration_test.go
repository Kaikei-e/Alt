// TDD Phase 3: Envoy Proxy Integration Tests
// ABOUTME: End-to-end integration tests for Envoy proxy communication
// ABOUTME: Tests real proxy behavior, DNS resolution, and configuration switching

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"log/slog"

	"pre-processor/config"
)

// TestEnvoyIntegration_EndToEnd tests complete Envoy integration workflow
func TestEnvoyIntegration_EndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create mock Envoy proxy server that simulates Envoy behavior
	envoyMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Envoy-specific headers are present
		targetDomain := r.Header.Get("X-Target-Domain")
		resolvedIP := r.Header.Get("X-Resolved-IP")

		if targetDomain == "" || resolvedIP == "" {
			t.Logf("Missing Envoy headers: X-Target-Domain=%s, X-Resolved-IP=%s",
				targetDomain, resolvedIP)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Verify the request path follows Envoy proxy pattern
		if !strings.HasPrefix(r.URL.Path, "/proxy/https://") {
			t.Logf("Invalid proxy path: %s", r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Mock successful response with target content
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`
			<!DOCTYPE html>
			<html>
			<head><title>Integration Test Article</title></head>
			<body>
				<article>
					<h1>Envoy Proxy Integration Test</h1>
					<p>This article was fetched through Envoy proxy successfully.</p>
				</article>
			</body>
			</html>
		`))
	}))
	defer envoyMock.Close()

	tests := map[string]struct {
		config        *config.Config
		targetURL     string
		expectSuccess bool
		expectEnvoy   bool
		description   string
	}{
		"envoy_proxy_article_fetch": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy:  true,
					EnvoyProxyURL:  envoyMock.URL,
					EnvoyProxyPath: "/proxy/https://",
					EnvoyTimeout:   30 * time.Second,
					UserAgent:      "integration-test-envoy",
				},
			},
			targetURL:     "https://example.com/article",
			expectSuccess: true,
			expectEnvoy:   true,
			description:   "Article fetching through Envoy proxy should succeed",
		},
		"direct_http_fallback": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: false,
					Timeout:       30 * time.Second,
					UserAgent:     "integration-test-direct",
				},
			},
			targetURL:     "https://example.com/article",
			expectSuccess: true, // example.com is reachable directly
			expectEnvoy:   false,
			description:   "Direct HTTP should be used when Envoy is disabled",
		},
		"envoy_configuration_error": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy:  true,
					EnvoyProxyURL:  "", // Invalid empty URL
					EnvoyProxyPath: "/proxy/https://",
					EnvoyTimeout:   30 * time.Second,
				},
			},
			targetURL:     "https://example.com/article",
			expectSuccess: false,
			expectEnvoy:   true,
			description:   "Invalid Envoy config should fail gracefully",
		},
	}

	logger := slog.Default()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create article fetcher with factory
			service := NewArticleFetcherServiceWithFactory(tc.config, logger)

			// Test article fetching
			ctx := context.Background()
			article, err := service.FetchArticle(ctx, tc.targetURL)

			if tc.expectSuccess {
				if err != nil {
					t.Errorf("%s: expected success but got error: %v", tc.description, err)
					return
				}

				if article == nil {
					t.Errorf("%s: expected article but got nil", tc.description)
					return
				}

				// Verify we got some article content (different content based on Envoy vs Direct)
				if tc.expectEnvoy && !strings.Contains(article.Title, "Integration Test") {
					t.Errorf("%s: expected Envoy integration test content but got: %s", tc.description, article.Title)
				} else if !tc.expectEnvoy && article.Title == "" {
					t.Errorf("%s: expected some article title but got empty string", tc.description)
				}

				t.Logf("%s: successfully fetched article via %s", tc.description,
					map[bool]string{true: "Envoy", false: "Direct"}[tc.expectEnvoy])
			} else {
				if err == nil {
					t.Errorf("%s: expected error but got success", tc.description)
				} else {
					t.Logf("%s: got expected error: %v", tc.description, err)
				}
			}
		})
	}
}

// TestEnvoyIntegration_HealthCheck tests health checking through Envoy
func TestEnvoyIntegration_HealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Mock Envoy proxy for health checks
	envoyMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate health check endpoint through proxy
		if strings.Contains(r.URL.Path, "/api/tags") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"models": [{"name": "integration-test-model"}]}`))
		} else if strings.Contains(r.URL.Path, "/json") {
			// Mock response for httpbin.org/json endpoint
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"models": [{"name": "integration-test-model"}]}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer envoyMock.Close()

	tests := map[string]struct {
		config        *config.Config
		serviceURL    string
		expectHealthy bool
		description   string
	}{
		"envoy_health_check_success": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy:  true,
					EnvoyProxyURL:  envoyMock.URL,
					EnvoyProxyPath: "/proxy/https://",
					EnvoyTimeout:   30 * time.Second,
					UserAgent:      "integration-test-health-envoy",
				},
			},
			serviceURL:    "https://httpbin.org/json", // Use resolvable test service
			expectHealthy: true,
			description:   "Health check through Envoy should succeed",
		},
		"direct_health_check_fail": {
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy: false,
					Timeout:       30 * time.Second,
					UserAgent:     "integration-test-health-direct",
				},
			},
			serviceURL:    "http://nonexistent-service:11434",
			expectHealthy: false,
			description:   "Direct health check to nonexistent service should fail",
		},
	}

	logger := slog.Default()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create health checker with factory
			service := NewHealthCheckerServiceWithFactory(tc.config, tc.serviceURL, logger)

			// Test health check
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := service.CheckNewsCreatorHealth(ctx)

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

// TestEnvoyIntegration_ConfigurationSwitching tests dynamic config switching
func TestEnvoyIntegration_ConfigurationSwitching(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.Default()

	// Test switching from direct to Envoy
	t.Run("direct_to_envoy_switch", func(t *testing.T) {
		// Start with direct HTTP
		directConfig := &config.Config{
			HTTP: config.HTTPConfig{
				UseEnvoyProxy: false,
				Timeout:       30 * time.Second,
				UserAgent:     "integration-test-switch",
			},
		}

		directService := NewArticleFetcherServiceWithFactory(directConfig, logger)

		// Verify direct client type
		fetcherService, ok := directService.(*articleFetcherService)
		if !ok {
			t.Errorf("Expected *articleFetcherService but got different type")
			return
		}

		directClientType := getClientTypeName(fetcherService.httpClient)
		if strings.Contains(directClientType, "Envoy") {
			t.Errorf("Expected non-Envoy client but got: %s", directClientType)
		}

		// Switch to Envoy configuration
		envoyMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer envoyMock.Close()

		envoyConfig := &config.Config{
			HTTP: config.HTTPConfig{
				UseEnvoyProxy:  true,
				EnvoyProxyURL:  envoyMock.URL,
				EnvoyProxyPath: "/proxy/https://",
				EnvoyTimeout:   30 * time.Second,
				UserAgent:      "integration-test-switch",
			},
		}

		envoyService := NewArticleFetcherServiceWithFactory(envoyConfig, logger)

		// Verify Envoy client type
		envoyFetcherService, ok := envoyService.(*articleFetcherService)
		if !ok {
			t.Errorf("Expected *articleFetcherService but got different type")
			return
		}

		envoyClientType := getClientTypeName(envoyFetcherService.httpClient)
		if envoyClientType != "EnvoyHTTPClient" {
			t.Errorf("Expected EnvoyHTTPClient but got: %s", envoyClientType)
		}

		t.Logf("Configuration switching: %s -> %s", directClientType, envoyClientType)
	})
}

// TestEnvoyIntegration_DNSResolution tests DNS resolution functionality
func TestEnvoyIntegration_DNSResolution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create mock Envoy that validates DNS resolution
	requestCount := 0
	envoyMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		targetDomain := r.Header.Get("X-Target-Domain")
		resolvedIP := r.Header.Get("X-Resolved-IP")

		t.Logf("Request %d: X-Target-Domain=%s, X-Resolved-IP=%s",
			requestCount, targetDomain, resolvedIP)

		// Verify headers are present
		if targetDomain == "" {
			t.Errorf("Missing X-Target-Domain header")
		}
		if resolvedIP == "" {
			t.Errorf("Missing X-Resolved-IP header")
		}

		// Verify IP format (should be valid IP address)
		if resolvedIP != "" && !isValidIP(resolvedIP) {
			t.Errorf("Invalid IP format: %s", resolvedIP)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("DNS resolution test successful"))
	}))
	defer envoyMock.Close()

	config := &config.Config{
		HTTP: config.HTTPConfig{
			UseEnvoyProxy:  true,
			EnvoyProxyURL:  envoyMock.URL,
			EnvoyProxyPath: "/proxy/https://",
			EnvoyTimeout:   30 * time.Second,
			UserAgent:      "integration-test-dns",
		},
	}

	logger := slog.Default()
	factory := NewHTTPClientFactory(config, logger)
	client := factory.CreateClient()

	// Test DNS resolution with well-known domain
	_, err := client.Get("https://example.com")

	if err != nil {
		t.Errorf("DNS resolution test failed: %v", err)
	}

	if requestCount != 1 {
		t.Errorf("Expected 1 request but got %d", requestCount)
	}
}

// Helper function to validate IP address format
func isValidIP(ip string) bool {
	// Simple IP validation - could be enhanced
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return false
	}

	for _, part := range parts {
		if len(part) == 0 || len(part) > 3 {
			return false
		}
		for _, char := range part {
			if char < '0' || char > '9' {
				return false
			}
		}
	}
	return true
}

// TestEnvoyIntegration_ErrorScenarios tests error handling and recovery
func TestEnvoyIntegration_ErrorScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.Default()

	tests := map[string]struct {
		setupMock   func() *httptest.Server
		config      *config.Config
		targetURL   string
		expectError bool
		description string
	}{
		"envoy_server_down": {
			setupMock: func() *httptest.Server {
				// Create and immediately close server to simulate down server
				server := httptest.NewServer(nil)
				server.Close()
				return server
			},
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy:  true,
					EnvoyProxyPath: "/proxy/https://",
					EnvoyTimeout:   5 * time.Second,
					UserAgent:      "integration-test-error",
				},
			},
			targetURL:   "https://example.com",
			expectError: true,
			description: "Should handle Envoy server being down",
		},
		"envoy_timeout": {
			setupMock: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Simulate slow response that exceeds timeout
					time.Sleep(2 * time.Second)
					w.WriteHeader(http.StatusOK)
				}))
			},
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy:  true,
					EnvoyProxyPath: "/proxy/https://",
					EnvoyTimeout:   1 * time.Second, // Short timeout
					UserAgent:      "integration-test-timeout",
				},
			},
			targetURL:   "https://example.com",
			expectError: true,
			description: "Should handle Envoy request timeout",
		},
		"envoy_error_response": {
			setupMock: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusBadGateway)
					w.Write([]byte("")) // Empty content will cause article parsing to fail
				}))
			},
			config: &config.Config{
				HTTP: config.HTTPConfig{
					UseEnvoyProxy:  true,
					EnvoyProxyPath: "/proxy/https://",
					EnvoyTimeout:   30 * time.Second,
					UserAgent:      "integration-test-error-response",
				},
			},
			targetURL:   "https://example.com",
			expectError: true,
			description: "Should handle Envoy error responses",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mock := tc.setupMock()
			tc.config.HTTP.EnvoyProxyURL = mock.URL
			defer mock.Close()

			service := NewArticleFetcherServiceWithFactory(tc.config, logger)

			ctx := context.Background()
			_, err := service.FetchArticle(ctx, tc.targetURL)

			if tc.expectError {
				if err == nil {
					t.Errorf("%s: expected error but got success", tc.description)
				} else {
					t.Logf("%s: got expected error: %v", tc.description, err)
				}
			} else {
				if err != nil {
					t.Errorf("%s: unexpected error: %v", tc.description, err)
				}
			}
		})
	}
}
