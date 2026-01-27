// TDD Phase 3 - REFACTOR: Structured Logging & Monitoring Implementation
package utils

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// MetricType represents the type of metric being recorded
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeGauge     MetricType = "gauge"
)

// Metric represents a single monitoring metric
type Metric struct {
	Name      string                 `json:"name"`
	Type      MetricType             `json:"type"`
	Value     float64                `json:"value"`
	Labels    map[string]string      `json:"labels"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// MonitoringConfig holds configuration for monitoring
type MonitoringConfig struct {
	EnableMetrics     bool          `json:"enable_metrics"`
	EnableTracing     bool          `json:"enable_tracing"`
	MetricsBatchSize  int           `json:"metrics_batch_size"`
	FlushInterval     time.Duration `json:"flush_interval"`
	RetentionDuration time.Duration `json:"retention_duration"`
}

// DefaultMonitoringConfig returns default monitoring configuration
func DefaultMonitoringConfig() *MonitoringConfig {
	return &MonitoringConfig{
		EnableMetrics:     true,
		EnableTracing:     true,
		MetricsBatchSize:  100,
		FlushInterval:     30 * time.Second,
		RetentionDuration: 24 * time.Hour,
	}
}

// Monitor handles structured logging and metrics collection
type Monitor struct {
	config  *MonitoringConfig
	logger  *slog.Logger
	metrics map[string]*Metric
	mu      sync.RWMutex

	// Channels for async processing
	metricsChan chan *Metric
	done        chan struct{}
}

// NewMonitor creates a new monitoring instance
func NewMonitor(config *MonitoringConfig, logger *slog.Logger) *Monitor {
	if config == nil {
		config = DefaultMonitoringConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}

	monitor := &Monitor{
		config:      config,
		logger:      logger,
		metrics:     make(map[string]*Metric),
		metricsChan: make(chan *Metric, config.MetricsBatchSize),
		done:        make(chan struct{}),
	}

	// Start async processing
	if config.EnableMetrics {
		go monitor.processMetrics()
	}

	return monitor
}

// LogAPIRequest logs an API request with structured information
func (m *Monitor) LogAPIRequest(ctx context.Context, method, endpoint string, statusCode int, duration time.Duration, err error) {
	attributes := []any{
		"method", method,
		"endpoint", endpoint,
		"status_code", statusCode,
		"duration_ms", duration.Milliseconds(),
		"success", err == nil,
	}

	if err != nil {
		attributes = append(attributes, "error", err.Error())
		m.logger.ErrorContext(ctx, "API request failed", attributes...)
	} else {
		m.logger.InfoContext(ctx, "API request completed", attributes...)
	}

	// Record metric
	if m.config.EnableMetrics {
		m.RecordAPIRequestMetric(method, endpoint, statusCode, duration, err)
	}
}

// LogCircuitBreakerEvent logs circuit breaker state changes
func (m *Monitor) LogCircuitBreakerEvent(ctx context.Context, oldState, newState CircuitBreakerState, service string) {
	m.logger.InfoContext(ctx, "Circuit breaker state transition",
		"service", service,
		"old_state", oldState.String(),
		"new_state", newState.String(),
		"timestamp", time.Now().Format(time.RFC3339))

	// Record metric
	if m.config.EnableMetrics {
		m.RecordCounter("circuit_breaker_state_transitions_total", 1, map[string]string{
			"service":   service,
			"old_state": oldState.String(),
			"new_state": newState.String(),
		})
	}
}

// LogTokenRefresh logs OAuth2 token refresh events
func (m *Monitor) LogTokenRefresh(ctx context.Context, success bool, duration time.Duration, err error) {
	attributes := []any{
		"success", success,
		"duration_ms", duration.Milliseconds(),
	}

	if err != nil {
		attributes = append(attributes, "error", err.Error())
		m.logger.ErrorContext(ctx, "Token refresh failed", attributes...)
	} else {
		m.logger.InfoContext(ctx, "Token refresh completed", attributes...)
	}

	// Record metric
	if m.config.EnableMetrics {
		status := "success"
		if err != nil {
			status = "failure"
		}
		m.RecordCounter("token_refresh_total", 1, map[string]string{
			"status": status,
		})
		m.RecordHistogram("token_refresh_duration_seconds", duration.Seconds(), map[string]string{
			"status": status,
		})
	}
}

// LogArticleProcessing logs article processing events
func (m *Monitor) LogArticleProcessing(ctx context.Context, operation string, articleCount int, success bool, duration time.Duration, err error) {
	attributes := []any{
		"operation", operation,
		"article_count", articleCount,
		"success", success,
		"duration_ms", duration.Milliseconds(),
	}

	if err != nil {
		attributes = append(attributes, "error", err.Error())
		m.logger.ErrorContext(ctx, "Article processing failed", attributes...)
	} else {
		m.logger.InfoContext(ctx, "Article processing completed", attributes...)
	}

	// Record metrics
	if m.config.EnableMetrics {
		status := "success"
		if err != nil {
			status = "failure"
		}
		m.RecordCounter("articles_processed_total", float64(articleCount), map[string]string{
			"operation": operation,
			"status":    status,
		})
		m.RecordHistogram("article_processing_duration_seconds", duration.Seconds(), map[string]string{
			"operation": operation,
			"status":    status,
		})
	}
}

// RecordCounter records a counter metric
func (m *Monitor) RecordCounter(name string, value float64, labels map[string]string) {
	if !m.config.EnableMetrics {
		return
	}

	metric := &Metric{
		Name:      name,
		Type:      MetricTypeCounter,
		Value:     value,
		Labels:    labels,
		Timestamp: time.Now(),
	}

	select {
	case m.metricsChan <- metric:
	default:
		m.logger.Warn("Metrics channel full, dropping metric", "name", name)
	}
}

// RecordHistogram records a histogram metric
func (m *Monitor) RecordHistogram(name string, value float64, labels map[string]string) {
	if !m.config.EnableMetrics {
		return
	}

	metric := &Metric{
		Name:      name,
		Type:      MetricTypeHistogram,
		Value:     value,
		Labels:    labels,
		Timestamp: time.Now(),
	}

	select {
	case m.metricsChan <- metric:
	default:
		m.logger.Warn("Metrics channel full, dropping metric", "name", name)
	}
}

// RecordGauge records a gauge metric
func (m *Monitor) RecordGauge(name string, value float64, labels map[string]string) {
	if !m.config.EnableMetrics {
		return
	}

	metric := &Metric{
		Name:      name,
		Type:      MetricTypeGauge,
		Value:     value,
		Labels:    labels,
		Timestamp: time.Now(),
	}

	select {
	case m.metricsChan <- metric:
	default:
		m.logger.Warn("Metrics channel full, dropping metric", "name", name)
	}
}

// RecordAPIRequestMetric records metrics for API requests
func (m *Monitor) RecordAPIRequestMetric(method, endpoint string, statusCode int, duration time.Duration, err error) {
	status := "success"
	if err != nil {
		status = "failure"
	}

	labels := map[string]string{
		"method":   method,
		"endpoint": endpoint,
		"status":   status,
	}

	// Record request count
	m.RecordCounter("api_requests_total", 1, labels)

	// Record request duration
	m.RecordHistogram("api_request_duration_seconds", duration.Seconds(), labels)

	// Record response status code
	statusLabels := map[string]string{
		"method":      method,
		"endpoint":    endpoint,
		"status_code": string(rune(statusCode + 48)), // Convert to string
	}
	m.RecordCounter("api_response_status_total", 1, statusLabels)
}

// GetMetrics returns current metrics snapshot
func (m *Monitor) GetMetrics() map[string]*Metric {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := make(map[string]*Metric)
	for k, v := range m.metrics {
		snapshot[k] = v
	}
	return snapshot
}

// processMetrics handles async metric processing
func (m *Monitor) processMetrics() {
	ticker := time.NewTicker(m.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case metric := <-m.metricsChan:
			m.storeMetric(metric)
		case <-ticker.C:
			m.flushOldMetrics()
		case <-m.done:
			return
		}
	}
}

// storeMetric stores a metric in memory
func (m *Monitor) storeMetric(metric *Metric) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.generateMetricKey(metric)

	// For counters, accumulate values
	if existing, exists := m.metrics[key]; exists && metric.Type == MetricTypeCounter {
		existing.Value += metric.Value
		existing.Timestamp = metric.Timestamp
	} else {
		m.metrics[key] = metric
	}
}

// generateMetricKey generates a unique key for a metric
func (m *Monitor) generateMetricKey(metric *Metric) string {
	key := metric.Name + ":" + string(metric.Type)

	// Add labels to key for uniqueness
	for k, v := range metric.Labels {
		key += ":" + k + "=" + v
	}

	return key
}

// flushOldMetrics removes metrics older than retention duration
func (m *Monitor) flushOldMetrics() {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-m.config.RetentionDuration)
	for key, metric := range m.metrics {
		if metric.Timestamp.Before(cutoff) {
			delete(m.metrics, key)
		}
	}
}

// Close shuts down the monitor
func (m *Monitor) Close() {
	close(m.done)
}

// HealthCheck performs a health check of the monitoring system
func (m *Monitor) HealthCheck() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"status":             "healthy",
		"metrics_enabled":    m.config.EnableMetrics,
		"tracing_enabled":    m.config.EnableTracing,
		"metrics_count":      len(m.metrics),
		"queue_length":       len(m.metricsChan),
		"queue_capacity":     cap(m.metricsChan),
		"flush_interval":     m.config.FlushInterval.String(),
		"retention_duration": m.config.RetentionDuration.String(),
	}
}
