package register_feed_gateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
)

// TDD RED PHASE: Security Integration Tests
// These tests will fail until we implement security component integration

func TestRegisterFeedGateway_URLSecurityValidation(t *testing.T) {
	mockFetcher := NewMockRSSFeedFetcher()
	gateway := NewRegisterFeedLinkGatewayWithFetcher(nil, mockFetcher)

	tests := []struct {
		name          string
		url           string
		expectedError string
		wantErr       bool
		setupMock     func()
	}{
		{
			name:          "private IP should be blocked",
			url:           "http://192.168.1.1/feed.xml",
			expectedError: "private network access denied",
			wantErr:       true,
			setupMock: func() {
				// This should be blocked by URLSecurityValidator before reaching fetcher
			},
		},
		{
			name:          "localhost should be blocked",
			url:           "http://localhost/feed.xml",
			expectedError: "private network access denied",
			wantErr:       true,
			setupMock: func() {
				// This should be blocked by URLSecurityValidator before reaching fetcher
			},
		},
		{
			name:          "metadata server access should be blocked",
			url:           "http://metadata.google.internal/feed.xml",
			expectedError: "metadata server access denied",
			wantErr:       true,
			setupMock: func() {
				// This should be blocked by URLSecurityValidator before reaching fetcher
			},
		},
		{
			name:          "malicious scheme should be blocked",
			url:           "javascript:alert('xss')",
			expectedError: "only HTTP and HTTPS schemes allowed",
			wantErr:       true,
			setupMock: func() {
				// This should be blocked by URLSecurityValidator before reaching fetcher
			},
		},
		{
			name:          "valid public RSS URL should pass security validation",
			url:           "https://example.com/feed.xml",
			expectedError: "database connection not available", // Should pass security, fail at DB
			wantErr:       true,
			setupMock: func() {
				mockFetcher.SetFeed("https://example.com/feed.xml", &gofeed.Feed{
					Title:    "Example RSS Feed",
					Link:     "https://example.com",
					FeedLink: "https://example.com/feed.xml",
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := gateway.RegisterRSSFeedLink(context.Background(), tt.url)

			assert.Error(t, err, "Expected error for URL: %s", tt.url)
			if tt.expectedError != "" {
				assert.Contains(t, err.Error(), tt.expectedError, "Error should contain expected message")
			}
		})
	}
}

func TestRegisterFeedGateway_CircuitBreakerIntegration(t *testing.T) {
	mockFetcher := NewMockRSSFeedFetcher()
	gateway := NewRegisterFeedLinkGatewayWithFetcher(nil, mockFetcher)

	tests := []struct {
		name            string
		url             string
		failureCount    int
		expectedError   string
		expectOpenState bool
		wantErr         bool
		setupMock       func()
	}{
		{
			name:         "circuit breaker should remain closed on success",
			url:          "https://example.com/rss.xml",
			failureCount: 0,
			wantErr:      true, // Will fail at DB layer, but circuit breaker should be closed
			setupMock: func() {
				mockFetcher.SetFeed("https://example.com/rss.xml", &gofeed.Feed{
					Title:    "Example RSS Feed",
					Link:     "https://example.com",
					FeedLink: "https://example.com/rss.xml",
				})
			},
		},
		{
			name:            "circuit breaker should open after multiple failures",
			url:             "https://example.com/failing-feed.xml",
			failureCount:    5, // Should exceed default threshold
			expectedError:   "circuit breaker is open",
			expectOpenState: true,
			wantErr:         true,
			setupMock: func() {
				mockFetcher.SetError("https://example.com/failing-feed.xml", errors.New("service unavailable"))
			},
		},
		{
			name:          "circuit breaker should protect against repeated failures",
			url:           "https://example.com/unreliable-feed.xml",
			failureCount:  3,
			expectedError: "circuit breaker is open",
			wantErr:       true,
			setupMock: func() {
				mockFetcher.SetError("https://example.com/unreliable-feed.xml", errors.New("timeout"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			// Simulate multiple failures if needed
			for i := 0; i < tt.failureCount; i++ {
				gateway.RegisterRSSFeedLink(context.Background(), tt.url)
			}

			// Final attempt should be affected by circuit breaker state
			err := gateway.RegisterRSSFeedLink(context.Background(), tt.url)

			assert.Error(t, err, "Expected error for circuit breaker test")
			if tt.expectedError != "" {
				// This will fail until circuit breaker is integrated
				assert.Contains(t, err.Error(), tt.expectedError, "Error should indicate circuit breaker state")
			}
		})
	}
}

func TestRegisterFeedGateway_MetricsCollection(t *testing.T) {
	tests := []struct {
		name                 string
		operations           []string // "success" or "failure" 
		expectedTotalReqs    int64
		expectedSuccessReqs  int64
		expectedFailureReqs  int64
		expectedSuccessRate  float64
		setupMock            func(*MockRSSFeedFetcher)
	}{
		{
			name:                "metrics should track failed operations (all fail at DB layer)",
			operations:          []string{"success", "success", "success"}, // Will fail at DB layer
			expectedTotalReqs:   3,
			expectedSuccessReqs: 0, // All fail at DB layer, so no successes
			expectedFailureReqs: 3,
			expectedSuccessRate: 0.0,
			setupMock: func(mockFetcher *MockRSSFeedFetcher) {
				mockFetcher.SetFeed("https://example.com/feed.xml", &gofeed.Feed{
					Title:    "Example RSS Feed",
					Link:     "https://example.com",
					FeedLink: "https://example.com/feed.xml",
				})
			},
		},
		{
			name:                "metrics should track failed operations at fetch layer",
			operations:          []string{"failure", "failure"},
			expectedTotalReqs:   2,
			expectedSuccessReqs: 0,
			expectedFailureReqs: 2,
			expectedSuccessRate: 0.0,
			setupMock: func(mockFetcher *MockRSSFeedFetcher) {
				mockFetcher.SetError("https://example.com/failing.xml", errors.New("service error"))
			},
		},
		{
			name:                "metrics should track security validation failures",
			operations:          []string{"security_fail", "security_fail"},
			expectedTotalReqs:   2,
			expectedSuccessReqs: 0,
			expectedFailureReqs: 2,
			expectedSuccessRate: 0.0,
			setupMock: func(mockFetcher *MockRSSFeedFetcher) {
				// No mock needed - these will fail at security validation
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh instances for each test to avoid metric accumulation
			mockFetcher := NewMockRSSFeedFetcher()
			gateway := NewRegisterFeedLinkGatewayWithFetcher(nil, mockFetcher)
			
			tt.setupMock(mockFetcher)

			// Perform operations
			for _, op := range tt.operations {
				switch op {
				case "success":
					gateway.RegisterRSSFeedLink(context.Background(), "https://example.com/feed.xml")
				case "failure":
					gateway.RegisterRSSFeedLink(context.Background(), "https://example.com/failing.xml")
				case "security_fail":
					gateway.RegisterRSSFeedLink(context.Background(), "http://192.168.1.1/feed.xml") // Will fail security validation
				}
			}

			// Verify metrics collection is working
			metrics := gateway.GetMetrics()
			assert.Equal(t, tt.expectedTotalReqs, metrics.GetTotalRequests(), "Total requests should match")
			assert.Equal(t, tt.expectedSuccessReqs, metrics.GetSuccessfulRequests(), "Successful requests should match")
			assert.Equal(t, tt.expectedFailureReqs, metrics.GetFailedRequests(), "Failed requests should match")
			assert.InDelta(t, tt.expectedSuccessRate, metrics.GetSuccessRate(), 0.01, "Success rate should match")
		})
	}
}

func TestRegisterFeedGateway_IntegratedSecurityWorkflow(t *testing.T) {
	mockFetcher := NewMockRSSFeedFetcher()
	gateway := NewRegisterFeedLinkGatewayWithFetcher(nil, mockFetcher)

	tests := []struct {
		name                string
		url                 string
		expectedSecurityErr string
		expectMetrics       bool
		expectCircuitBreaker bool
		wantErr             bool
		setupMock           func()
	}{
		{
			name:                "complete security workflow for valid RSS URL",
			url:                 "https://example.com/feed.xml",  // RSS対応URL
			expectedSecurityErr: "", // Should pass RSS-specific validation
			expectMetrics:       true,
			expectCircuitBreaker: false, // Should not trigger circuit breaker
			wantErr:             true,   // Will fail at DB, but RSS validation should pass
			setupMock: func() {
				mockFetcher.SetFeed("https://example.com/feed.xml", &gofeed.Feed{
					Title:    "Example RSS Feed",
					Link:     "https://example.com",
					FeedLink: "https://example.com/feed.xml",
				})
			},
		},
		{
			name:                "security validation should block malicious URL",
			url:                 "http://127.0.0.1/feed.xml",
			expectedSecurityErr: "private network access denied",
			expectMetrics:       true, // Should still collect metrics
			expectCircuitBreaker: false,
			wantErr:             true,
			setupMock: func() {
				// Mock won't be called due to security validation
			},
		},
		{
			name:                "RSS-specific validation should work for valid atom feed",
			url:                 "https://example.com/feeds/atom.xml",  // RSS対応URL
			expectedSecurityErr: "", // Should pass RSS-specific validation
			expectMetrics:       true,
			expectCircuitBreaker: false,
			wantErr:             true,   // Will fail at DB, but RSS validation should pass
			setupMock: func() {
				mockFetcher.SetFeed("https://example.com/feeds/atom.xml", &gofeed.Feed{
					Title:    "Example Atom Feed",
					Link:     "https://example.com",
					FeedLink: "https://example.com/feeds/atom.xml",
				})
			},
		},
		{
			name:                "RSS feeds directory path should work",
			url:                 "https://example.com/feeds/rss",  // RSS対応URL (フィードディレクトリ)
			expectedSecurityErr: "", // Should pass RSS-specific validation
			expectMetrics:       true,
			expectCircuitBreaker: false,
			wantErr:             true,   // Will fail at DB, but RSS validation should pass
			setupMock: func() {
				mockFetcher.SetFeed("https://example.com/feeds/rss", &gofeed.Feed{
					Title:    "Example RSS Directory",
					Link:     "https://example.com",
					FeedLink: "https://example.com/feeds/rss",
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := gateway.RegisterRSSFeedLink(context.Background(), tt.url)

			assert.Error(t, err, "Expected error for integrated workflow test")
			
			if tt.expectedSecurityErr != "" {
				// This will fail until security integration is complete
				assert.Contains(t, err.Error(), tt.expectedSecurityErr, "Error should indicate security validation failure")
			}
		})
	}
}

func TestRegisterFeedGateway_ResponseTimeMetrics(t *testing.T) {
	mockFetcher := NewMockRSSFeedFetcher()
	gateway := NewRegisterFeedLinkGatewayWithFetcher(nil, mockFetcher)

	// Setup mock to simulate delayed response for multiple URLs
	mockFetcher.SetFeed("https://example.com/feed.xml", &gofeed.Feed{
		Title:    "Example RSS Feed",
		Link:     "https://example.com",
		FeedLink: "https://example.com/feed.xml",
	})
	mockFetcher.SetFeed("https://example.com/rss.xml", &gofeed.Feed{
		Title:    "Example RSS Feed 2",
		Link:     "https://example.com",
		FeedLink: "https://example.com/rss.xml", 
	})
	mockFetcher.SetFeed("https://example.com/feeds/atom.xml", &gofeed.Feed{
		Title:    "Example Atom Feed",
		Link:     "https://example.com",
		FeedLink: "https://example.com/feeds/atom.xml",
	})

	// Simulate multiple requests
	urls := []string{
		"https://example.com/feed.xml",
		"https://example.com/rss.xml",
		"https://example.com/feeds/atom.xml",
	}

	start := time.Now()
	for _, url := range urls {
		gateway.RegisterRSSFeedLink(context.Background(), url)
	}
	elapsed := time.Since(start)

	// Verify response time tracking is working
	metrics := gateway.GetMetrics()
	avgResponseTime := metrics.GetAverageResponseTime()
	assert.Greater(t, avgResponseTime, time.Duration(0), "Average response time should be tracked")
	assert.Less(t, avgResponseTime, elapsed, "Average response time should be reasonable")
}

func TestRegisterFeedGateway_SecurityValidationIntegration(t *testing.T) {
	mockFetcher := NewMockRSSFeedFetcher()
	gateway := NewRegisterFeedLinkGatewayWithFetcher(nil, mockFetcher)

	// Test that the gateway properly integrates URLSecurityValidator
	// This test will fail until integration is complete

	maliciousURLs := []string{
		"http://192.168.1.1/feed.xml",     // Private IP
		"http://10.0.0.1/feed.xml",        // Private IP
		"http://172.16.0.1/feed.xml",      // Private IP
		"http://localhost/feed.xml",       // Localhost
		"http://127.0.0.1/feed.xml",       // Loopback
		"ftp://example.com/feed.xml",      // Non-HTTP scheme
		"javascript:alert('xss')",         // Malicious scheme
		"file:///etc/passwd",              // File scheme
		"http://metadata.amazonaws.com/",   // Metadata server
	}

	for _, url := range maliciousURLs {
		t.Run("should block "+url, func(t *testing.T) {
			err := gateway.RegisterRSSFeedLink(context.Background(), url)
			
			// This assertion will fail until security integration is complete
			assert.Error(t, err, "Malicious URL should be blocked: %s", url)
			assert.True(t, 
				strings.Contains(err.Error(), "private network access denied") ||
				strings.Contains(err.Error(), "only HTTP and HTTPS schemes allowed") ||
				strings.Contains(err.Error(), "metadata server access denied") ||
				strings.Contains(err.Error(), "invalid URL format"),
				"Error should indicate security validation failure for URL: %s, got: %s", url, err.Error())
		})
	}
}

// TDD RED PHASE: Test to reproduce URL construction bug
func TestDefaultRSSFeedFetcher_ConvertToProxyURL_URLConstruction(t *testing.T) {
	tests := []struct {
		name        string
		originalURL string
		baseURL     string
		expected    string
	}{
		{
			name:        "HTTPS RSS URL should have correct double slash",
			originalURL: "https://example.com/feed.xml",
			baseURL:     "http://envoy-proxy.alt-apps.svc.cluster.local:8085",
			expected:    "http://envoy-proxy.alt-apps.svc.cluster.local:8085/proxy/https://example.com/feed.xml",
		},
		{
			name:        "HTTP URL should have correct double slash",
			originalURL: "http://example.com/rss.xml",
			baseURL:     "http://envoy-proxy.alt-apps.svc.cluster.local:8085",
			expected:    "http://envoy-proxy.alt-apps.svc.cluster.local:8085/proxy/http://example.com/rss.xml",
		},
		{
			name:        "RSS URL with query parameters should be preserved",
			originalURL: "https://example.com/feeds/rss.xml?format=atom",
			baseURL:     "http://envoy-proxy.alt-apps.svc.cluster.local:8085",
			expected:    "http://envoy-proxy.alt-apps.svc.cluster.local:8085/proxy/https://example.com/feeds/rss.xml?format=atom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock fetcher and strategy
			fetcher := &DefaultRSSFeedFetcher{
				proxyConfig:      nil,
				envoyProxyConfig: nil,
				proxyStrategy:    nil,
			}
			strategy := &ProxyStrategy{
				Mode:    ProxyModeEnvoy,
				BaseURL: tt.baseURL,
			}

			result := fetcher.convertToProxyURL(tt.originalURL, strategy)
			
			// This assertion will fail if URL construction has bugs
			assert.Equal(t, tt.expected, result, 
				"URL construction should produce correct proxy URL format")
		})
	}
}