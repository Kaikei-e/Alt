// ABOUTME: This file implements metrics collection system for performance monitoring and SLA tracking
// ABOUTME: Provides aggregation, reporting, and HTTP endpoint integration for monitoring dashboards
package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"pre-processor/config"
)

// DomainMetrics tracks performance metrics for a specific domain
type DomainMetrics struct {
	Domain            string        `json:"domain"`
	TotalRequests     int64         `json:"total_requests"`
	SuccessCount      int64         `json:"success_count"`
	FailureCount      int64         `json:"failure_count"`
	SuccessRate       float64       `json:"success_rate"`
	AvgResponseTime   time.Duration `json:"avg_response_time_ms"`
	MinResponseTime   time.Duration `json:"min_response_time_ms"`
	MaxResponseTime   time.Duration `json:"max_response_time_ms"`
	LastRequestTime   time.Time     `json:"last_request_time"`
	FirstRequestTime  time.Time     `json:"first_request_time"`
	TotalResponseTime time.Duration `json:"-"` // Internal field for calculation
}

// AggregateMetrics provides system-wide performance statistics
type AggregateMetrics struct {
	TotalRequests   int64         `json:"total_requests"`
	SuccessCount    int64         `json:"success_count"`
	FailureCount    int64         `json:"failure_count"`
	SuccessRate     float64       `json:"success_rate"`
	AvgResponseTime time.Duration `json:"avg_response_time_ms"`
	ActiveDomains   int           `json:"active_domains"`
	CollectionTime  time.Time     `json:"collection_time"`
}

// ExportData contains all metrics for export
type ExportData struct {
	Aggregate     *AggregateMetrics          `json:"aggregate"`
	DomainMetrics map[string]*DomainMetrics `json:"domains"`
	ExportTime    time.Time                  `json:"export_time"`
	ServiceName   string                     `json:"service_name"`
}

// Collector manages metric collection and aggregation
type Collector struct {
	enabled        bool
	port           int
	path           string
	updateInterval time.Duration
	logger         *slog.Logger

	// Metrics storage
	metrics map[string]*DomainMetrics
	mu      sync.RWMutex

	// HTTP server
	server   *http.Server
	serverMu sync.Mutex
}

// NewCollector creates a new metrics collector
func NewCollector(cfg config.MetricsConfig, logger *slog.Logger) (*Collector, error) {
	if cfg.Enabled {
		if cfg.Port < 0 || cfg.Port > 65535 {
			return nil, errors.New("invalid metrics port")
		}
		if cfg.UpdateInterval <= 0 {
			return nil, errors.New("invalid update interval")
		}
	}

	collector := &Collector{
		enabled:        cfg.Enabled,
		port:           cfg.Port,
		path:           cfg.Path,
		updateInterval: cfg.UpdateInterval,
		logger:         logger,
		metrics:        make(map[string]*DomainMetrics),
	}

	if cfg.Path == "" {
		collector.path = "/metrics"
	}

	logger.Info("metrics collector initialized",
		"enabled", cfg.Enabled,
		"port", cfg.Port,
		"path", cfg.Path,
		"update_interval", cfg.UpdateInterval)

	return collector, nil
}

// RecordRequest records a request metric for a domain
func (c *Collector) RecordRequest(domain string, responseTime time.Duration, success bool) {
	if !c.enabled {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	
	domainMetrics, exists := c.metrics[domain]
	if !exists {
		domainMetrics = &DomainMetrics{
			Domain:           domain,
			FirstRequestTime: now,
			MinResponseTime:  responseTime,
			MaxResponseTime:  responseTime,
		}
		c.metrics[domain] = domainMetrics
	}

	// Update counters
	domainMetrics.TotalRequests++
	domainMetrics.LastRequestTime = now
	domainMetrics.TotalResponseTime += responseTime

	if success {
		domainMetrics.SuccessCount++
	} else {
		domainMetrics.FailureCount++
	}

	// Update response time statistics
	if responseTime < domainMetrics.MinResponseTime {
		domainMetrics.MinResponseTime = responseTime
	}
	if responseTime > domainMetrics.MaxResponseTime {
		domainMetrics.MaxResponseTime = responseTime
	}

	// Calculate derived metrics
	if domainMetrics.TotalRequests > 0 {
		domainMetrics.SuccessRate = float64(domainMetrics.SuccessCount) / float64(domainMetrics.TotalRequests)
		domainMetrics.AvgResponseTime = time.Duration(domainMetrics.TotalResponseTime.Nanoseconds() / domainMetrics.TotalRequests)
	}

	c.logger.Debug("recorded request metric",
		"domain", domain,
		"response_time", responseTime,
		"success", success,
		"total_requests", domainMetrics.TotalRequests,
		"success_rate", domainMetrics.SuccessRate)
}

// GetDomainMetrics returns metrics for a specific domain
func (c *Collector) GetDomainMetrics(domain string) *DomainMetrics {
	if !c.enabled {
		return nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	metrics, exists := c.metrics[domain]
	if !exists {
		return nil
	}

	// Return a copy to avoid race conditions
	copy := *metrics
	return &copy
}

// GetAggregateMetrics returns system-wide aggregate metrics
func (c *Collector) GetAggregateMetrics() *AggregateMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	aggregate := &AggregateMetrics{
		CollectionTime: time.Now(),
		ActiveDomains:  len(c.metrics),
	}

	var totalResponseTime time.Duration

	for _, domainMetrics := range c.metrics {
		aggregate.TotalRequests += domainMetrics.TotalRequests
		aggregate.SuccessCount += domainMetrics.SuccessCount
		aggregate.FailureCount += domainMetrics.FailureCount
		totalResponseTime += domainMetrics.TotalResponseTime
	}

	if aggregate.TotalRequests > 0 {
		aggregate.SuccessRate = float64(aggregate.SuccessCount) / float64(aggregate.TotalRequests)
		aggregate.AvgResponseTime = time.Duration(totalResponseTime.Nanoseconds() / aggregate.TotalRequests)
	}

	return aggregate
}

// ExportJSON exports all metrics in JSON format
func (c *Collector) ExportJSON() ([]byte, error) {
	if !c.enabled {
		return []byte("{}"), nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	exportData := &ExportData{
		Aggregate:     c.GetAggregateMetrics(),
		DomainMetrics: make(map[string]*DomainMetrics),
		ExportTime:    time.Now(),
		ServiceName:   "pre-processor",
	}

	// Copy domain metrics
	for domain, metrics := range c.metrics {
		copy := *metrics
		exportData.DomainMetrics[domain] = &copy
	}

	return json.MarshalIndent(exportData, "", "  ")
}

// ExportPrometheus exports metrics in Prometheus format
func (c *Collector) ExportPrometheus() string {
	if !c.enabled {
		return ""
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	var builder strings.Builder

	// Write headers
	builder.WriteString("# HELP preprocessor_requests_total Total number of requests processed\n")
	builder.WriteString("# TYPE preprocessor_requests_total counter\n")

	builder.WriteString("# HELP preprocessor_requests_success_total Total number of successful requests\n")
	builder.WriteString("# TYPE preprocessor_requests_success_total counter\n")

	builder.WriteString("# HELP preprocessor_requests_failure_total Total number of failed requests\n")
	builder.WriteString("# TYPE preprocessor_requests_failure_total counter\n")

	builder.WriteString("# HELP preprocessor_response_time_seconds Average response time in seconds\n")
	builder.WriteString("# TYPE preprocessor_response_time_seconds gauge\n")

	builder.WriteString("# HELP preprocessor_success_rate Ratio of successful requests\n")
	builder.WriteString("# TYPE preprocessor_success_rate gauge\n")

	// Sort domains for consistent output
	domains := make([]string, 0, len(c.metrics))
	for domain := range c.metrics {
		domains = append(domains, domain)
	}
	sort.Strings(domains)

	// Write domain-specific metrics
	for _, domain := range domains {
		metrics := c.metrics[domain]
		
		builder.WriteString(fmt.Sprintf("preprocessor_requests_total{domain=\"%s\"} %d\n", 
			domain, metrics.TotalRequests))
		builder.WriteString(fmt.Sprintf("preprocessor_requests_success_total{domain=\"%s\"} %d\n", 
			domain, metrics.SuccessCount))
		builder.WriteString(fmt.Sprintf("preprocessor_requests_failure_total{domain=\"%s\"} %d\n", 
			domain, metrics.FailureCount))
		builder.WriteString(fmt.Sprintf("preprocessor_response_time_seconds{domain=\"%s\"} %.6f\n", 
			domain, metrics.AvgResponseTime.Seconds()))
		builder.WriteString(fmt.Sprintf("preprocessor_success_rate{domain=\"%s\"} %.4f\n", 
			domain, metrics.SuccessRate))
	}

	// Write aggregate metrics
	aggregate := c.GetAggregateMetrics()
	builder.WriteString(fmt.Sprintf("preprocessor_requests_total{domain=\"_aggregate\"} %d\n", 
		aggregate.TotalRequests))
	builder.WriteString(fmt.Sprintf("preprocessor_requests_success_total{domain=\"_aggregate\"} %d\n", 
		aggregate.SuccessCount))
	builder.WriteString(fmt.Sprintf("preprocessor_requests_failure_total{domain=\"_aggregate\"} %d\n", 
		aggregate.FailureCount))
	builder.WriteString(fmt.Sprintf("preprocessor_response_time_seconds{domain=\"_aggregate\"} %.6f\n", 
		aggregate.AvgResponseTime.Seconds()))
	builder.WriteString(fmt.Sprintf("preprocessor_success_rate{domain=\"_aggregate\"} %.4f\n", 
		aggregate.SuccessRate))

	return builder.String()
}

// Reset clears all collected metrics
func (c *Collector) Reset() {
	if !c.enabled {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.metrics = make(map[string]*DomainMetrics)
	c.logger.Info("metrics reset completed")
}

// Cleanup removes old domain metrics to prevent memory leaks
func (c *Collector) Cleanup() {
	if !c.enabled {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	cleanupThreshold := 24 * time.Hour // Remove domains unused for 24 hours
	removed := 0

	for domain, metrics := range c.metrics {
		if now.Sub(metrics.LastRequestTime) > cleanupThreshold {
			delete(c.metrics, domain)
			removed++
		}
	}

	if removed > 0 {
		c.logger.Info("metrics cleanup completed", 
			"removed_domains", removed,
			"remaining_domains", len(c.metrics))
	}
}

// Start starts the HTTP metrics server
func (c *Collector) Start(ctx context.Context) error {
	if !c.enabled {
		return nil
	}

	c.serverMu.Lock()
	defer c.serverMu.Unlock()

	if c.server != nil {
		return errors.New("metrics server already running")
	}

	mux := http.NewServeMux()
	
	// JSON metrics endpoint
	mux.HandleFunc(c.path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		jsonData, err := c.ExportJSON()
		if err != nil {
			c.logger.Error("failed to export JSON metrics", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		
		w.Write(jsonData)
	})

	// Prometheus metrics endpoint
	mux.HandleFunc(c.path+"/prometheus", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(c.ExportPrometheus()))
	})

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"healthy","service":"pre-processor-metrics"}`))
	})

	c.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", c.port),
		Handler: mux,
	}

	go func() {
		c.logger.Info("starting metrics server", 
			"port", c.port, 
			"path", c.path)
		
		if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			c.logger.Error("metrics server failed", "error", err)
		}
	}()

	return nil
}

// Stop stops the HTTP metrics server
func (c *Collector) Stop(ctx context.Context) error {
	if !c.enabled {
		return nil
	}

	c.serverMu.Lock()
	defer c.serverMu.Unlock()

	if c.server == nil {
		return nil
	}

	c.logger.Info("stopping metrics server")

	err := c.server.Shutdown(ctx)
	c.server = nil

	if err != nil {
		c.logger.Error("error stopping metrics server", "error", err)
		return err
	}

	c.logger.Info("metrics server stopped")
	return nil
}