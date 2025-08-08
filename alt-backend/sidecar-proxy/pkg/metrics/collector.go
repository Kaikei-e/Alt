// Package metrics provides monitoring and metrics collection for the proxy sidecar
// This package implements the comprehensive monitoring system described in ISSUE_RESOLVE_PLAN.md
// to track upstream resolution success and overall system performance.
package metrics

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Collector manages all metrics for the proxy sidecar
// This provides detailed insight into the upstream resolution performance
type Collector struct {
	// Basic counters (atomic for thread safety)
	totalRequests      int64
	successfulRequests int64
	failedRequests     int64

	// DNS metrics
	dnsRequests    int64
	dnsSuccesses   int64
	dnsFailures    int64
	dnsCacheHits   int64
	dnsCacheMisses int64

	// Upstream resolution metrics (key for solving the problem)
	upstreamResolutions int64
	upstreamFailures    int64

	// Domain-specific metrics
	domainMetrics map[string]*DomainMetrics
	domainMutex   sync.RWMutex

	// Performance metrics
	responseTimes      []time.Duration
	responseTimesMutex sync.RWMutex

	// Error tracking
	errorsByType map[string]int64
	errorsMutex  sync.RWMutex

	// System info
	startTime   time.Time
	serviceName string
	version     string

	// Configuration
	maxResponseTimes int
}

// DomainMetrics tracks metrics for individual domains
type DomainMetrics struct {
	TotalRequests  int64         `json:"total_requests"`
	SuccessfulReqs int64         `json:"successful_requests"`
	FailedRequests int64         `json:"failed_requests"`
	AverageLatency time.Duration `json:"average_latency"`
	LastRequest    time.Time     `json:"last_request"`
	FirstRequest   time.Time     `json:"first_request"`
	ErrorRate      float64       `json:"error_rate"`
}

// ProxyMetrics represents the complete metrics snapshot
type ProxyMetrics struct {
	// Basic request metrics
	TotalRequests      int64   `json:"total_requests"`
	SuccessfulRequests int64   `json:"successful_requests"`
	FailedRequests     int64   `json:"failed_requests"`
	RequestRate        float64 `json:"request_rate"`
	ErrorRate          float64 `json:"error_rate"`

	// DNS metrics
	DNSRequests     int64   `json:"dns_requests"`
	DNSSuccesses    int64   `json:"dns_successes"`
	DNSFailures     int64   `json:"dns_failures"`
	DNSCacheHits    int64   `json:"dns_cache_hits"`
	DNSCacheMisses  int64   `json:"dns_cache_misses"`
	DNSCacheHitRate float64 `json:"dns_cache_hit_rate"`

	// Upstream resolution metrics (critical for problem tracking)
	UpstreamResolutions int64   `json:"upstream_resolutions"`
	UpstreamFailures    int64   `json:"upstream_failures"`
	UpstreamSuccessRate float64 `json:"upstream_success_rate"`

	// Performance metrics
	AverageResponseTime time.Duration `json:"average_response_time"`
	P50ResponseTime     time.Duration `json:"p50_response_time"`
	P95ResponseTime     time.Duration `json:"p95_response_time"`
	P99ResponseTime     time.Duration `json:"p99_response_time"`

	// Domain breakdown
	DomainMetrics map[string]*DomainMetrics `json:"domain_metrics"`

	// Error tracking
	ErrorsByType map[string]int64 `json:"errors_by_type"`

	// System information
	Uptime      time.Duration `json:"uptime"`
	StartTime   time.Time     `json:"start_time"`
	ServiceName string        `json:"service_name"`
	Version     string        `json:"version"`

	// Timestamps
	LastUpdate  time.Time `json:"last_update"`
	LastRequest time.Time `json:"last_request"`
}

// NewCollector creates a new metrics collector
func NewCollector(serviceName string) *Collector {
	return &Collector{
		domainMetrics:    make(map[string]*DomainMetrics),
		errorsByType:     make(map[string]int64),
		responseTimes:    make([]time.Duration, 0, 1000),
		startTime:        time.Now(),
		serviceName:      serviceName,
		version:          "1.0.0",
		maxResponseTimes: 1000,
	}
}

// RecordRequest records a completed request with its metrics
// This is called for every proxy request to track upstream resolution success
func (c *Collector) RecordRequest(domain string, statusCode int, duration time.Duration) {
	atomic.AddInt64(&c.totalRequests, 1)

	// Determine if request was successful
	if statusCode >= 200 && statusCode < 400 {
		atomic.AddInt64(&c.successfulRequests, 1)
		atomic.AddInt64(&c.upstreamResolutions, 1) // Count successful upstream resolutions
	} else {
		atomic.AddInt64(&c.failedRequests, 1)
		atomic.AddInt64(&c.upstreamFailures, 1)
	}

	// Record response time
	c.recordResponseTime(duration)

	// Update domain-specific metrics
	c.updateDomainMetrics(domain, statusCode, duration)
}

// RecordDNSQuery records DNS resolution metrics
func (c *Collector) RecordDNSQuery(success bool, cached bool) {
	atomic.AddInt64(&c.dnsRequests, 1)

	if success {
		atomic.AddInt64(&c.dnsSuccesses, 1)
	} else {
		atomic.AddInt64(&c.dnsFailures, 1)
	}

	if cached {
		atomic.AddInt64(&c.dnsCacheHits, 1)
	} else {
		atomic.AddInt64(&c.dnsCacheMisses, 1)
	}
}

// RecordError records an error by type for analysis
func (c *Collector) RecordError(errorType string) {
	c.errorsMutex.Lock()
	defer c.errorsMutex.Unlock()

	c.errorsByType[errorType]++
}

// GetMetrics returns a complete metrics snapshot as JSON
func (c *Collector) GetMetrics() string {
	metrics := c.buildMetricsSnapshot()

	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to marshal metrics: %v"}`, err)
	}

	return string(data)
}

// GetPrometheusMetrics returns metrics in Prometheus format
func (c *Collector) GetPrometheusMetrics() string {
	metrics := c.buildMetricsSnapshot()

	var output string

	// Basic request metrics
	output += fmt.Sprintf("# HELP proxy_sidecar_requests_total Total number of proxy requests\n")
	output += fmt.Sprintf("# TYPE proxy_sidecar_requests_total counter\n")
	output += fmt.Sprintf("proxy_sidecar_requests_total{status=\"success\"} %d\n", metrics.SuccessfulRequests)
	output += fmt.Sprintf("proxy_sidecar_requests_total{status=\"failed\"} %d\n", metrics.FailedRequests)

	// DNS metrics
	output += fmt.Sprintf("# HELP proxy_sidecar_dns_requests_total Total DNS requests\n")
	output += fmt.Sprintf("# TYPE proxy_sidecar_dns_requests_total counter\n")
	output += fmt.Sprintf("proxy_sidecar_dns_requests_total %d\n", metrics.DNSRequests)

	output += fmt.Sprintf("# HELP proxy_sidecar_dns_cache_operations_total DNS cache operations\n")
	output += fmt.Sprintf("# TYPE proxy_sidecar_dns_cache_operations_total counter\n")
	output += fmt.Sprintf("proxy_sidecar_dns_cache_operations_total{operation=\"hit\"} %d\n", metrics.DNSCacheHits)
	output += fmt.Sprintf("proxy_sidecar_dns_cache_operations_total{operation=\"miss\"} %d\n", metrics.DNSCacheMisses)

	// Upstream resolution metrics (KEY METRICS for tracking the solution)
	output += fmt.Sprintf("# HELP proxy_sidecar_upstream_resolutions_total Successful upstream resolutions\n")
	output += fmt.Sprintf("# TYPE proxy_sidecar_upstream_resolutions_total counter\n")
	output += fmt.Sprintf("proxy_sidecar_upstream_resolutions_total %d\n", metrics.UpstreamResolutions)

	output += fmt.Sprintf("# HELP proxy_sidecar_upstream_failures_total Failed upstream resolutions\n")
	output += fmt.Sprintf("# TYPE proxy_sidecar_upstream_failures_total counter\n")
	output += fmt.Sprintf("proxy_sidecar_upstream_failures_total %d\n", metrics.UpstreamFailures)

	output += fmt.Sprintf("# HELP proxy_sidecar_upstream_success_rate Success rate of upstream resolution\n")
	output += fmt.Sprintf("# TYPE proxy_sidecar_upstream_success_rate gauge\n")
	output += fmt.Sprintf("proxy_sidecar_upstream_success_rate %.6f\n", metrics.UpstreamSuccessRate)

	// Response time metrics
	output += fmt.Sprintf("# HELP proxy_sidecar_response_time_seconds Response time percentiles\n")
	output += fmt.Sprintf("# TYPE proxy_sidecar_response_time_seconds gauge\n")
	output += fmt.Sprintf("proxy_sidecar_response_time_seconds{quantile=\"0.50\"} %.6f\n", metrics.P50ResponseTime.Seconds())
	output += fmt.Sprintf("proxy_sidecar_response_time_seconds{quantile=\"0.95\"} %.6f\n", metrics.P95ResponseTime.Seconds())
	output += fmt.Sprintf("proxy_sidecar_response_time_seconds{quantile=\"0.99\"} %.6f\n", metrics.P99ResponseTime.Seconds())

	// Error metrics by type
	output += fmt.Sprintf("# HELP proxy_sidecar_errors_total Errors by type\n")
	output += fmt.Sprintf("# TYPE proxy_sidecar_errors_total counter\n")
	for errorType, count := range metrics.ErrorsByType {
		output += fmt.Sprintf("proxy_sidecar_errors_total{type=\"%s\"} %d\n", errorType, count)
	}

	// System metrics
	output += fmt.Sprintf("# HELP proxy_sidecar_uptime_seconds Service uptime\n")
	output += fmt.Sprintf("# TYPE proxy_sidecar_uptime_seconds gauge\n")
	output += fmt.Sprintf("proxy_sidecar_uptime_seconds %.0f\n", metrics.Uptime.Seconds())

	return output
}

// GetDomainMetrics returns metrics for a specific domain
func (c *Collector) GetDomainMetrics(domain string) *DomainMetrics {
	c.domainMutex.RLock()
	defer c.domainMutex.RUnlock()

	return c.domainMetrics[domain]
}

// GetUpstreamResolutionRate returns the success rate of upstream resolution
// This is a key metric for monitoring the effectiveness of our solution
func (c *Collector) GetUpstreamResolutionRate() float64 {
	total := atomic.LoadInt64(&c.upstreamResolutions) + atomic.LoadInt64(&c.upstreamFailures)
	if total == 0 {
		return 0.0
	}

	successes := atomic.LoadInt64(&c.upstreamResolutions)
	return float64(successes) / float64(total)
}

// Private helper methods

func (c *Collector) recordResponseTime(duration time.Duration) {
	c.responseTimesMutex.Lock()
	defer c.responseTimesMutex.Unlock()

	// Add new response time
	c.responseTimes = append(c.responseTimes, duration)

	// Keep only the most recent N response times
	if len(c.responseTimes) > c.maxResponseTimes {
		// Remove oldest 25% when limit exceeded
		removeCount := c.maxResponseTimes / 4
		c.responseTimes = c.responseTimes[removeCount:]
	}
}

func (c *Collector) updateDomainMetrics(domain string, statusCode int, duration time.Duration) {
	c.domainMutex.Lock()
	defer c.domainMutex.Unlock()

	metrics, exists := c.domainMetrics[domain]
	if !exists {
		metrics = &DomainMetrics{
			FirstRequest: time.Now(),
		}
		c.domainMetrics[domain] = metrics
	}

	metrics.TotalRequests++
	metrics.LastRequest = time.Now()

	if statusCode >= 200 && statusCode < 400 {
		metrics.SuccessfulReqs++
	} else {
		metrics.FailedRequests++
	}

	// Update average latency (simple moving average)
	if metrics.AverageLatency == 0 {
		metrics.AverageLatency = duration
	} else {
		// Simple exponential moving average
		alpha := 0.1
		metrics.AverageLatency = time.Duration(float64(metrics.AverageLatency)*(1-alpha) + float64(duration)*alpha)
	}

	// Calculate error rate
	if metrics.TotalRequests > 0 {
		metrics.ErrorRate = float64(metrics.FailedRequests) / float64(metrics.TotalRequests)
	}
}

func (c *Collector) buildMetricsSnapshot() *ProxyMetrics {
	// Atomic loads for thread-safe access
	totalReqs := atomic.LoadInt64(&c.totalRequests)
	successfulReqs := atomic.LoadInt64(&c.successfulRequests)
	failedReqs := atomic.LoadInt64(&c.failedRequests)

	dnsReqs := atomic.LoadInt64(&c.dnsRequests)
	dnsSuccesses := atomic.LoadInt64(&c.dnsSuccesses)
	dnsFailures := atomic.LoadInt64(&c.dnsFailures)
	dnsCacheHits := atomic.LoadInt64(&c.dnsCacheHits)
	dnsCacheMisses := atomic.LoadInt64(&c.dnsCacheMisses)

	upstreamResolutions := atomic.LoadInt64(&c.upstreamResolutions)
	upstreamFailures := atomic.LoadInt64(&c.upstreamFailures)

	// Calculate rates
	var errorRate, dnsHitRate, upstreamSuccessRate float64

	if totalReqs > 0 {
		errorRate = float64(failedReqs) / float64(totalReqs)
	}

	totalDNSRequests := dnsCacheHits + dnsCacheMisses
	if totalDNSRequests > 0 {
		dnsHitRate = float64(dnsCacheHits) / float64(totalDNSRequests)
	}

	totalUpstream := upstreamResolutions + upstreamFailures
	if totalUpstream > 0 {
		upstreamSuccessRate = float64(upstreamResolutions) / float64(totalUpstream)
	}

	// Calculate response time percentiles
	responseTimeStats := c.calculateResponseTimePercentiles()

	// Copy domain metrics
	c.domainMutex.RLock()
	domainMetricsCopy := make(map[string]*DomainMetrics)
	for domain, metrics := range c.domainMetrics {
		// Create a copy to avoid race conditions
		domainMetricsCopy[domain] = &DomainMetrics{
			TotalRequests:  metrics.TotalRequests,
			SuccessfulReqs: metrics.SuccessfulReqs,
			FailedRequests: metrics.FailedRequests,
			AverageLatency: metrics.AverageLatency,
			LastRequest:    metrics.LastRequest,
			FirstRequest:   metrics.FirstRequest,
			ErrorRate:      metrics.ErrorRate,
		}
	}
	c.domainMutex.RUnlock()

	// Copy error metrics
	c.errorsMutex.RLock()
	errorsByTypeCopy := make(map[string]int64)
	for errorType, count := range c.errorsByType {
		errorsByTypeCopy[errorType] = count
	}
	c.errorsMutex.RUnlock()

	uptime := time.Since(c.startTime)
	var requestRate float64
	if uptime.Seconds() > 0 {
		requestRate = float64(totalReqs) / uptime.Seconds()
	}

	return &ProxyMetrics{
		TotalRequests:      totalReqs,
		SuccessfulRequests: successfulReqs,
		FailedRequests:     failedReqs,
		RequestRate:        requestRate,
		ErrorRate:          errorRate,

		DNSRequests:     dnsReqs,
		DNSSuccesses:    dnsSuccesses,
		DNSFailures:     dnsFailures,
		DNSCacheHits:    dnsCacheHits,
		DNSCacheMisses:  dnsCacheMisses,
		DNSCacheHitRate: dnsHitRate,

		UpstreamResolutions: upstreamResolutions,
		UpstreamFailures:    upstreamFailures,
		UpstreamSuccessRate: upstreamSuccessRate,

		AverageResponseTime: responseTimeStats.Average,
		P50ResponseTime:     responseTimeStats.P50,
		P95ResponseTime:     responseTimeStats.P95,
		P99ResponseTime:     responseTimeStats.P99,

		DomainMetrics: domainMetricsCopy,
		ErrorsByType:  errorsByTypeCopy,

		Uptime:      uptime,
		StartTime:   c.startTime,
		ServiceName: c.serviceName,
		Version:     c.version,
		LastUpdate:  time.Now(),
	}
}

type ResponseTimeStats struct {
	Average time.Duration
	P50     time.Duration
	P95     time.Duration
	P99     time.Duration
}

func (c *Collector) calculateResponseTimePercentiles() ResponseTimeStats {
	c.responseTimesMutex.RLock()
	defer c.responseTimesMutex.RUnlock()

	if len(c.responseTimes) == 0 {
		return ResponseTimeStats{}
	}

	// Make a copy and sort for percentile calculation
	times := make([]time.Duration, len(c.responseTimes))
	copy(times, c.responseTimes)

	// Simple bubble sort for small datasets
	for i := 0; i < len(times); i++ {
		for j := 0; j < len(times)-1-i; j++ {
			if times[j] > times[j+1] {
				times[j], times[j+1] = times[j+1], times[j]
			}
		}
	}

	// Calculate average
	var total time.Duration
	for _, t := range times {
		total += t
	}
	average := total / time.Duration(len(times))

	// Calculate percentiles
	p50Index := int(float64(len(times)) * 0.50)
	p95Index := int(float64(len(times)) * 0.95)
	p99Index := int(float64(len(times)) * 0.99)

	if p50Index >= len(times) {
		p50Index = len(times) - 1
	}
	if p95Index >= len(times) {
		p95Index = len(times) - 1
	}
	if p99Index >= len(times) {
		p99Index = len(times) - 1
	}

	return ResponseTimeStats{
		Average: average,
		P50:     times[p50Index],
		P95:     times[p95Index],
		P99:     times[p99Index],
	}
}
