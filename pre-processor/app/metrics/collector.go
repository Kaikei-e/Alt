// ABOUTME: This file implements metrics collection system for performance monitoring and SLA tracking
// ABOUTME: Provides aggregation, reporting, and HTTP endpoint integration for monitoring dashboards
package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"pre-processor/config"
)

// metricsListenerEnabledLog and metricsListenerDisabledLog state the metrics
// listener's wiring on one loud line. Exactly one of them is emitted at
// startup, so "nothing is scraping this service" can never be inferred from
// silence (CLAUDE.md rule 8).
const (
	metricsListenerEnabledLog  = "metrics_listener_enabled"
	metricsListenerDisabledLog = "metrics_listener_disabled"
)

// PrometheusExporter renders extra series in Prometheus text exposition
// format. Subsystems that own their own gauges (the notification-outbox relay,
// for one) register themselves here instead of hanging a route off the service
// API listener, whose access control is only "who can open a socket".
type PrometheusExporter interface {
	Prometheus() string
}

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
	Aggregate     *AggregateMetrics         `json:"aggregate"`
	DomainMetrics map[string]*DomainMetrics `json:"domains"`
	ExportTime    time.Time                 `json:"export_time"`
	ServiceName   string                    `json:"service_name"`
}

// Collector manages metric collection and aggregation
type Collector struct {
	enabled           bool
	port              int
	path              string
	updateInterval    time.Duration
	readHeaderTimeout time.Duration
	readTimeout       time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
	logger            *slog.Logger

	// Metrics storage
	metrics map[string]*DomainMetrics
	mu      sync.RWMutex

	// Extra exposition sources appended to the Prometheus endpoint
	exporters   map[string]PrometheusExporter
	exportersMu sync.RWMutex

	// HTTP server
	server   *http.Server
	listener net.Listener
	serverMu sync.Mutex
	serverWg sync.WaitGroup
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
		enabled:           cfg.Enabled,
		port:              cfg.Port,
		path:              cfg.Path,
		updateInterval:    cfg.UpdateInterval,
		readHeaderTimeout: cfg.ReadHeaderTimeout,
		readTimeout:       cfg.ReadTimeout,
		writeTimeout:      cfg.WriteTimeout,
		idleTimeout:       cfg.IdleTimeout,
		logger:            logger,
		metrics:           make(map[string]*DomainMetrics),
		exporters:         make(map[string]PrometheusExporter),
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

// RegisterExporter attaches an extra exposition source under name. It fails on
// a missing exporter or a duplicate name rather than accepting them, because
// both produce a scrape target that looks alive while the series it was added
// for is absent or doubled.
func (c *Collector) RegisterExporter(name string, exporter PrometheusExporter) error {
	if name == "" {
		return errors.New("metrics exporter: name is required")
	}
	if exporter == nil {
		return fmt.Errorf("metrics exporter %q is not wired", name)
	}

	c.exportersMu.Lock()
	defer c.exportersMu.Unlock()

	if _, exists := c.exporters[name]; exists {
		return fmt.Errorf("metrics exporter %q is already registered", name)
	}
	c.exporters[name] = exporter

	c.logger.Info("metrics exporter registered", "exporter", name)
	return nil
}

// exportRegistered renders every registered exporter, sorted by name so the
// exposition is stable across scrapes.
func (c *Collector) exportRegistered() string {
	c.exportersMu.RLock()
	defer c.exportersMu.RUnlock()

	names := make([]string, 0, len(c.exporters))
	for name := range c.exporters {
		names = append(names, name)
	}
	sort.Strings(names)

	var builder strings.Builder
	for _, name := range names {
		builder.WriteString(c.exporters[name].Prometheus())
	}
	return builder.String()
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
	metricsCopy := *metrics
	return &metricsCopy
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
		metricsCopy := *metrics
		exportData.DomainMetrics[domain] = &metricsCopy
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

	builder.WriteString(c.exportRegistered())

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

// Start binds the dedicated metrics listener and serves it in the background.
//
// The listener is deliberately separate from the service API listener: the API
// listener's access control is "who can open a socket", so a metrics route on
// it is a new unauthenticated surface on the API.
//
// The bind is synchronous, so a port that is already taken fails the caller
// instead of leaving the process running with nothing to scrape. errCh is
// required and carries any serve failure after a successful bind.
func (c *Collector) Start(ctx context.Context, errCh chan<- error) error {
	if !c.enabled {
		c.logger.Warn(metricsListenerDisabledLog,
			"reason", "METRICS_ENABLED=false",
			"consequence", "no scrape target for this service, including the notification-outbox relay gauges")
		return nil
	}

	if errCh == nil {
		return errors.New("metrics server: an error channel is required so a serve failure reaches the composition root")
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

		if _, err := w.Write(jsonData); err != nil {
			c.logger.Error("failed to write JSON response", "error", err)
		}
	})

	// Prometheus metrics endpoint
	mux.HandleFunc(c.path+"/prometheus", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if _, err := w.Write([]byte(c.ExportPrometheus())); err != nil {
			c.logger.Error("failed to write Prometheus response", "error", err)
		}
	})

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"status":"healthy","service":"pre-processor-metrics"}`)); err != nil {
			c.logger.Error("failed to write health response", "error", err)
		}
	})

	addr := fmt.Sprintf(":%d", c.port)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: c.readHeaderTimeout,
		ReadTimeout:       c.readTimeout,
		WriteTimeout:      c.writeTimeout,
		IdleTimeout:       c.idleTimeout,
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("metrics listener on %s: %w", addr, err)
	}

	c.server = server
	c.listener = listener
	c.serverWg.Add(1)

	c.logger.Info(metricsListenerEnabledLog,
		"addr", listener.Addr().String(),
		"prometheus_path", c.path+"/prometheus",
		"json_path", c.path)

	go func() {
		defer c.serverWg.Done()

		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			c.logger.Error("metrics server failed", "error", err)
			errCh <- fmt.Errorf("metrics server: %w", err)
		}

		c.logger.Info("metrics server stopped")
	}()

	return nil
}

// Addr reports the address the metrics listener bound to, or the empty string
// when it is not running. A port-0 bind only resolves at listen time, so this
// is how a caller learns where to scrape.
func (c *Collector) Addr() string {
	c.serverMu.Lock()
	defer c.serverMu.Unlock()

	if c.listener == nil {
		return ""
	}
	return c.listener.Addr().String()
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

	// Shutdown closes the listener it was serving on, so dropping the
	// reference here is enough to make Addr report "not running".
	err := c.server.Shutdown(ctx)
	c.server = nil
	c.listener = nil

	if err != nil {
		c.logger.Error("error stopping metrics server", "error", err)
		return err
	}

	c.logger.Info("metrics server stopped")
	return nil
}
