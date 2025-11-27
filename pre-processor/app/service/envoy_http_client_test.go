// TDD Phase 1 - RED: EnvoyHTTPClient Test Suite
// ABOUTME: This file tests Envoy proxy integration for external HTTP requests
// ABOUTME: Implements TDD cycle for RSS article fetching through Envoy proxy

package service

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"log/slog"

	"pre-processor/config"
)

// TestEnvoyHTTPClient_Get tests HTTP GET through Envoy proxy
func TestEnvoyHTTPClient_Get(t *testing.T) {
	tests := map[string]struct {
		targetURL     string
		envoyConfig   *config.HTTPConfig
		expectError   bool
		expectHeaders map[string]string
		description   string
	}{
		"valid_rss_url": {
			targetURL: "https://feeds.bbci.co.uk/news/rss.xml",
			envoyConfig: &config.HTTPConfig{
				UseEnvoyProxy:  true,
				EnvoyProxyURL:  "http://test-envoy:8080",
				EnvoyProxyPath: "/proxy/https://",
				EnvoyTimeout:   30 * time.Second,
			},
			expectError: false,
			expectHeaders: map[string]string{
				"X-Target-Domain": "feeds.bbci.co.uk",
				"X-Resolved-IP":   "", // Will be resolved dynamically
			},
			description: "Valid RSS URL should generate correct Envoy proxy request",
		},
		"invalid_url": {
			targetURL: "invalid-url",
			envoyConfig: &config.HTTPConfig{
				UseEnvoyProxy: true,
			},
			expectError: true,
			description: "Invalid URL should return error",
		},
		"empty_proxy_url": {
			targetURL: "https://example.com",
			envoyConfig: &config.HTTPConfig{
				UseEnvoyProxy: true,
				EnvoyProxyURL: "",
			},
			expectError: true,
			description: "Empty proxy URL should return error",
		},
		"valid_qiita_url": {
			targetURL: "https://qiita.com/tags/golang/feed",
			envoyConfig: &config.HTTPConfig{
				UseEnvoyProxy:  true,
				EnvoyProxyURL:  "http://test-envoy:8080",
				EnvoyProxyPath: "/proxy/https://",
				EnvoyTimeout:   30 * time.Second,
			},
			expectError: false,
			expectHeaders: map[string]string{
				"X-Target-Domain": "qiita.com",
			},
			description: "Qiita RSS URL should be handled correctly",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create mock Envoy server
			mockEnvoy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify Envoy proxy request format
				if !strings.HasPrefix(r.URL.Path, "/proxy/https://") {
					t.Errorf("Expected proxy path prefix, got: %s", r.URL.Path)
				}

				// Verify required headers
				for header, expectedValue := range tc.expectHeaders {
					if expectedValue != "" {
						if actualValue := r.Header.Get(header); actualValue != expectedValue {
							t.Errorf("Expected header %s=%s, got: %s", header, expectedValue, actualValue)
						}
					} else {
						// Just check if header exists for dynamic values like IP
						if header == "X-Resolved-IP" && r.Header.Get(header) == "" {
							t.Errorf("Expected header %s to be present", header)
						}
					}
				}

				// Return mock RSS content
				w.Header().Set("Content-Type", "application/rss+xml")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test RSS Feed</title>
    <description>Test feed for Envoy proxy</description>
    <item>
      <title>Test Article</title>
      <link>https://example.com/article</link>
      <description>Test article content</description>
    </item>
  </channel>
</rss>`))
			}))
			defer mockEnvoy.Close()

			// Update config with mock server URL
			testConfig := *tc.envoyConfig
			if testConfig.EnvoyProxyURL != "" {
				testConfig.EnvoyProxyURL = mockEnvoy.URL
			}

			// Create EnvoyHTTPClient
			logger := slog.Default()
			client := NewEnvoyHTTPClient(&testConfig, logger)

			// Execute test
			resp, err := client.Get(tc.targetURL)

			// Verify results
			if tc.expectError {
				if err == nil {
					t.Errorf("%s: expected error but got none", tc.description)
				}
				return
			}

			if err != nil {
				t.Errorf("%s: unexpected error: %v", tc.description, err)
				return
			}

			if resp == nil {
				t.Errorf("%s: expected response but got nil", tc.description)
				return
			}

			defer func() {
				_ = resp.Body.Close()
			}()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("%s: expected status 200, got: %d", tc.description, resp.StatusCode)
			}

			// Verify content type
			if contentType := resp.Header.Get("Content-Type"); !strings.Contains(contentType, "rss") {
				t.Logf("%s: Note: Content-Type is %s (expected RSS)", tc.description, contentType)
			}
		})
	}
}

// TestEnvoyHTTPClient_DNSResolution tests DNS resolution for X-Resolved-IP header
func TestEnvoyHTTPClient_DNSResolution(t *testing.T) {
	tests := map[string]struct {
		targetURL     string
		expectResolve bool
		description   string
	}{
		"resolve_bbc": {
			targetURL:     "https://feeds.bbci.co.uk/news/rss.xml",
			expectResolve: true,
			description:   "BBC domain should resolve to valid IP",
		},
		"resolve_qiita": {
			targetURL:     "https://qiita.com/tags/golang/feed",
			expectResolve: true,
			description:   "Qiita domain should resolve to valid IP",
		},
		"invalid_domain": {
			targetURL:     "https://non-existent-domain-12345.com",
			expectResolve: false,
			description:   "Non-existent domain should fail resolution",
		},
	}

	config := &config.HTTPConfig{
		UseEnvoyProxy:  true,
		EnvoyProxyURL:  "http://test-envoy:8080",
		EnvoyProxyPath: "/proxy/https://",
		EnvoyTimeout:   30 * time.Second,
	}

	logger := slog.Default()
	client := NewEnvoyHTTPClient(config, logger)

	// Type assert to access ResolveDomain method
	envoyClient, ok := client.(*EnvoyHTTPClient)
	if !ok {
		t.Fatalf("Expected EnvoyHTTPClient but got different type")
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Parse URL to extract hostname
			parsedURL, err := url.Parse(tc.targetURL)
			if err != nil {
				t.Fatalf("Failed to parse test URL: %v", err)
			}

			// Test DNS resolution directly
			ip, err := envoyClient.ResolveDomain(parsedURL.Hostname())

			if tc.expectResolve {
				if err != nil {
					t.Errorf("%s: expected successful resolution but got error: %v", tc.description, err)
					return
				}
				if ip == "" {
					t.Errorf("%s: expected valid IP but got empty string", tc.description)
					return
				}
				t.Logf("%s: successfully resolved %s to %s", tc.description, parsedURL.Hostname(), ip)
			} else {
				if err == nil {
					t.Errorf("%s: expected DNS resolution failure but got success", tc.description)
				}
			}
		})
	}
}

// TestEnvoyHTTPClient_TimeoutHandling tests timeout scenarios
func TestEnvoyHTTPClient_TimeoutHandling(t *testing.T) {
	// Create slow mock server
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Simulate slow response
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("slow response"))
	}))
	defer slowServer.Close()

	tests := map[string]struct {
		timeout       time.Duration
		expectTimeout bool
		description   string
	}{
		"fast_timeout": {
			timeout:       100 * time.Millisecond,
			expectTimeout: true,
			description:   "Fast timeout should trigger timeout error",
		},
		"slow_timeout": {
			timeout:       5 * time.Second,
			expectTimeout: false,
			description:   "Slow timeout should complete successfully",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			config := &config.HTTPConfig{
				UseEnvoyProxy:  true,
				EnvoyProxyURL:  slowServer.URL,
				EnvoyProxyPath: "/proxy/https://",
				EnvoyTimeout:   tc.timeout,
			}

			logger := slog.Default()
			client := NewEnvoyHTTPClient(config, logger)

			start := time.Now()
			resp, err := client.Get("https://example.com")

			duration := time.Since(start)

			if tc.expectTimeout {
				if err == nil {
					t.Errorf("%s: expected timeout error but got success", tc.description)
					if resp != nil {
						resp.Body.Close()
					}
					return
				}
				if duration > tc.timeout*2 { // Allow some margin
					t.Errorf("%s: timeout took too long: %v (expected around %v)", tc.description, duration, tc.timeout)
				}
			} else {
				if err != nil {
					t.Errorf("%s: unexpected error: %v", tc.description, err)
					return
				}
				if resp == nil {
					t.Errorf("%s: expected response but got nil", tc.description)
					return
				}
				defer func() {
					_ = resp.Body.Close()
				}()
			}
		})
	}
}

// TestEnvoyHTTPClient_ErrorHandling tests various error scenarios
func TestEnvoyHTTPClient_ErrorHandling(t *testing.T) {
	tests := map[string]struct {
		config      *config.HTTPConfig
		targetURL   string
		expectError bool
		description string
	}{
		"nil_config": {
			config:      nil,
			targetURL:   "https://example.com",
			expectError: true,
			description: "Nil config should return error",
		},
		"disabled_proxy": {
			config: &config.HTTPConfig{
				UseEnvoyProxy: false,
			},
			targetURL:   "https://example.com",
			expectError: true,
			description: "Disabled proxy should return error when using EnvoyHTTPClient",
		},
		"malformed_proxy_url": {
			config: &config.HTTPConfig{
				UseEnvoyProxy: true,
				EnvoyProxyURL: "invalid-url-format",
			},
			targetURL:   "https://example.com",
			expectError: true,
			description: "Malformed proxy URL should return error",
		},
	}

	logger := slog.Default()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := NewEnvoyHTTPClient(tc.config, logger)

			resp, err := client.Get(tc.targetURL)

			if tc.expectError {
				if err == nil {
					t.Errorf("%s: expected error but got none", tc.description)
					if resp != nil {
						resp.Body.Close()
					}
				}
			} else {
				if err != nil {
					t.Errorf("%s: unexpected error: %v", tc.description, err)
				}
				if resp != nil {
					defer func() {
						_ = resp.Body.Close()
					}()
				}
			}
		})
	}
}
