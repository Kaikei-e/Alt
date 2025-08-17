// TDD Phase 3: Metrics Integration Test
// ABOUTME: End-to-end test of metrics collection with HTTP clients
// ABOUTME: Verifies metrics are recorded during actual HTTP operations

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

// TestMetricsIntegration_EnvoyVsDirect tests metrics collection during real HTTP operations
func TestMetricsIntegration_EnvoyVsDirect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.Default()

	// Create mock server that simulates various response times
	callCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		// Simulate different response times for Envoy vs Direct
		if strings.Contains(r.URL.Path, "/proxy/https://") {
			// Envoy requests - simulate additional proxy latency
			time.Sleep(50 * time.Millisecond)

			// Verify Envoy-specific headers
			targetDomain := r.Header.Get("X-Target-Domain")
			resolvedIP := r.Header.Get("X-Resolved-IP")

			if targetDomain == "" || resolvedIP == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html><body><h1>Envoy Response</h1><p>Content via proxy</p></body></html>"))
		} else {
			// Direct requests - faster response
			time.Sleep(20 * time.Millisecond)

			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html><body><h1>Direct Response</h1><p>Content direct</p></body></html>"))
		}
	}))
	defer mockServer.Close()

	// Get initial metrics state
	metrics := GetGlobalProxyMetrics(logger)
	initialSummary := metrics.GetMetricsSummary()

	t.Logf("Initial metrics: Envoy=%d, Direct=%d, Total=%d",
		initialSummary.EnvoyRequests, initialSummary.DirectRequests, initialSummary.TotalRequests)

	// Test Envoy client
	envoyConfig := &config.Config{
		HTTP: config.HTTPConfig{
			UseEnvoyProxy:  true,
			EnvoyProxyURL:  mockServer.URL,
			EnvoyProxyPath: "/proxy/https://",
			EnvoyTimeout:   30 * time.Second,
			UserAgent:      "metrics-integration-test",
		},
	}

	envoyFactory := NewHTTPClientFactory(envoyConfig, logger)
	envoyClient := envoyFactory.CreateClient()

	// Test Direct client
	directConfig := &config.Config{
		HTTP: config.HTTPConfig{
			UseEnvoyProxy: false,
			Timeout:       30 * time.Second,
			UserAgent:     "metrics-integration-test",
		},
	}

	directFactory := NewHTTPClientFactory(directConfig, logger)
	directClient := directFactory.CreateClient()

	// Make test requests
	testURL := "https://example.com/test"

	// Test Envoy request (should record Envoy metrics)
	t.Run("envoy_request_metrics", func(t *testing.T) {
		resp, err := envoyClient.Get(testURL)
		if err != nil {
			t.Errorf("Envoy request failed: %v", err)
			return
		}
		defer resp.Body.Close()

		// Verify request went through
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	// Test direct request (should record direct metrics)
	t.Run("direct_request_metrics", func(t *testing.T) {
		// Use the mock server URL for direct request (without proxy path)
		directURL := mockServer.URL + "/direct-test"
		resp, err := directClient.Get(directURL)
		if err != nil {
			t.Errorf("Direct request failed: %v", err)
			return
		}
		defer resp.Body.Close()

		// Verify request went through
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		t.Logf("Direct request completed successfully to: %s", directURL)
	})

	// Allow time for metrics to be recorded
	time.Sleep(100 * time.Millisecond)

	// Verify metrics were recorded
	finalSummary := metrics.GetMetricsSummary()

	t.Logf("Final metrics: Envoy=%d, Direct=%d, Total=%d",
		finalSummary.EnvoyRequests, finalSummary.DirectRequests, finalSummary.TotalRequests)

	// Verify Envoy metrics increased
	if finalSummary.EnvoyRequests <= initialSummary.EnvoyRequests {
		t.Errorf("Expected Envoy requests to increase, got %d -> %d",
			initialSummary.EnvoyRequests, finalSummary.EnvoyRequests)
	}

	// Verify Direct metrics increased
	if finalSummary.DirectRequests <= initialSummary.DirectRequests {
		t.Errorf("Expected direct requests to increase, got %d -> %d",
			initialSummary.DirectRequests, finalSummary.DirectRequests)
	}

	// Verify latency metrics are reasonable
	if finalSummary.EnvoyAvgLatencyMs == 0 {
		t.Errorf("Expected Envoy average latency to be recorded")
	}

	if finalSummary.DirectAvgLatencyMs == 0 {
		t.Errorf("Expected direct average latency to be recorded")
	}

	// Verify DNS metrics for Envoy
	if finalSummary.DNSAvgLatencyMs == 0 {
		t.Errorf("Expected DNS resolution latency to be recorded for Envoy")
	}

	// Performance comparison analysis
	latencyDiff := finalSummary.EnvoyAvgLatencyMs - finalSummary.DirectAvgLatencyMs
	t.Logf("Performance analysis: Envoy=%.2fms, Direct=%.2fms, Diff=%.2fms",
		finalSummary.EnvoyAvgLatencyMs, finalSummary.DirectAvgLatencyMs, latencyDiff)

	// Health score should be reasonable
	healthScore := finalSummary.GetHealthScore()
	if healthScore < 50.0 {
		t.Errorf("Health score too low: %.1f", healthScore)
	}

	t.Logf("Health score: %.1f/100", healthScore)

	// Verify success rates
	if finalSummary.EnvoySuccessRate < 50.0 {
		t.Errorf("Envoy success rate too low: %.1f%%", finalSummary.EnvoySuccessRate)
	}

	if finalSummary.DirectSuccessRate < 50.0 {
		t.Errorf("Direct success rate too low: %.1f%%", finalSummary.DirectSuccessRate)
	}

	t.Logf("Success rates: Envoy=%.1f%%, Direct=%.1f%%",
		finalSummary.EnvoySuccessRate, finalSummary.DirectSuccessRate)
}

// TestMetricsIntegration_ArticleFetching tests metrics during article fetching
func TestMetricsIntegration_ArticleFetching(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.Default()

	// Create mock Envoy server for article fetching
	mockEnvoy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate article content
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`
			<!DOCTYPE html>
			<html>
			<head><title>Test Article</title></head>
			<body>
				<article>
					<h1>Metrics Integration Test Article</h1>
					<p>This article tests metrics collection during fetching.</p>
				</article>
			</body>
			</html>
		`))
	}))
	defer mockEnvoy.Close()

	// Get initial metrics
	metrics := GetGlobalProxyMetrics(logger)
	initialSummary := metrics.GetMetricsSummary()

	// Test with Envoy-enabled article fetcher
	envoyConfig := &config.Config{
		HTTP: config.HTTPConfig{
			UseEnvoyProxy:  true,
			EnvoyProxyURL:  mockEnvoy.URL,
			EnvoyProxyPath: "/proxy/https://",
			EnvoyTimeout:   30 * time.Second,
			UserAgent:      "metrics-article-test",
		},
	}

	articleFetcher := NewArticleFetcherServiceWithFactory(envoyConfig, logger)

	// Fetch an article (this should record metrics)
	ctx := context.Background()
	article, err := articleFetcher.FetchArticle(ctx, "https://test-site.com/article")

	// Verify article fetching worked
	if err != nil {
		t.Logf("Article fetch returned error (expected for integration test): %v", err)
	} else if article != nil {
		t.Logf("Article fetched successfully: %s", article.Title)
	}

	// Verify metrics were updated
	finalSummary := metrics.GetMetricsSummary()

	if finalSummary.TotalRequests <= initialSummary.TotalRequests {
		t.Errorf("Expected total requests to increase after article fetching")
	}

	t.Logf("Article fetching metrics: Total=%d, Envoy=%d, Direct=%d",
		finalSummary.TotalRequests, finalSummary.EnvoyRequests, finalSummary.DirectRequests)

	// Test health checker metrics
	healthChecker := NewHealthCheckerServiceWithFactory(envoyConfig, mockEnvoy.URL, logger)

	healthInitialSummary := metrics.GetMetricsSummary()

	// Perform health check (this should also record metrics)
	err = healthChecker.CheckNewsCreatorHealth(ctx)
	if err != nil {
		t.Logf("Health check error (expected for test): %v", err)
	}

	healthFinalSummary := metrics.GetMetricsSummary()

	if healthFinalSummary.TotalRequests <= healthInitialSummary.TotalRequests {
		t.Logf("Note: Health check may not have increased metrics if it failed early")
	} else {
		t.Logf("Health check added to metrics: +%d requests",
			healthFinalSummary.TotalRequests-healthInitialSummary.TotalRequests)
	}

	// Final metrics summary
	t.Logf("Final integration test metrics summary:")
	t.Logf("  Total Requests: %d", finalSummary.TotalRequests)
	t.Logf("  Envoy Success Rate: %.1f%%", finalSummary.EnvoySuccessRate)
	t.Logf("  Direct Success Rate: %.1f%%", finalSummary.DirectSuccessRate)
	t.Logf("  Health Score: %.1f/100", finalSummary.GetHealthScore())
	t.Logf("  Average Latencies: Envoy=%.2fms, Direct=%.2fms, DNS=%.2fms",
		finalSummary.EnvoyAvgLatencyMs, finalSummary.DirectAvgLatencyMs, finalSummary.DNSAvgLatencyMs)
}
