// ABOUTME: This file contains tests for enhanced health handler with monitoring endpoints
// ABOUTME: Tests health metrics, SLA compliance, and alerting functionality
package handler

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"pre-processor/service"
	"pre-processor/utils/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthHandler_GetHealthMetrics(t *testing.T) {
	tests := map[string]struct {
		name                 string
		setupMetrics         func(*service.HealthMetricsCollector, context.Context)
		expectError          bool
		expectedMetricsCount int
		expectedStatus       string
	}{
		"successful_metrics_retrieval": {
			name: "healthy metrics",
			setupMetrics: func(collector *service.HealthMetricsCollector, ctx context.Context) {
				collector.RecordRequest(ctx, 100*time.Millisecond, true)
				collector.RecordRequest(ctx, 150*time.Millisecond, true)
			},
			expectError:          false,
			expectedMetricsCount: 11, // All health metrics fields
			expectedStatus:       "healthy",
		},
		"metrics_with_errors": {
			name: "metrics with error rate",
			setupMetrics: func(collector *service.HealthMetricsCollector, ctx context.Context) {
				collector.RecordRequest(ctx, 100*time.Millisecond, true)
				collector.RecordRequest(ctx, 200*time.Millisecond, false)
			},
			expectError:          false,
			expectedMetricsCount: 11,
			expectedStatus:       "healthy", // 50% error rate should still be "healthy" for this test
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup
			contextLogger := logger.NewContextLogger("json", "debug")
			metricsCollector := service.NewHealthMetricsCollector(contextLogger)

			mockHealthChecker := &mockHealthChecker{}
			handler := NewHealthHandler(mockHealthChecker, metricsCollector, slog.Default())

			ctx := context.Background()

			// Setup metrics according to test case
			tc.setupMetrics(metricsCollector, ctx)

			// Test GetHealthMetrics
			result, err := handler.GetHealthMetrics(ctx)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, result, tc.expectedMetricsCount)

			// Verify key metrics are present
			assert.Contains(t, result, "logs_per_second")
			assert.Contains(t, result, "avg_latency_ms")
			assert.Contains(t, result, "error_rate")
			assert.Contains(t, result, "memory_usage_mb")
			assert.Contains(t, result, "service_status")
			assert.Contains(t, result, "uptime_seconds")
			assert.Contains(t, result, "timestamp")

			// Verify values are reasonable
			assert.GreaterOrEqual(t, result["uptime_seconds"], 0.0)
			assert.GreaterOrEqual(t, result["memory_usage_mb"], uint64(0))
			assert.NotEmpty(t, result["service_status"])
		})
	}
}

func TestHealthHandler_GetExtendedHealthMetrics(t *testing.T) {
	contextLogger := logger.NewContextLogger("json", "debug")
	metricsCollector := service.NewHealthMetricsCollector(contextLogger)

	mockHealthChecker := &mockHealthChecker{}
	handler := NewHealthHandler(mockHealthChecker, metricsCollector, slog.Default())

	ctx := context.Background()

	// Record some sample data
	metricsCollector.RecordRequest(ctx, 120*time.Millisecond, true)

	result, err := handler.GetExtendedHealthMetrics(ctx)

	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify extended structure
	assert.Contains(t, result, "basic_metrics")
	assert.Contains(t, result, "performance_metrics")
	assert.Contains(t, result, "component_health")
	assert.Contains(t, result, "external_api_status")
	assert.Contains(t, result, "database_connections")
	assert.Contains(t, result, "timestamp")

	// Verify basic metrics are nested correctly
	basicMetrics, ok := result["basic_metrics"].(map[string]interface{})
	require.True(t, ok, "basic_metrics should be a map")
	assert.Contains(t, basicMetrics, "logs_per_second")
	assert.Contains(t, basicMetrics, "service_status")

	// Verify performance metrics are nested correctly
	perfMetrics, ok := result["performance_metrics"].(map[string]interface{})
	require.True(t, ok, "performance_metrics should be a map")
	assert.Contains(t, perfMetrics, "heap_size_mb")
	assert.Contains(t, perfMetrics, "gc_count")
}

func TestHealthHandler_CheckSLACompliance(t *testing.T) {
	tests := map[string]struct {
		name             string
		setupRequests    func(*service.HealthMetricsCollector, context.Context)
		expectedMeetsSLA bool
		expectedAvail    float64
	}{
		"meets_sla_target": {
			name: "perfect availability",
			setupRequests: func(collector *service.HealthMetricsCollector, ctx context.Context) {
				for i := 0; i < 1000; i++ {
					collector.RecordRequest(ctx, 100*time.Millisecond, true)
				}
			},
			expectedMeetsSLA: true,
			expectedAvail:    100.0,
		},
		"meets_sla_at_boundary": {
			name: "99.9% availability",
			setupRequests: func(collector *service.HealthMetricsCollector, ctx context.Context) {
				for i := 0; i < 999; i++ {
					collector.RecordRequest(ctx, 100*time.Millisecond, true)
				}
				collector.RecordRequest(ctx, 100*time.Millisecond, false)
			},
			expectedMeetsSLA: true,
			expectedAvail:    99.9,
		},
		"below_sla_target": {
			name: "below 99.9% availability",
			setupRequests: func(collector *service.HealthMetricsCollector, ctx context.Context) {
				for i := 0; i < 990; i++ {
					collector.RecordRequest(ctx, 100*time.Millisecond, true)
				}
				for i := 0; i < 10; i++ {
					collector.RecordRequest(ctx, 100*time.Millisecond, false)
				}
			},
			expectedMeetsSLA: false,
			expectedAvail:    99.0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			contextLogger := logger.NewContextLogger("json", "debug")
			metricsCollector := service.NewHealthMetricsCollector(contextLogger)

			mockHealthChecker := &mockHealthChecker{}
			handler := NewHealthHandler(mockHealthChecker, metricsCollector, slog.Default())

			ctx := context.Background()

			// Setup requests according to test case
			tc.setupRequests(metricsCollector, ctx)

			result, err := handler.CheckSLACompliance(ctx)

			require.NoError(t, err)
			assert.NotNil(t, result)

			// Verify SLA compliance structure
			assert.Contains(t, result, "meets_sla")
			assert.Contains(t, result, "target_availability")
			assert.Contains(t, result, "current_availability")
			assert.Contains(t, result, "status")

			// Verify expected values
			assert.Equal(t, tc.expectedMeetsSLA, result["meets_sla"])
			assert.Equal(t, 99.9, result["target_availability"])
			assert.Equal(t, tc.expectedAvail, result["current_availability"])

			expectedStatus := "compliant"
			if !tc.expectedMeetsSLA {
				expectedStatus = "non_compliant"
			}
			assert.Equal(t, expectedStatus, result["status"])
		})
	}
}

func TestHealthHandler_GetHealthAlerts(t *testing.T) {
	tests := map[string]struct {
		name           string
		setupMetrics   func(*service.HealthMetricsCollector, context.Context)
		expectedAlerts int
		expectedLevels []string
	}{
		"no_alerts": {
			name: "healthy system",
			setupMetrics: func(collector *service.HealthMetricsCollector, ctx context.Context) {
				for i := 0; i < 100; i++ {
					collector.RecordRequest(ctx, 50*time.Millisecond, true)
				}
			},
			expectedAlerts: 0,
			expectedLevels: []string{},
		},
		"error_rate_alert": {
			name: "high error rate",
			setupMetrics: func(collector *service.HealthMetricsCollector, ctx context.Context) {
				// Generate 10% error rate (critical threshold)
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
		"latency_alert": {
			name: "high latency",
			setupMetrics: func(collector *service.HealthMetricsCollector, ctx context.Context) {
				// Generate high latency request (warning threshold)
				collector.RecordRequest(ctx, 600*time.Millisecond, true)
			},
			expectedAlerts: 1,
			expectedLevels: []string{"warning"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			contextLogger := logger.NewContextLogger("json", "debug")
			metricsCollector := service.NewHealthMetricsCollector(contextLogger)

			mockHealthChecker := &mockHealthChecker{}
			handler := NewHealthHandler(mockHealthChecker, metricsCollector, slog.Default())

			ctx := context.Background()

			// Setup metrics according to test case
			tc.setupMetrics(metricsCollector, ctx)

			result, err := handler.GetHealthAlerts(ctx)

			require.NoError(t, err)
			assert.Len(t, result, tc.expectedAlerts)

			// Verify alert structure and levels
			for i, alert := range result {
				assert.Contains(t, alert, "level")
				assert.Contains(t, alert, "message")
				assert.Contains(t, alert, "metric")
				assert.Contains(t, alert, "value")
				assert.Contains(t, alert, "threshold")
				assert.Contains(t, alert, "timestamp")

				if i < len(tc.expectedLevels) {
					assert.Equal(t, tc.expectedLevels[i], alert["level"])
				}
			}
		})
	}
}

func TestHealthHandler_MetricsCollectorNotConfigured(t *testing.T) {
	// Test handler behavior when metrics collector is not configured
	mockHealthChecker := &mockHealthChecker{}
	handler := NewHealthHandler(mockHealthChecker, nil, slog.Default())

	ctx := context.Background()

	// Test all metrics endpoints should return error
	_, err := handler.GetHealthMetrics(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "metrics collector not configured")

	_, err = handler.GetExtendedHealthMetrics(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "metrics collector not configured")

	_, err = handler.CheckSLACompliance(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "metrics collector not configured")

	_, err = handler.GetHealthAlerts(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "metrics collector not configured")
}

func TestHealthHandler_ExistingMethods(t *testing.T) {
	// Test that existing health check methods still work
	contextLogger := logger.NewContextLogger("json", "debug")
	metricsCollector := service.NewHealthMetricsCollector(contextLogger)

	mockHealthChecker := &mockHealthChecker{}
	handler := NewHealthHandler(mockHealthChecker, metricsCollector, slog.Default())

	ctx := context.Background()

	// Test CheckHealth
	err := handler.CheckHealth(ctx)
	assert.NoError(t, err)

	// Test CheckDependencies
	err = handler.CheckDependencies(ctx)
	assert.NoError(t, err)
}

// Mock implementations for testing

type mockHealthChecker struct{}

func (m *mockHealthChecker) CheckNewsCreatorHealth(ctx context.Context) error {
	return nil
}

func (m *mockHealthChecker) WaitForHealthy(ctx context.Context) error {
	return nil
}
