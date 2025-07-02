// ABOUTME: This file contains tests for health metrics collection and monitoring
// ABOUTME: Tests SLA compliance, alert conditions, and metrics accuracy
package service

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"pre-processor/utils/logger"

	"github.com/stretchr/testify/assert"
)

func TestHealthMetricsCollector_RecordRequest(t *testing.T) {
	tests := map[string]struct {
		requests []struct {
			latency time.Duration
			success bool
		}
		expectedErrorRate  float64
		expectedAvgLatency float64
	}{
		"all_successful_requests": {
			requests: []struct {
				latency time.Duration
				success bool
			}{
				{100 * time.Millisecond, true},
				{200 * time.Millisecond, true},
				{150 * time.Millisecond, true},
			},
			expectedErrorRate:  0.0,
			expectedAvgLatency: 150.0, // (100+200+150)/3
		},
		"mixed_success_failure": {
			requests: []struct {
				latency time.Duration
				success bool
			}{
				{100 * time.Millisecond, true},
				{500 * time.Millisecond, false},
				{200 * time.Millisecond, true},
				{300 * time.Millisecond, false},
			},
			expectedErrorRate:  50.0,  // 2 failures out of 4
			expectedAvgLatency: 275.0, // (100+500+200+300)/4
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			contextLogger := logger.NewContextLogger("json", "debug")
			collector := NewHealthMetricsCollector(contextLogger)

			ctx := context.Background()

			// Record all requests
			for _, req := range tc.requests {
				collector.RecordRequest(ctx, req.latency, req.success)
			}

			// Get metrics and validate
			metrics := collector.GetHealthMetrics(ctx)

			assert.Equal(t, tc.expectedErrorRate, metrics.ErrorRate,
				"error rate should match expected")
			assert.Equal(t, tc.expectedAvgLatency, metrics.AvgLatency,
				"average latency should match expected")
			assert.Equal(t, uint64(len(tc.requests)), metrics.RequestCount,
				"request count should match")
		})
	}
}

func TestHealthMetricsCollector_SLACompliance(t *testing.T) {
	tests := map[string]struct {
		successCount  uint64
		failureCount  uint64
		expectedSLA   float64
		expectedMeets bool
	}{
		"perfect_sla": {
			successCount:  1000,
			failureCount:  0,
			expectedSLA:   100.0,
			expectedMeets: true,
		},
		"meets_sla_target": {
			successCount:  999,
			failureCount:  1,
			expectedSLA:   99.9,
			expectedMeets: true,
		},
		"below_sla_target": {
			successCount:  990,
			failureCount:  10,
			expectedSLA:   99.0,
			expectedMeets: false,
		},
		"no_requests": {
			successCount:  0,
			failureCount:  0,
			expectedSLA:   100.0,
			expectedMeets: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			contextLogger := logger.NewContextLogger("json", "debug")
			collector := NewHealthMetricsCollector(contextLogger)

			ctx := context.Background()

			// Simulate requests
			for i := uint64(0); i < tc.successCount; i++ {
				collector.RecordRequest(ctx, 100*time.Millisecond, true)
			}
			for i := uint64(0); i < tc.failureCount; i++ {
				collector.RecordRequest(ctx, 100*time.Millisecond, false)
			}

			// Check SLA compliance
			meetsSLA := collector.CheckSLACompliance(ctx)
			assert.Equal(t, tc.expectedMeets, meetsSLA,
				"SLA compliance should match expected")

			// Verify calculated SLA value
			calculatedSLA := collector.calculateSLACompliance()
			assert.Equal(t, tc.expectedSLA, calculatedSLA,
				"calculated SLA should match expected")
		})
	}
}

func TestHealthMetricsCollector_ServiceStatus(t *testing.T) {
	tests := map[string]struct {
		errorRate      float64
		expectedStatus string
	}{
		"healthy_status": {
			errorRate:      0.05, // 0.05%
			expectedStatus: "healthy",
		},
		"degraded_status": {
			errorRate:      0.5, // 0.5%
			expectedStatus: "degraded",
		},
		"warning_status": {
			errorRate:      2.0, // 2%
			expectedStatus: "warning",
		},
		"critical_status": {
			errorRate:      10.0, // 10%
			expectedStatus: "critical",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			contextLogger := logger.NewContextLogger("json", "debug")
			collector := NewHealthMetricsCollector(contextLogger)

			status := collector.calculateServiceStatus(tc.errorRate)
			assert.Equal(t, tc.expectedStatus, status,
				"service status should match expected for error rate %.2f%%", tc.errorRate)
		})
	}
}

func TestHealthMetricsCollector_LogsPerSecond(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive test")
	}

	contextLogger := logger.NewContextLogger("json", "debug")
	collector := NewHealthMetricsCollector(contextLogger)

	ctx := context.Background()

	// Record log entries over a short period
	numLogs := 10
	for i := 0; i < numLogs; i++ {
		collector.RecordLogEntry(ctx)
		time.Sleep(10 * time.Millisecond) // Spread out over time
	}

	metrics := collector.GetHealthMetrics(ctx)

	// Should have some logs per second (exact value depends on timing)
	assert.Greater(t, metrics.LogsPerSecond, 0.0,
		"should calculate logs per second")

	t.Logf("Recorded %d logs, calculated %.2f logs per second",
		numLogs, metrics.LogsPerSecond)
}

func TestHealthMetricsCollector_Alerts(t *testing.T) {
	tests := map[string]struct {
		name           string
		setupMetrics   func(*HealthMetricsCollector, context.Context)
		expectedAlerts int
		expectedLevels []string
	}{
		"no_alerts_healthy": {
			name: "healthy system",
			setupMetrics: func(collector *HealthMetricsCollector, ctx context.Context) {
				// Record successful requests with low latency
				for i := 0; i < 100; i++ {
					collector.RecordRequest(ctx, 50*time.Millisecond, true)
				}
			},
			expectedAlerts: 0,
			expectedLevels: []string{},
		},
		"error_rate_warning": {
			name: "high error rate",
			setupMetrics: func(collector *HealthMetricsCollector, ctx context.Context) {
				// Record requests with 2% error rate (warning threshold)
				for i := 0; i < 98; i++ {
					collector.RecordRequest(ctx, 100*time.Millisecond, true)
				}
				for i := 0; i < 2; i++ {
					collector.RecordRequest(ctx, 100*time.Millisecond, false)
				}
			},
			expectedAlerts: 1,
			expectedLevels: []string{"warning"},
		},
		"error_rate_critical": {
			name: "critical error rate",
			setupMetrics: func(collector *HealthMetricsCollector, ctx context.Context) {
				// Record requests with 10% error rate (critical threshold)
				for i := 0; i < 90; i++ {
					collector.RecordRequest(ctx, 100*time.Millisecond, true)
				}
				for i := 0; i < 10; i++ {
					collector.RecordRequest(ctx, 100*time.Millisecond, false)
				}
			},
			expectedAlerts: 1,
			expectedLevels: []string{"critical"},
		},
		"high_latency_warning": {
			name: "high latency",
			setupMetrics: func(collector *HealthMetricsCollector, ctx context.Context) {
				// Record requests with high latency (600ms average)
				collector.RecordRequest(ctx, 600*time.Millisecond, true)
			},
			expectedAlerts: 1,
			expectedLevels: []string{"warning"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			contextLogger := logger.NewContextLogger("json", "debug")
			collector := NewHealthMetricsCollector(contextLogger)

			ctx := context.Background()

			// Setup metrics according to test case
			tc.setupMetrics(collector, ctx)

			// Check for alerts
			alerts := collector.CheckAlerts(ctx)

			assert.Equal(t, tc.expectedAlerts, len(alerts),
				"should have expected number of alerts")

			// Verify alert levels
			for i, expectedLevel := range tc.expectedLevels {
				if i < len(alerts) {
					assert.Equal(t, expectedLevel, alerts[i].Level,
						"alert %d should have expected level", i)
				}
			}

			if len(alerts) > 0 {
				t.Logf("Generated alerts for %s: %+v", tc.name, alerts)
			}
		})
	}
}

func TestHealthMetricsCollector_ExtendedMetrics(t *testing.T) {
	contextLogger := logger.NewContextLogger("json", "debug")
	collector := NewHealthMetricsCollector(contextLogger)

	ctx := context.Background()

	// Record some sample data
	collector.RecordRequest(ctx, 100*time.Millisecond, true)
	collector.RecordRequest(ctx, 200*time.Millisecond, true)

	extendedMetrics := collector.GetExtendedHealthMetrics(ctx)

	// Verify basic metrics are included
	assert.Equal(t, uint64(2), extendedMetrics.RequestCount,
		"request count should be included")
	assert.Equal(t, 150.0, extendedMetrics.AvgLatency,
		"average latency should be calculated correctly")

	// Verify extended fields are present
	assert.NotNil(t, extendedMetrics.ComponentHealth,
		"component health should be present")
	assert.NotNil(t, extendedMetrics.ExternalAPIStatus,
		"external API status should be present")
	assert.NotNil(t, extendedMetrics.PerformanceMetrics,
		"performance metrics should be present")

	// Verify component health structure
	assert.Contains(t, extendedMetrics.ComponentHealth, "database",
		"should include database health")
	assert.Contains(t, extendedMetrics.ComponentHealth, "memory",
		"should include memory health")
	assert.Contains(t, extendedMetrics.ComponentHealth, "goroutines",
		"should include goroutine health")
}

func TestHealthMetricsCollector_ResetMetrics(t *testing.T) {
	contextLogger := logger.NewContextLogger("json", "debug")
	collector := NewHealthMetricsCollector(contextLogger)

	ctx := context.Background()

	// Record some data
	collector.RecordRequest(ctx, 100*time.Millisecond, true)
	collector.RecordRequest(ctx, 200*time.Millisecond, false)
	collector.RecordLogEntry(ctx)

	// Verify data is present
	metrics := collector.GetHealthMetrics(ctx)
	assert.Equal(t, uint64(2), metrics.RequestCount, "should have request data")
	assert.Equal(t, uint64(1), metrics.SuccessCount, "should have success data")
	assert.Equal(t, uint64(1), metrics.FailureCount, "should have failure data")

	// Reset metrics
	collector.ResetMetrics(ctx)

	// Verify data is cleared
	resetMetrics := collector.GetHealthMetrics(ctx)
	assert.Equal(t, uint64(0), resetMetrics.RequestCount, "request count should be reset")
	assert.Equal(t, uint64(0), resetMetrics.SuccessCount, "success count should be reset")
	assert.Equal(t, uint64(0), resetMetrics.FailureCount, "failure count should be reset")
	assert.Equal(t, 0.0, resetMetrics.ErrorRate, "error rate should be reset")
	assert.Equal(t, 0.0, resetMetrics.AvgLatency, "average latency should be reset")
}

func TestHealthMetricsCollector_ConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent test")
	}

	contextLogger := logger.NewContextLogger("json", "debug")
	collector := NewHealthMetricsCollector(contextLogger)

	ctx := context.Background()
	numWorkers := 10
	requestsPerWorker := 100

	done := make(chan bool, numWorkers)

	// Start concurrent workers
	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			defer func() { done <- true }()

			for j := 0; j < requestsPerWorker; j++ {
				latency := time.Duration(j%100) * time.Millisecond
				success := j%10 != 0 // 90% success rate

				collector.RecordRequest(ctx, latency, success)
				collector.RecordLogEntry(ctx)

				// Occasionally get metrics (test concurrent read/write)
				if j%20 == 0 {
					collector.GetHealthMetrics(ctx)
				}
			}
		}(i)
	}

	// Wait for all workers to complete
	for i := 0; i < numWorkers; i++ {
		<-done
	}

	// Verify final metrics
	metrics := collector.GetHealthMetrics(ctx)
	expectedRequests := uint64(numWorkers * requestsPerWorker)

	assert.Equal(t, expectedRequests, metrics.RequestCount,
		"should record all concurrent requests")
	assert.Greater(t, metrics.LogsPerSecond, 0.0,
		"should calculate logs per second from concurrent operations")

	// Verify data consistency
	assert.Equal(t, metrics.RequestCount, metrics.SuccessCount+metrics.FailureCount,
		"success + failure should equal total requests")
}

func TestHealthMetricsCollector_LogHealthMetrics(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	contextLogger := logger.NewContextLogger("json", "debug")
	collector := NewHealthMetricsCollector(contextLogger)

	ctx := context.Background()

	// Record some sample data
	collector.RecordRequest(ctx, 150*time.Millisecond, true)
	collector.RecordRequest(ctx, 250*time.Millisecond, false)

	// Log health metrics
	collector.LogHealthMetrics(ctx)

	// Restore stdout and read captured output
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	logOutput := buf.String()

	// Verify log was generated
	assert.Contains(t, logOutput, "health_metrics",
		"should log health metrics")
	assert.Contains(t, logOutput, "error_rate",
		"should include error rate in log")
	assert.Contains(t, logOutput, "avg_latency_ms",
		"should include average latency in log")
	assert.Contains(t, logOutput, "sla_compliance",
		"should include SLA compliance in log")
}
