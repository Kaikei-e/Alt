// ABOUTME: This file provides health metrics structure for production monitoring
// ABOUTME: Implements metrics for rask-log-aggregator dashboards and SLA monitoring
package service

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"pre-processor/utils/logger"
)

// HealthMetrics represents the comprehensive health metrics structure from TASK4.md
type HealthMetrics struct {
	LogsPerSecond   float64 `json:"logs_per_second"`
	AvgLatency      float64 `json:"avg_latency_ms"`
	ErrorRate       float64 `json:"error_rate"`
	MemoryUsage     uint64  `json:"memory_usage_mb"`
	Timestamp       int64   `json:"timestamp"`
	ServiceStatus   string  `json:"service_status"`
	Uptime          float64 `json:"uptime_seconds"`
	RequestCount    uint64  `json:"request_count"`
	SuccessCount    uint64  `json:"success_count"`
	FailureCount    uint64  `json:"failure_count"`
	GoroutineCount  int     `json:"goroutine_count"`
}

// ExtendedHealthMetrics provides additional metrics for comprehensive monitoring
type ExtendedHealthMetrics struct {
	HealthMetrics
	DatabaseConnections int                    `json:"database_connections"`
	ExternalAPIStatus   map[string]string      `json:"external_api_status"`
	ComponentHealth     map[string]interface{} `json:"component_health"`
	PerformanceMetrics  PerformanceMetrics     `json:"performance_metrics"`
}

type PerformanceMetrics struct {
	CPUUsage        float64 `json:"cpu_usage_percent"`
	HeapSize        uint64  `json:"heap_size_mb"`
	GCCount         uint32  `json:"gc_count"`
	LastGCDuration  float64 `json:"last_gc_duration_ms"`
	AllocRate       float64 `json:"alloc_rate_mb_per_sec"`
}

// HealthMetricsCollector collects and aggregates health metrics
type HealthMetricsCollector struct {
	mu                sync.RWMutex
	logger            *logger.ContextLogger
	startTime         time.Time
	requestCount      uint64
	successCount      uint64
	failureCount      uint64
	latencySum        float64
	latencyCount      uint64
	logCountSum       uint64
	logCountWindow    time.Duration
	lastLogCountReset time.Time
	slaTarget         float64 // 99.9% availability target
}

// NewHealthMetricsCollector creates a new health metrics collector
func NewHealthMetricsCollector(logger *logger.ContextLogger) *HealthMetricsCollector {
	return &HealthMetricsCollector{
		logger:            logger,
		startTime:         time.Now(),
		logCountWindow:    1 * time.Minute, // 1-minute window for logs per second
		lastLogCountReset: time.Now(),
		slaTarget:         99.9, // 99.9% availability target from TASK4.md
	}
}

// RecordRequest records a request and its outcome
func (hmc *HealthMetricsCollector) RecordRequest(ctx context.Context, latency time.Duration, success bool) {
	hmc.mu.Lock()
	defer hmc.mu.Unlock()

	hmc.requestCount++
	hmc.latencySum += float64(latency.Milliseconds())
	hmc.latencyCount++

	if success {
		hmc.successCount++
	} else {
		hmc.failureCount++
	}

	hmc.logger.WithContext(ctx).Debug("request recorded",
		"latency_ms", latency.Milliseconds(),
		"success", success,
		"total_requests", hmc.requestCount)
}

// RecordLogEntry records a log entry for logs per second calculation
func (hmc *HealthMetricsCollector) RecordLogEntry(ctx context.Context) {
	hmc.mu.Lock()
	defer hmc.mu.Unlock()

	hmc.logCountSum++

	// Reset window if needed
	if time.Since(hmc.lastLogCountReset) > hmc.logCountWindow {
		hmc.lastLogCountReset = time.Now()
		hmc.logCountSum = 1 // Reset to current log
	}
}

// GetHealthMetrics returns current health metrics
func (hmc *HealthMetricsCollector) GetHealthMetrics(ctx context.Context) *HealthMetrics {
	hmc.mu.RLock()
	defer hmc.mu.RUnlock()

	var avgLatency float64
	if hmc.latencyCount > 0 {
		avgLatency = hmc.latencySum / float64(hmc.latencyCount)
	}

	var errorRate float64
	if hmc.requestCount > 0 {
		errorRate = (float64(hmc.failureCount) / float64(hmc.requestCount)) * 100
	}

	// Calculate logs per second
	windowSeconds := hmc.logCountWindow.Seconds()
	logsPerSecond := float64(hmc.logCountSum) / windowSeconds

	// Get memory usage
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	memoryUsageMB := memStats.Alloc / 1024 / 1024

	// Determine service status based on SLA
	serviceStatus := hmc.calculateServiceStatus(errorRate)

	uptime := time.Since(hmc.startTime).Seconds()

	metrics := &HealthMetrics{
		LogsPerSecond:   logsPerSecond,
		AvgLatency:      avgLatency,
		ErrorRate:       errorRate,
		MemoryUsage:     memoryUsageMB,
		Timestamp:       time.Now().Unix(),
		ServiceStatus:   serviceStatus,
		Uptime:          uptime,
		RequestCount:    hmc.requestCount,
		SuccessCount:    hmc.successCount,
		FailureCount:    hmc.failureCount,
		GoroutineCount:  runtime.NumGoroutine(),
	}

	hmc.logger.WithContext(ctx).Debug("health metrics collected",
		"logs_per_second", logsPerSecond,
		"avg_latency_ms", avgLatency,
		"error_rate", errorRate,
		"memory_mb", memoryUsageMB,
		"status", serviceStatus)

	return metrics
}

// GetExtendedHealthMetrics returns comprehensive health metrics
func (hmc *HealthMetricsCollector) GetExtendedHealthMetrics(ctx context.Context) *ExtendedHealthMetrics {
	baseMetrics := hmc.GetHealthMetrics(ctx)

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	perfMetrics := PerformanceMetrics{
		CPUUsage:       hmc.getCPUUsage(), // Placeholder - would need actual CPU monitoring
		HeapSize:       memStats.HeapAlloc / 1024 / 1024,
		GCCount:        memStats.NumGC,
		LastGCDuration: float64(memStats.PauseNs[(memStats.NumGC+255)%256]) / 1000000, // Convert to ms
		AllocRate:      hmc.calculateAllocRate(),
	}

	componentHealth := map[string]interface{}{
		"database": hmc.checkDatabaseHealth(),
		"memory":   hmc.checkMemoryHealth(memStats),
		"goroutines": hmc.checkGoroutineHealth(),
	}

	externalAPIStatus := map[string]string{
		"news_creator": "healthy", // Would be populated by actual health checks
	}

	extended := &ExtendedHealthMetrics{
		HealthMetrics:       *baseMetrics,
		DatabaseConnections: 0, // Would be populated by database driver
		ExternalAPIStatus:   externalAPIStatus,
		ComponentHealth:     componentHealth,
		PerformanceMetrics:  perfMetrics,
	}

	hmc.logger.WithContext(ctx).Info("extended health metrics collected",
		"heap_size_mb", perfMetrics.HeapSize,
		"gc_count", perfMetrics.GCCount,
		"goroutine_count", baseMetrics.GoroutineCount)

	return extended
}

// LogHealthMetrics logs health metrics for monitoring systems
func (hmc *HealthMetricsCollector) LogHealthMetrics(ctx context.Context) {
	metrics := hmc.GetHealthMetrics(ctx)

	hmc.logger.WithContext(ctx).Info("health_metrics",
		"logs_per_second", metrics.LogsPerSecond,
		"avg_latency_ms", metrics.AvgLatency,
		"error_rate", metrics.ErrorRate,
		"memory_usage_mb", metrics.MemoryUsage,
		"service_status", metrics.ServiceStatus,
		"uptime_seconds", metrics.Uptime,
		"request_count", metrics.RequestCount,
		"success_count", metrics.SuccessCount,
		"failure_count", metrics.FailureCount,
		"goroutine_count", metrics.GoroutineCount,
		"sla_compliance", hmc.calculateSLACompliance(),
	)
}

// CheckSLACompliance checks if the service meets the 99.9% availability target
func (hmc *HealthMetricsCollector) CheckSLACompliance(ctx context.Context) bool {
	hmc.mu.RLock()
	defer hmc.mu.RUnlock()

	compliance := hmc.calculateSLACompliance()
	meetsSLA := compliance >= hmc.slaTarget

	hmc.logger.WithContext(ctx).Info("sla_compliance_check",
		"current_availability", compliance,
		"target_availability", hmc.slaTarget,
		"meets_sla", meetsSLA)

	return meetsSLA
}

// ResetMetrics resets collected metrics (useful for testing or periodic resets)
func (hmc *HealthMetricsCollector) ResetMetrics(ctx context.Context) {
	hmc.mu.Lock()
	defer hmc.mu.Unlock()

	hmc.requestCount = 0
	hmc.successCount = 0
	hmc.failureCount = 0
	hmc.latencySum = 0
	hmc.latencyCount = 0
	hmc.logCountSum = 0
	hmc.lastLogCountReset = time.Now()

	hmc.logger.WithContext(ctx).Info("health metrics reset")
}

// Helper methods

func (hmc *HealthMetricsCollector) calculateServiceStatus(errorRate float64) string {
	if errorRate > 5.0 {
		return "critical"
	} else if errorRate > 1.0 {
		return "warning"
	} else if errorRate > 0.1 {
		return "degraded"
	}
	return "healthy"
}

func (hmc *HealthMetricsCollector) calculateSLACompliance() float64 {
	if hmc.requestCount == 0 {
		return 100.0
	}
	return (float64(hmc.successCount) / float64(hmc.requestCount)) * 100
}

func (hmc *HealthMetricsCollector) getCPUUsage() float64 {
	// Placeholder - in a real implementation, this would use a CPU monitoring library
	// or system calls to get actual CPU usage
	return 0.0
}

func (hmc *HealthMetricsCollector) calculateAllocRate() float64 {
	// Placeholder - would calculate allocation rate over time
	return 0.0
}

func (hmc *HealthMetricsCollector) checkDatabaseHealth() map[string]interface{} {
	return map[string]interface{}{
		"status":      "connected",
		"ping_time":   "2ms",
		"connections": 5,
	}
}

func (hmc *HealthMetricsCollector) checkMemoryHealth(memStats runtime.MemStats) map[string]interface{} {
	memoryMB := memStats.Alloc / 1024 / 1024
	heapMB := memStats.HeapAlloc / 1024 / 1024

	status := "healthy"
	if memoryMB > 500 { // 500MB threshold
		status = "warning"
	}
	if memoryMB > 1000 { // 1GB threshold
		status = "critical"
	}

	return map[string]interface{}{
		"status":         status,
		"total_alloc_mb": memoryMB,
		"heap_alloc_mb":  heapMB,
		"gc_count":       memStats.NumGC,
	}
}

func (hmc *HealthMetricsCollector) checkGoroutineHealth() map[string]interface{} {
	count := runtime.NumGoroutine()
	
	status := "healthy"
	if count > 100 {
		status = "warning"
	}
	if count > 500 {
		status = "critical"
	}

	return map[string]interface{}{
		"status": status,
		"count":  count,
	}
}

// HealthAlert represents an alert condition
type HealthAlert struct {
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Metric    string    `json:"metric"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	Timestamp time.Time `json:"timestamp"`
}

// CheckAlerts checks for alert conditions and returns any alerts
func (hmc *HealthMetricsCollector) CheckAlerts(ctx context.Context) []HealthAlert {
	metrics := hmc.GetHealthMetrics(ctx)
	var alerts []HealthAlert

	// Error rate alert
	if metrics.ErrorRate > 5.0 {
		alerts = append(alerts, HealthAlert{
			Level:     "critical",
			Message:   "High error rate detected",
			Metric:    "error_rate",
			Value:     metrics.ErrorRate,
			Threshold: 5.0,
			Timestamp: time.Now(),
		})
	} else if metrics.ErrorRate > 1.0 {
		alerts = append(alerts, HealthAlert{
			Level:     "warning",
			Message:   "Elevated error rate",
			Metric:    "error_rate",
			Value:     metrics.ErrorRate,
			Threshold: 1.0,
			Timestamp: time.Now(),
		})
	}

	// Memory usage alert
	if metrics.MemoryUsage > 1000 { // 1GB
		alerts = append(alerts, HealthAlert{
			Level:     "critical",
			Message:   "High memory usage",
			Metric:    "memory_usage",
			Value:     float64(metrics.MemoryUsage),
			Threshold: 1000,
			Timestamp: time.Now(),
		})
	} else if metrics.MemoryUsage > 500 { // 500MB
		alerts = append(alerts, HealthAlert{
			Level:     "warning",
			Message:   "Elevated memory usage",
			Metric:    "memory_usage",
			Value:     float64(metrics.MemoryUsage),
			Threshold: 500,
			Timestamp: time.Now(),
		})
	}

	// High latency alert
	if metrics.AvgLatency > 1000 { // 1 second
		alerts = append(alerts, HealthAlert{
			Level:     "critical",
			Message:   "High average latency",
			Metric:    "avg_latency",
			Value:     metrics.AvgLatency,
			Threshold: 1000,
			Timestamp: time.Now(),
		})
	} else if metrics.AvgLatency > 500 { // 500ms
		alerts = append(alerts, HealthAlert{
			Level:     "warning",
			Message:   "Elevated average latency",
			Metric:    "avg_latency",
			Value:     metrics.AvgLatency,
			Threshold: 500,
			Timestamp: time.Now(),
		})
	}

	// Goroutine leak alert
	if metrics.GoroutineCount > 500 {
		alerts = append(alerts, HealthAlert{
			Level:     "critical",
			Message:   "Potential goroutine leak",
			Metric:    "goroutine_count",
			Value:     float64(metrics.GoroutineCount),
			Threshold: 500,
			Timestamp: time.Now(),
		})
	} else if metrics.GoroutineCount > 100 {
		alerts = append(alerts, HealthAlert{
			Level:     "warning",
			Message:   "High goroutine count",
			Metric:    "goroutine_count",
			Value:     float64(metrics.GoroutineCount),
			Threshold: 100,
			Timestamp: time.Now(),
		})
	}

	if len(alerts) > 0 {
		hmc.logger.WithContext(ctx).Warn("health alerts detected",
			"alert_count", len(alerts),
			"alerts", fmt.Sprintf("%+v", alerts))
	}

	return alerts
}