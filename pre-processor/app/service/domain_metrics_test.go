// TDD Phase 5: Domain-specific error classification tracking - Tests
// ABOUTME: Tests for domain-specific metrics tracking and bot detection patterns

package service

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestProxyMetrics_RecordDomainRequest(t *testing.T) {
	logger := slog.Default()
	metrics := NewProxyMetrics(logger)

	tests := []struct {
		name         string
		requestURL   string
		duration     time.Duration
		success      bool
		errorType    ProxyErrorType
		expectDomain string
	}{
		{
			name:         "successful ZDNet request",
			requestURL:   "https://www.zdnet.com/article/test-article",
			duration:     500 * time.Millisecond,
			success:      true,
			errorType:    ProxyErrorConfig, // No error for success
			expectDomain: "www.zdnet.com",
		},
		{
			name:         "bot detection from TechCrunch",
			requestURL:   "https://techcrunch.com/2024/01/01/test",
			duration:     200 * time.Millisecond,
			success:      false,
			errorType:    ProxyErrorConnection, // Bot detection
			expectDomain: "techcrunch.com",
		},
		{
			name:         "timeout from CNN",
			requestURL:   "https://cnn.com/news/test",
			duration:     30 * time.Second,
			success:      false,
			errorType:    ProxyErrorTimeout,
			expectDomain: "cnn.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Record domain request
			metrics.RecordDomainRequest(tt.requestURL, tt.duration, tt.success, tt.errorType)

			// Get domain metrics summary
			summary := metrics.GetDomainMetricsSummary()

			// Verify domain was tracked
			domainMetrics, exists := summary.DomainBreakdown[tt.expectDomain]
			if !exists {
				t.Fatalf("Expected domain %s to be tracked", tt.expectDomain)
			}

			// Verify basic metrics
			if domainMetrics.TotalRequests != 1 {
				t.Errorf("Expected 1 total request, got %d", domainMetrics.TotalRequests)
			}

			if tt.success {
				if domainMetrics.SuccessfulRequests != 1 {
					t.Errorf("Expected 1 successful request, got %d", domainMetrics.SuccessfulRequests)
				}
				if domainMetrics.SuccessRate != 100.0 {
					t.Errorf("Expected 100%% success rate, got %.2f", domainMetrics.SuccessRate)
				}
			} else {
				if domainMetrics.FailedRequests != 1 {
					t.Errorf("Expected 1 failed request, got %d", domainMetrics.FailedRequests)
				}
				if domainMetrics.SuccessRate != 0.0 {
					t.Errorf("Expected 0%% success rate, got %.2f", domainMetrics.SuccessRate)
				}
			}

			// Verify average latency
			expectedLatencyMs := float64(tt.duration.Milliseconds())
			if domainMetrics.AvgLatencyMs != expectedLatencyMs {
				t.Errorf("Expected avg latency %.2f ms, got %.2f ms",
					expectedLatencyMs, domainMetrics.AvgLatencyMs)
			}
		})
	}
}

func TestDomainMetrics_BotDetectionSuspicion(t *testing.T) {
	logger := slog.Default()
	metrics := NewProxyMetrics(logger)

	testURL := "https://problematic-domain.com/article"

	// Record multiple bot detection errors
	for i := 0; i < 12; i++ {
		metrics.RecordDomainRequest(testURL, 100*time.Millisecond, false, ProxyErrorConnection)
	}

	summary := metrics.GetDomainMetricsSummary()
	domainMetrics := summary.DomainBreakdown["problematic-domain.com"]

	// Should be suspected of bot detection
	if !domainMetrics.IsBotDetectionSuspected() {
		t.Error("Expected domain to be suspected of bot detection")
	}

	// Should have low health score
	healthScore := domainMetrics.GetHealthScore()
	if healthScore > 50 {
		t.Errorf("Expected low health score for problematic domain, got %.2f", healthScore)
	}

	// Should appear in top error domains
	if summary.BotDetectionDomains != 1 {
		t.Errorf("Expected 1 bot detection domain, got %d", summary.BotDetectionDomains)
	}

	if len(summary.TopErrorDomains) == 0 {
		t.Error("Expected domain to appear in top error domains")
	}
}

func TestDomainMetricsSummary_HealthClassification(t *testing.T) {
	logger := slog.Default()
	metrics := NewProxyMetrics(logger)

	// Healthy domain
	for i := 0; i < 10; i++ {
		metrics.RecordDomainRequest("https://healthy-domain.com/article",
			100*time.Millisecond, true, ProxyErrorConfig)
	}

	// Problematic domain
	for i := 0; i < 10; i++ {
		success := i < 3 // Only 30% success rate
		errorType := ProxyErrorConfig
		if !success {
			errorType = ProxyErrorConnection
		}
		metrics.RecordDomainRequest("https://problematic-domain.com/article",
			2*time.Second, success, errorType)
	}

	summary := metrics.GetDomainMetricsSummary()

	// Should have correct classification counts
	if summary.TotalDomains != 2 {
		t.Errorf("Expected 2 total domains, got %d", summary.TotalDomains)
	}

	if summary.HealthyDomains != 1 {
		t.Errorf("Expected 1 healthy domain, got %d", summary.HealthyDomains)
	}

	if summary.ProblematicDomains != 1 {
		t.Errorf("Expected 1 problematic domain, got %d", summary.ProblematicDomains)
	}

	// Verify overall health score
	if summary.OverallDomainHealthScore < 30 || summary.OverallDomainHealthScore > 70 {
		t.Errorf("Expected moderate overall health score, got %.2f",
			summary.OverallDomainHealthScore)
	}
}

func TestProxyMetrics_ExtractDomain(t *testing.T) {
	logger := slog.Default()
	metrics := NewProxyMetrics(logger)

	tests := []struct {
		name           string
		requestURL     string
		expectedDomain string
	}{
		{
			name:           "basic HTTPS URL",
			requestURL:     "https://www.example.com/path",
			expectedDomain: "www.example.com",
		},
		{
			name:           "HTTP URL",
			requestURL:     "http://example.com",
			expectedDomain: "example.com",
		},
		{
			name:           "URL with port",
			requestURL:     "https://localhost:8080/api",
			expectedDomain: "localhost",
		},
		{
			name:           "invalid URL",
			requestURL:     "not-a-url",
			expectedDomain: "",
		},
		{
			name:           "empty URL",
			requestURL:     "",
			expectedDomain: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domain := metrics.extractDomain(tt.requestURL)
			if domain != tt.expectedDomain {
				t.Errorf("Expected domain %q, got %q", tt.expectedDomain, domain)
			}
		})
	}
}

func TestProxyMetrics_ConsecutiveErrorsTracking(t *testing.T) {
	logger := slog.Default()
	metrics := NewProxyMetrics(logger)

	testURL := "https://error-prone-domain.com/article"

	// Record 5 consecutive errors
	for i := 0; i < 5; i++ {
		metrics.RecordDomainRequest(testURL, 100*time.Millisecond, false, ProxyErrorTimeout)
	}

	summary := metrics.GetDomainMetricsSummary()
	domainMetrics := summary.DomainBreakdown["error-prone-domain.com"]

	if domainMetrics.ConsecutiveErrors != 5 {
		t.Errorf("Expected 5 consecutive errors, got %d", domainMetrics.ConsecutiveErrors)
	}

	// Now record a success - should reset consecutive errors
	metrics.RecordDomainRequest(testURL, 100*time.Millisecond, true, ProxyErrorConfig)

	summary = metrics.GetDomainMetricsSummary()
	domainMetrics = summary.DomainBreakdown["error-prone-domain.com"]

	if domainMetrics.ConsecutiveErrors != 0 {
		t.Errorf("Expected consecutive errors to reset to 0, got %d", domainMetrics.ConsecutiveErrors)
	}
}

func TestProxyMetrics_DomainMetricsLogging(t *testing.T) {
	// Create a logger that captures output
	var logOutput strings.Builder
	logger := slog.New(slog.NewTextHandler(&logOutput, nil))
	metrics := NewProxyMetrics(logger)

	testURL := "https://logging-test-domain.com/article"

	// Record requests to trigger logging (every 25 requests)
	for i := 0; i < 26; i++ {
		success := i%2 == 0 // 50% success rate
		errorType := ProxyErrorConfig
		if !success {
			errorType = ProxyErrorConnection
		}
		metrics.RecordDomainRequest(testURL, 100*time.Millisecond, success, errorType)
	}

	// Verify logging occurred
	logContent := logOutput.String()
	if !strings.Contains(logContent, "domain-specific metrics") {
		t.Error("Expected domain-specific metrics to be logged")
	}

	if !strings.Contains(logContent, "logging-test-domain.com") {
		t.Error("Expected domain name to be logged")
	}
}
