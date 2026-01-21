package handler

import (
	"context"
	"fmt"
	"log/slog"

	"pre-processor/service"
)

// HealthHandler implementation.
type healthHandler struct {
	healthChecker    service.HealthCheckerService
	metricsCollector *service.HealthMetricsCollector
	logger           *slog.Logger
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(healthChecker service.HealthCheckerService, metricsCollector *service.HealthMetricsCollector, logger *slog.Logger) HealthHandler {
	return &healthHandler{
		healthChecker:    healthChecker,
		metricsCollector: metricsCollector,
		logger:           logger,
	}
}

// CheckHealth checks the health of the service.
func (h *healthHandler) CheckHealth(ctx context.Context) error {
	h.logger.InfoContext(ctx, "performing health check")

	// Check if we can perform basic operations
	// This is a simple implementation - in a real system you might check database connectivity, etc.
	h.logger.InfoContext(ctx, "health check completed - service is healthy")

	return nil
}

// CheckDependencies checks the health of external dependencies.
func (h *healthHandler) CheckDependencies(ctx context.Context) error {
	h.logger.InfoContext(ctx, "checking dependencies health")

	// Check news creator health
	if err := h.healthChecker.CheckNewsCreatorHealth(ctx); err != nil {
		h.logger.ErrorContext(ctx, "news creator health check failed", "error", err)
		return fmt.Errorf("news creator health check failed: %w", err)
	}

	h.logger.InfoContext(ctx, "all dependencies are healthy")

	return nil
}

// GetHealthMetrics returns current health metrics for monitoring
func (h *healthHandler) GetHealthMetrics(ctx context.Context) (map[string]interface{}, error) {
	h.logger.InfoContext(ctx, "retrieving health metrics")

	if h.metricsCollector == nil {
		h.logger.ErrorContext(ctx, "metrics collector not available")
		return nil, fmt.Errorf("metrics collector not configured")
	}

	metrics := h.metricsCollector.GetHealthMetrics(ctx)

	result := map[string]interface{}{
		"logs_per_second": metrics.LogsPerSecond,
		"avg_latency_ms":  metrics.AvgLatency,
		"error_rate":      metrics.ErrorRate,
		"memory_usage_mb": metrics.MemoryUsage,
		"timestamp":       metrics.Timestamp,
		"service_status":  metrics.ServiceStatus,
		"uptime_seconds":  metrics.Uptime,
		"request_count":   metrics.RequestCount,
		"success_count":   metrics.SuccessCount,
		"failure_count":   metrics.FailureCount,
		"goroutine_count": metrics.GoroutineCount,
	}

	h.logger.InfoContext(ctx, "health metrics retrieved successfully",
		"metrics_count", len(result),
		"service_status", metrics.ServiceStatus)

	return result, nil
}

// GetExtendedHealthMetrics returns comprehensive health metrics for detailed monitoring
func (h *healthHandler) GetExtendedHealthMetrics(ctx context.Context) (map[string]interface{}, error) {
	h.logger.InfoContext(ctx, "retrieving extended health metrics")

	if h.metricsCollector == nil {
		h.logger.ErrorContext(ctx, "metrics collector not available")
		return nil, fmt.Errorf("metrics collector not configured")
	}

	extendedMetrics := h.metricsCollector.GetExtendedHealthMetrics(ctx)

	result := map[string]interface{}{
		"basic_metrics": map[string]interface{}{
			"logs_per_second": extendedMetrics.LogsPerSecond,
			"avg_latency_ms":  extendedMetrics.AvgLatency,
			"error_rate":      extendedMetrics.ErrorRate,
			"memory_usage_mb": extendedMetrics.MemoryUsage,
			"service_status":  extendedMetrics.ServiceStatus,
			"uptime_seconds":  extendedMetrics.Uptime,
			"request_count":   extendedMetrics.RequestCount,
			"success_count":   extendedMetrics.SuccessCount,
			"failure_count":   extendedMetrics.FailureCount,
			"goroutine_count": extendedMetrics.GoroutineCount,
		},
		"performance_metrics": map[string]interface{}{
			"cpu_usage_percent":     extendedMetrics.PerformanceMetrics.CPUUsage,
			"heap_size_mb":          extendedMetrics.PerformanceMetrics.HeapSize,
			"gc_count":              extendedMetrics.PerformanceMetrics.GCCount,
			"last_gc_duration_ms":   extendedMetrics.PerformanceMetrics.LastGCDuration,
			"alloc_rate_mb_per_sec": extendedMetrics.PerformanceMetrics.AllocRate,
		},
		"component_health":     extendedMetrics.ComponentHealth,
		"external_api_status":  extendedMetrics.ExternalAPIStatus,
		"database_connections": extendedMetrics.DatabaseConnections,
		"timestamp":            extendedMetrics.Timestamp,
	}

	h.logger.InfoContext(ctx, "extended health metrics retrieved successfully",
		"component_count", len(extendedMetrics.ComponentHealth),
		"external_apis", len(extendedMetrics.ExternalAPIStatus))

	return result, nil
}

// CheckSLACompliance checks and returns SLA compliance status
func (h *healthHandler) CheckSLACompliance(ctx context.Context) (map[string]interface{}, error) {
	h.logger.InfoContext(ctx, "checking SLA compliance")

	if h.metricsCollector == nil {
		h.logger.ErrorContext(ctx, "metrics collector not available")
		return nil, fmt.Errorf("metrics collector not configured")
	}

	meetsSLA := h.metricsCollector.CheckSLACompliance(ctx)
	metrics := h.metricsCollector.GetHealthMetrics(ctx)

	// Calculate availability percentage
	availability := 100.0
	if metrics.RequestCount > 0 {
		availability = (float64(metrics.SuccessCount) / float64(metrics.RequestCount)) * 100
	}

	result := map[string]interface{}{
		"meets_sla":            meetsSLA,
		"target_availability":  99.9, // From TASK4.md
		"current_availability": availability,
		"total_requests":       metrics.RequestCount,
		"successful_requests":  metrics.SuccessCount,
		"failed_requests":      metrics.FailureCount,
		"error_rate":           metrics.ErrorRate,
		"uptime_seconds":       metrics.Uptime,
		"timestamp":            metrics.Timestamp,
		"status": func() string {
			if meetsSLA {
				return "compliant"
			}
			return "non_compliant"
		}(),
	}

	h.logger.InfoContext(ctx, "SLA compliance check completed",
		"meets_sla", meetsSLA,
		"availability", availability,
		"target", 99.9)

	return result, nil
}

// GetHealthAlerts returns current health alerts
func (h *healthHandler) GetHealthAlerts(ctx context.Context) ([]map[string]interface{}, error) {
	h.logger.InfoContext(ctx, "retrieving health alerts")

	if h.metricsCollector == nil {
		h.logger.ErrorContext(ctx, "metrics collector not available")
		return nil, fmt.Errorf("metrics collector not configured")
	}

	alerts := h.metricsCollector.CheckAlerts(ctx)

	var result []map[string]interface{}
	for _, alert := range alerts {
		alertMap := map[string]interface{}{
			"level":     alert.Level,
			"message":   alert.Message,
			"metric":    alert.Metric,
			"value":     alert.Value,
			"threshold": alert.Threshold,
			"timestamp": alert.Timestamp.Unix(),
		}
		result = append(result, alertMap)
	}

	h.logger.InfoContext(ctx, "health alerts retrieved",
		"alert_count", len(result),
		"critical_alerts", countAlertsByLevel(alerts, "critical"),
		"warning_alerts", countAlertsByLevel(alerts, "warning"))

	return result, nil
}

// Helper function to count alerts by level
func countAlertsByLevel(alerts []service.HealthAlert, level string) int {
	count := 0
	for _, alert := range alerts {
		if alert.Level == level {
			count++
		}
	}
	return count
}
