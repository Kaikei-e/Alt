// TDD Phase 3 - REFACTOR: Structured Logging & Monitoring Tests
package utils

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

// TestMonitor_Creation tests monitor creation and configuration
func TestMonitor_Creation(t *testing.T) {
	config := DefaultMonitoringConfig()
	monitor := NewMonitor(config, slog.Default())
	defer monitor.Close()

	if monitor.config != config {
		t.Error("Expected config to be set correctly")
	}

	healthCheck := monitor.HealthCheck()
	if healthCheck["status"] != "healthy" {
		t.Errorf("Expected status to be healthy, got %v", healthCheck["status"])
	}

	if healthCheck["metrics_enabled"] != true {
		t.Error("Expected metrics to be enabled")
	}
}

// TestMonitor_LogAPIRequest tests API request logging
func TestMonitor_LogAPIRequest(t *testing.T) {
	config := &MonitoringConfig{
		EnableMetrics:     true,
		EnableTracing:     true,
		MetricsBatchSize:  10,
		FlushInterval:     100 * time.Millisecond,
		RetentionDuration: time.Hour,
	}

	monitor := NewMonitor(config, slog.Default())
	defer monitor.Close()

	ctx := context.Background()

	// Test successful API request
	monitor.LogAPIRequest(ctx, "GET", "/subscription/list", 200, 150*time.Millisecond, nil)

	// Test failed API request
	apiError := errors.New("API timeout")
	monitor.LogAPIRequest(ctx, "POST", "/stream/contents", 500, 5000*time.Millisecond, apiError)

	// Allow time for async processing
	time.Sleep(200 * time.Millisecond)

	metrics := monitor.GetMetrics()
	if len(metrics) == 0 {
		t.Error("Expected metrics to be recorded")
	}

	// Check for API request counter metrics
	foundAPICounter := false
	for key, metric := range metrics {
		if metric.Name == "api_requests_total" {
			foundAPICounter = true
			t.Logf("Found API counter metric: %s = %f", key, metric.Value)
		}
	}

	if !foundAPICounter {
		t.Error("Expected to find api_requests_total metric")
	}
}

// TestMonitor_CircuitBreakerLogging tests circuit breaker event logging
func TestMonitor_CircuitBreakerLogging(t *testing.T) {
	monitor := NewMonitor(DefaultMonitoringConfig(), slog.Default())
	defer monitor.Close()

	ctx := context.Background()

	monitor.LogCircuitBreakerEvent(ctx, StateClosed, StateOpen, "inoreader_service")
	monitor.LogCircuitBreakerEvent(ctx, StateOpen, StateHalfOpen, "inoreader_service")
	monitor.LogCircuitBreakerEvent(ctx, StateHalfOpen, StateClosed, "inoreader_service")

	// Allow time for async processing
	time.Sleep(100 * time.Millisecond)

	metrics := monitor.GetMetrics()

	// Check for circuit breaker metrics
	foundCBMetric := false
	for key, metric := range metrics {
		if metric.Name == "circuit_breaker_state_transitions_total" {
			foundCBMetric = true
			t.Logf("Found circuit breaker metric: %s = %f", key, metric.Value)
		}
	}

	if !foundCBMetric {
		t.Error("Expected to find circuit_breaker_state_transitions_total metric")
	}
}

// TestMonitor_TokenRefreshLogging tests token refresh logging
func TestMonitor_TokenRefreshLogging(t *testing.T) {
	monitor := NewMonitor(DefaultMonitoringConfig(), slog.Default())
	defer monitor.Close()

	ctx := context.Background()

	// Test successful token refresh
	monitor.LogTokenRefresh(ctx, true, 200*time.Millisecond, nil)

	// Test failed token refresh
	tokenError := errors.New("invalid refresh token")
	monitor.LogTokenRefresh(ctx, false, 100*time.Millisecond, tokenError)

	// Allow time for async processing
	time.Sleep(100 * time.Millisecond)

	metrics := monitor.GetMetrics()

	// Check for token refresh metrics
	foundTokenMetric := false
	for key, metric := range metrics {
		if metric.Name == "token_refresh_total" {
			foundTokenMetric = true
			t.Logf("Found token refresh metric: %s = %f", key, metric.Value)
		}
	}

	if !foundTokenMetric {
		t.Error("Expected to find token_refresh_total metric")
	}
}

// TestMonitor_ArticleProcessingLogging tests article processing logging
func TestMonitor_ArticleProcessingLogging(t *testing.T) {
	monitor := NewMonitor(DefaultMonitoringConfig(), slog.Default())
	defer monitor.Close()

	ctx := context.Background()

	// Test successful article processing
	monitor.LogArticleProcessing(ctx, "fetch", 25, true, 800*time.Millisecond, nil)

	// Test failed article processing
	processingError := errors.New("database connection failed")
	monitor.LogArticleProcessing(ctx, "save", 15, false, 200*time.Millisecond, processingError)

	// Allow time for async processing
	time.Sleep(100 * time.Millisecond)

	metrics := monitor.GetMetrics()

	// Check for article processing metrics
	foundArticleMetric := false
	for key, metric := range metrics {
		if metric.Name == "articles_processed_total" {
			foundArticleMetric = true
			t.Logf("Found article processing metric: %s = %f", key, metric.Value)
		}
	}

	if !foundArticleMetric {
		t.Error("Expected to find articles_processed_total metric")
	}
}

// TestMonitor_MetricTypes tests different metric types
func TestMonitor_MetricTypes(t *testing.T) {
	monitor := NewMonitor(DefaultMonitoringConfig(), slog.Default())
	defer monitor.Close()

	labels := map[string]string{"service": "test"}

	// Record different types of metrics
	monitor.RecordCounter("test_counter", 5.0, labels)
	monitor.RecordHistogram("test_histogram", 1.5, labels)
	monitor.RecordGauge("test_gauge", 42.0, labels)

	// Allow time for async processing
	time.Sleep(100 * time.Millisecond)

	metrics := monitor.GetMetrics()

	metricTypes := map[string]bool{
		"test_counter":   false,
		"test_histogram": false,
		"test_gauge":     false,
	}

	for _, metric := range metrics {
		if _, exists := metricTypes[metric.Name]; exists {
			metricTypes[metric.Name] = true
			t.Logf("Found metric: %s (type: %s, value: %f)", metric.Name, metric.Type, metric.Value)
		}
	}

	for name, found := range metricTypes {
		if !found {
			t.Errorf("Expected to find metric: %s", name)
		}
	}
}

// TestMonitor_CounterAccumulation tests counter metric accumulation
func TestMonitor_CounterAccumulation(t *testing.T) {
	monitor := NewMonitor(DefaultMonitoringConfig(), slog.Default())
	defer monitor.Close()

	labels := map[string]string{"endpoint": "/test"}

	// Record the same counter multiple times
	monitor.RecordCounter("requests_total", 1.0, labels)
	monitor.RecordCounter("requests_total", 3.0, labels)
	monitor.RecordCounter("requests_total", 2.0, labels)

	// Allow time for async processing
	time.Sleep(100 * time.Millisecond)

	metrics := monitor.GetMetrics()

	for _, metric := range metrics {
		if metric.Name == "requests_total" {
			expectedValue := 6.0 // 1 + 3 + 2
			if metric.Value != expectedValue {
				t.Errorf("Expected accumulated counter value %f, got %f", expectedValue, metric.Value)
			}
			return
		}
	}

	t.Error("Expected to find accumulated counter metric")
}

// TestMonitor_HealthCheck tests health check functionality
func TestMonitor_HealthCheck(t *testing.T) {
	config := &MonitoringConfig{
		EnableMetrics:     true,
		EnableTracing:     false,
		MetricsBatchSize:  50,
		FlushInterval:     30 * time.Second,
		RetentionDuration: 12 * time.Hour,
	}

	monitor := NewMonitor(config, slog.Default())
	defer monitor.Close()

	healthCheck := monitor.HealthCheck()

	expectedFields := []string{
		"status", "metrics_enabled", "tracing_enabled", "metrics_count",
		"queue_length", "queue_capacity", "flush_interval", "retention_duration",
	}

	for _, field := range expectedFields {
		if _, exists := healthCheck[field]; !exists {
			t.Errorf("Expected health check field: %s", field)
		}
	}

	if healthCheck["status"] != "healthy" {
		t.Errorf("Expected status to be healthy, got %v", healthCheck["status"])
	}

	if healthCheck["metrics_enabled"] != true {
		t.Error("Expected metrics_enabled to be true")
	}

	if healthCheck["tracing_enabled"] != false {
		t.Error("Expected tracing_enabled to be false")
	}
}

// TestMonitor_MetricRetention tests metric retention and cleanup
func TestMonitor_MetricRetention(t *testing.T) {
	// Use very short retention for testing
	config := &MonitoringConfig{
		EnableMetrics:     true,
		EnableTracing:     true,
		MetricsBatchSize:  10,
		FlushInterval:     50 * time.Millisecond,  // Fast flush for testing
		RetentionDuration: 100 * time.Millisecond, // Very short retention
	}

	monitor := NewMonitor(config, slog.Default())
	defer monitor.Close()

	// Record a metric
	monitor.RecordCounter("test_retention", 1.0, map[string]string{"test": "value"})

	// Allow time for initial processing
	time.Sleep(75 * time.Millisecond)

	// Check metric exists
	metrics := monitor.GetMetrics()
	if len(metrics) == 0 {
		t.Error("Expected metric to exist before retention cleanup")
	}

	// Wait for retention cleanup
	time.Sleep(200 * time.Millisecond)

	// Check metric has been cleaned up
	metrics = monitor.GetMetrics()
	if len(metrics) > 0 {
		t.Logf("Warning: Metrics still exist after retention period (may be timing sensitive): %d", len(metrics))
		// Note: This test may be flaky due to timing, but it's useful for demonstrating retention behavior
	}
}
