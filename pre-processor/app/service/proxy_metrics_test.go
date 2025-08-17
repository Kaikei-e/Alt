// TDD Phase 3: Proxy Metrics Tests
// ABOUTME: Comprehensive tests for proxy latency monitoring and metrics collection
// ABOUTME: Verifies metrics accuracy, performance tracking, and health scoring

package service

import (
	"log/slog"
	"testing"
	"time"
)

// TestProxyMetrics_RecordEnvoyRequest tests Envoy request recording
func TestProxyMetrics_RecordEnvoyRequest(t *testing.T) {
	logger := slog.Default()
	metrics := NewProxyMetrics(logger)

	tests := map[string]struct {
		duration          time.Duration
		success           bool
		dnsResolutionTime time.Duration
		description       string
	}{
		"successful_envoy_request": {
			duration:          100 * time.Millisecond,
			success:           true,
			dnsResolutionTime: 10 * time.Millisecond,
			description:       "Should record successful Envoy request with DNS timing",
		},
		"failed_envoy_request": {
			duration:          500 * time.Millisecond,
			success:           false,
			dnsResolutionTime: 20 * time.Millisecond,
			description:       "Should record failed Envoy request",
		},
		"zero_dns_time": {
			duration:          200 * time.Millisecond,
			success:           true,
			dnsResolutionTime: 0,
			description:       "Should handle zero DNS resolution time",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Record the request
			metrics.RecordEnvoyRequest(tc.duration, tc.success, tc.dnsResolutionTime)

			// Verify metrics
			summary := metrics.GetMetricsSummary()

			if summary.EnvoyRequests == 0 {
				t.Errorf("%s: expected Envoy requests to be recorded", tc.description)
			}

			if tc.success && summary.EnvoySuccessful == 0 {
				t.Errorf("%s: expected successful request to be recorded", tc.description)
			}

			if !tc.success && summary.EnvoyFailures == 0 {
				t.Errorf("%s: expected failed request to be recorded", tc.description)
			}

			if tc.dnsResolutionTime > 0 && summary.DNSAvgLatencyMs == 0 {
				t.Errorf("%s: expected DNS latency to be recorded", tc.description)
			}

			t.Logf("%s: recorded - Duration: %v, Success: %v, DNS: %v",
				tc.description, tc.duration, tc.success, tc.dnsResolutionTime)
		})
	}
}

// TestProxyMetrics_RecordDirectRequest tests direct HTTP request recording
func TestProxyMetrics_RecordDirectRequest(t *testing.T) {
	logger := slog.Default()
	metrics := NewProxyMetrics(logger)

	tests := map[string]struct {
		duration    time.Duration
		success     bool
		description string
	}{
		"successful_direct_request": {
			duration:    150 * time.Millisecond,
			success:     true,
			description: "Should record successful direct request",
		},
		"failed_direct_request": {
			duration:    300 * time.Millisecond,
			success:     false,
			description: "Should record failed direct request",
		},
		"fast_direct_request": {
			duration:    50 * time.Millisecond,
			success:     true,
			description: "Should record fast direct request",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Record the request
			metrics.RecordDirectRequest(tc.duration, tc.success)

			// Verify metrics
			summary := metrics.GetMetricsSummary()

			if summary.DirectRequests == 0 {
				t.Errorf("%s: expected direct requests to be recorded", tc.description)
			}

			if tc.success && summary.DirectSuccessful == 0 {
				t.Errorf("%s: expected successful request to be recorded", tc.description)
			}

			if !tc.success && summary.DirectFailures == 0 {
				t.Errorf("%s: expected failed request to be recorded", tc.description)
			}

			t.Logf("%s: recorded - Duration: %v, Success: %v",
				tc.description, tc.duration, tc.success)
		})
	}
}

// TestProxyMetrics_RecordError tests error categorization and tracking
func TestProxyMetrics_RecordError(t *testing.T) {
	logger := slog.Default()
	metrics := NewProxyMetrics(logger)

	tests := map[string]struct {
		errorType   ProxyErrorType
		description string
	}{
		"config_error": {
			errorType:   ProxyErrorConfig,
			description: "Should record configuration error",
		},
		"timeout_error": {
			errorType:   ProxyErrorTimeout,
			description: "Should record timeout error",
		},
		"connection_error": {
			errorType:   ProxyErrorConnection,
			description: "Should record connection error",
		},
		"dns_error": {
			errorType:   ProxyErrorDNS,
			description: "Should record DNS error",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			initialSummary := metrics.GetMetricsSummary()

			// Record the error
			metrics.RecordError(tc.errorType)

			// Verify error was recorded
			summary := metrics.GetMetricsSummary()

			switch tc.errorType {
			case ProxyErrorConfig:
				if summary.ConfigErrors <= initialSummary.ConfigErrors {
					t.Errorf("%s: expected config error count to increase", tc.description)
				}
			case ProxyErrorTimeout:
				if summary.TimeoutErrors <= initialSummary.TimeoutErrors {
					t.Errorf("%s: expected timeout error count to increase", tc.description)
				}
			case ProxyErrorConnection:
				if summary.ConnectionErrors <= initialSummary.ConnectionErrors {
					t.Errorf("%s: expected connection error count to increase", tc.description)
				}
			case ProxyErrorDNS:
				if summary.DNSErrors <= initialSummary.DNSErrors {
					t.Errorf("%s: expected DNS error count to increase", tc.description)
				}
			}

			t.Logf("%s: recorded error type: %s", tc.description, tc.errorType)
		})
	}
}

// TestProxyMetrics_ConfigSwitch tests configuration switch recording
func TestProxyMetrics_ConfigSwitch(t *testing.T) {
	logger := slog.Default()
	metrics := NewProxyMetrics(logger)

	initialSummary := metrics.GetMetricsSummary()
	initialSwitchCount := initialSummary.ConfigSwitchCount

	// Record config switch from direct to Envoy
	metrics.RecordConfigSwitch(false, true)

	summary := metrics.GetMetricsSummary()

	if summary.ConfigSwitchCount != initialSwitchCount+1 {
		t.Errorf("Expected config switch count to increase by 1, got %d -> %d",
			initialSwitchCount, summary.ConfigSwitchCount)
	}

	// Record another switch from Envoy to direct
	metrics.RecordConfigSwitch(true, false)

	summary = metrics.GetMetricsSummary()

	if summary.ConfigSwitchCount != initialSwitchCount+2 {
		t.Errorf("Expected config switch count to increase by 2, got %d -> %d",
			initialSwitchCount, summary.ConfigSwitchCount)
	}

	t.Logf("Configuration switches recorded successfully: %d total", summary.ConfigSwitchCount)
}

// TestProxyMetrics_PerformanceComparison tests performance analysis
func TestProxyMetrics_PerformanceComparison(t *testing.T) {
	logger := slog.Default()
	metrics := NewProxyMetrics(logger)

	// Record multiple Envoy requests (generally slower)
	envoyTimes := []time.Duration{
		200 * time.Millisecond,
		250 * time.Millisecond,
		300 * time.Millisecond,
	}

	// Record multiple direct requests (generally faster)
	directTimes := []time.Duration{
		100 * time.Millisecond,
		120 * time.Millisecond,
		150 * time.Millisecond,
	}

	for _, duration := range envoyTimes {
		metrics.RecordEnvoyRequest(duration, true, 10*time.Millisecond)
	}

	for _, duration := range directTimes {
		metrics.RecordDirectRequest(duration, true)
	}

	summary := metrics.GetMetricsSummary()

	// Verify basic counts
	if summary.EnvoyRequests != uint64(len(envoyTimes)) {
		t.Errorf("Expected %d Envoy requests, got %d", len(envoyTimes), summary.EnvoyRequests)
	}

	if summary.DirectRequests != uint64(len(directTimes)) {
		t.Errorf("Expected %d direct requests, got %d", len(directTimes), summary.DirectRequests)
	}

	// Verify averages are reasonable
	if summary.EnvoyAvgLatencyMs == 0 {
		t.Errorf("Expected Envoy average latency to be calculated")
	}

	if summary.DirectAvgLatencyMs == 0 {
		t.Errorf("Expected direct average latency to be calculated")
	}

	// In this test, Envoy should be slower
	if summary.EnvoyAvgLatencyMs <= summary.DirectAvgLatencyMs {
		t.Logf("Note: Envoy was actually faster than direct (%.2f ms vs %.2f ms)",
			summary.EnvoyAvgLatencyMs, summary.DirectAvgLatencyMs)
	}

	t.Logf("Performance comparison - Envoy: %.2f ms avg, Direct: %.2f ms avg",
		summary.EnvoyAvgLatencyMs, summary.DirectAvgLatencyMs)
}

// TestProxyMetrics_HealthScore tests health scoring algorithm
func TestProxyMetrics_HealthScore(t *testing.T) {
	tests := map[string]struct {
		setupMetrics func() *ProxyMetricsSummary
		expectedMin  float64
		expectedMax  float64
		description  string
	}{
		"perfect_health": {
			setupMetrics: func() *ProxyMetricsSummary {
				return &ProxyMetricsSummary{
					TotalRequests:      100,
					EnvoyRequests:      50,
					DirectRequests:     50,
					EnvoySuccessful:    50,
					DirectSuccessful:   50,
					EnvoyAvgLatencyMs:  100,
					DirectAvgLatencyMs: 80,
					EnvoySuccessRate:   100,
					DirectSuccessRate:  100,
					ConfigErrors:       0,
					TimeoutErrors:      0,
					ConnectionErrors:   0,
					DNSErrors:          0,
				}
			},
			expectedMin: 95.0,
			expectedMax: 100.0,
			description: "Perfect metrics should yield high health score",
		},
		"poor_success_rate": {
			setupMetrics: func() *ProxyMetricsSummary {
				return &ProxyMetricsSummary{
					TotalRequests:      100,
					EnvoyRequests:      50,
					DirectRequests:     50,
					EnvoySuccessful:    40, // 80% success rate
					DirectSuccessful:   45, // 90% success rate
					EnvoyAvgLatencyMs:  100,
					DirectAvgLatencyMs: 80,
					EnvoySuccessRate:   80,
					DirectSuccessRate:  90,
					ConfigErrors:       0,
					TimeoutErrors:      0,
					ConnectionErrors:   0,
					DNSErrors:          0,
				}
			},
			expectedMin: 60.0,
			expectedMax: 85.0,
			description: "Poor success rates should reduce health score",
		},
		"high_latency": {
			setupMetrics: func() *ProxyMetricsSummary {
				return &ProxyMetricsSummary{
					TotalRequests:      100,
					EnvoyRequests:      50,
					DirectRequests:     50,
					EnvoySuccessful:    50,
					DirectSuccessful:   50,
					EnvoyAvgLatencyMs:  8000, // Very high latency
					DirectAvgLatencyMs: 7000, // Very high latency
					EnvoySuccessRate:   100,
					DirectSuccessRate:  100,
					ConfigErrors:       0,
					TimeoutErrors:      0,
					ConnectionErrors:   0,
					DNSErrors:          0,
				}
			},
			expectedMin: 40.0,
			expectedMax: 80.0,
			description: "High latency should reduce health score",
		},
		"many_errors": {
			setupMetrics: func() *ProxyMetricsSummary {
				return &ProxyMetricsSummary{
					TotalRequests:      100,
					EnvoyRequests:      50,
					DirectRequests:     50,
					EnvoySuccessful:    50,
					DirectSuccessful:   50,
					EnvoyAvgLatencyMs:  100,
					DirectAvgLatencyMs: 80,
					EnvoySuccessRate:   100,
					DirectSuccessRate:  100,
					ConfigErrors:       10,
					TimeoutErrors:      5,
					ConnectionErrors:   3,
					DNSErrors:          2,
				}
			},
			expectedMin: 20.0,
			expectedMax: 70.0,
			description: "Many errors should significantly reduce health score",
		},
		"no_requests": {
			setupMetrics: func() *ProxyMetricsSummary {
				return &ProxyMetricsSummary{
					TotalRequests: 0,
				}
			},
			expectedMin: 100.0,
			expectedMax: 100.0,
			description: "No requests should yield perfect health score",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			summary := tc.setupMetrics()
			score := summary.GetHealthScore()

			if score < tc.expectedMin || score > tc.expectedMax {
				t.Errorf("%s: expected health score between %.1f and %.1f, got %.1f",
					tc.description, tc.expectedMin, tc.expectedMax, score)
			}

			t.Logf("%s: health score = %.1f (expected: %.1f - %.1f)",
				tc.description, score, tc.expectedMin, tc.expectedMax)
		})
	}
}

// TestProxyMetrics_MovingAverages tests moving average calculation
func TestProxyMetrics_MovingAverages(t *testing.T) {
	logger := slog.Default()
	metrics := NewProxyMetrics(logger)

	// Record several requests to test moving averages
	durations := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
		400 * time.Millisecond,
		500 * time.Millisecond,
	}

	// Record Envoy requests
	for _, duration := range durations {
		metrics.RecordEnvoyRequest(duration, true, 5*time.Millisecond)
	}

	// Record direct requests
	for _, duration := range durations {
		metrics.RecordDirectRequest(duration, true)
	}

	summary := metrics.GetMetricsSummary()

	// Verify moving averages are calculated
	if summary.EnvoyMovingAvgMs == 0 {
		t.Errorf("Expected Envoy moving average to be calculated")
	}

	if summary.DirectMovingAvgMs == 0 {
		t.Errorf("Expected direct moving average to be calculated")
	}

	// Moving averages should be reasonable (around 300ms for our test data)
	expectedAvg := 300.0 // Average of 100, 200, 300, 400, 500
	tolerance := 50.0

	if abs(summary.EnvoyMovingAvgMs-expectedAvg) > tolerance {
		t.Errorf("Envoy moving average %.2f ms not within tolerance of %.2f ms",
			summary.EnvoyMovingAvgMs, expectedAvg)
	}

	if abs(summary.DirectMovingAvgMs-expectedAvg) > tolerance {
		t.Errorf("Direct moving average %.2f ms not within tolerance of %.2f ms",
			summary.DirectMovingAvgMs, expectedAvg)
	}

	t.Logf("Moving averages - Envoy: %.2f ms, Direct: %.2f ms",
		summary.EnvoyMovingAvgMs, summary.DirectMovingAvgMs)
}

// TestProxyMetrics_GlobalSingleton tests global metrics singleton
func TestProxyMetrics_GlobalSingleton(t *testing.T) {
	logger := slog.Default()

	// Get global instance twice
	metrics1 := GetGlobalProxyMetrics(logger)
	metrics2 := GetGlobalProxyMetrics(logger)

	// Should be the same instance
	if metrics1 != metrics2 {
		t.Errorf("Expected global metrics to return the same singleton instance")
	}

	// Record request in first instance
	metrics1.RecordEnvoyRequest(100*time.Millisecond, true, 10*time.Millisecond)

	// Verify it's reflected in second instance
	summary := metrics2.GetMetricsSummary()

	if summary.EnvoyRequests == 0 {
		t.Errorf("Expected metrics to be shared across singleton instances")
	}

	t.Logf("Global singleton working correctly: %d requests recorded", summary.TotalRequests)
}

// Helper function for absolute value
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
