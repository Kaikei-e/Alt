// ABOUTME: Proxy metrics collection for Envoy proxy monitoring
// ABOUTME: Provides comprehensive latency, success rate, and performance monitoring

package service

import (
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"log/slog"
)

// ProxyMetrics collects and reports proxy performance metrics
type ProxyMetrics struct {
	logger *slog.Logger

	// Request counters
	totalRequests      uint64
	envoyRequests      uint64
	directRequests     uint64
	envoySuccessful    uint64
	directSuccessful   uint64
	envoyFailures      uint64
	directFailures     uint64

	// Latency tracking
	envoyLatencySum    uint64 // in milliseconds
	directLatencySum   uint64 // in milliseconds
	dnsResolutionSum   uint64 // in milliseconds
	dnsResolutionCount uint64

	// Error tracking
	configErrors      uint64
	timeoutErrors     uint64
	connectionErrors  uint64
	dnsErrors         uint64

	// Configuration switches
	configSwitchCount uint64
	lastSwitchTime    int64 // Unix timestamp

	// Performance windows (for moving averages)
	mutex             sync.RWMutex
	recentEnvoyTimes  []time.Duration
	recentDirectTimes []time.Duration
	windowSize        int

	// Domain-specific error tracking (Phase 5)
	domainErrors      map[string]*DomainMetrics
	domainMutex       sync.RWMutex

	// Start time for metrics collection
	startTime time.Time
}

// NewProxyMetrics creates a new proxy metrics collector
func NewProxyMetrics(logger *slog.Logger) *ProxyMetrics {
	return &ProxyMetrics{
		logger:            logger,
		windowSize:        100, // Keep last 100 measurements for moving averages
		recentEnvoyTimes:  make([]time.Duration, 0, 100),
		recentDirectTimes: make([]time.Duration, 0, 100),
		domainErrors:      make(map[string]*DomainMetrics),
		startTime:         time.Now(),
	}
}

// RecordEnvoyRequest records an Envoy proxy request with its outcome
func (m *ProxyMetrics) RecordEnvoyRequest(duration time.Duration, success bool, dnsResolutionTime time.Duration) {
	atomic.AddUint64(&m.totalRequests, 1)
	atomic.AddUint64(&m.envoyRequests, 1)

	durationMs := uint64(duration.Milliseconds())
	atomic.AddUint64(&m.envoyLatencySum, durationMs)

	if success {
		atomic.AddUint64(&m.envoySuccessful, 1)
	} else {
		atomic.AddUint64(&m.envoyFailures, 1)
	}

	// Record DNS resolution time
	if dnsResolutionTime > 0 {
		atomic.AddUint64(&m.dnsResolutionSum, uint64(dnsResolutionTime.Milliseconds()))
		atomic.AddUint64(&m.dnsResolutionCount, 1)
	}

	// Update moving average window
	m.mutex.Lock()
	m.recentEnvoyTimes = append(m.recentEnvoyTimes, duration)
	if len(m.recentEnvoyTimes) > m.windowSize {
		m.recentEnvoyTimes = m.recentEnvoyTimes[1:]
	}
	m.mutex.Unlock()

	// Log detailed metrics periodically
	if m.totalRequests%50 == 0 {
		m.logPerformanceMetrics()
	}
}

// RecordDirectRequest records a direct HTTP request with its outcome
func (m *ProxyMetrics) RecordDirectRequest(duration time.Duration, success bool) {
	atomic.AddUint64(&m.totalRequests, 1)
	atomic.AddUint64(&m.directRequests, 1)

	durationMs := uint64(duration.Milliseconds())
	atomic.AddUint64(&m.directLatencySum, durationMs)

	if success {
		atomic.AddUint64(&m.directSuccessful, 1)
	} else {
		atomic.AddUint64(&m.directFailures, 1)
	}

	// Update moving average window
	m.mutex.Lock()
	m.recentDirectTimes = append(m.recentDirectTimes, duration)
	if len(m.recentDirectTimes) > m.windowSize {
		m.recentDirectTimes = m.recentDirectTimes[1:]
	}
	m.mutex.Unlock()

	// Log detailed metrics periodically
	if m.totalRequests%50 == 0 {
		m.logPerformanceMetrics()
	}
}

// RecordError records specific types of errors for monitoring
func (m *ProxyMetrics) RecordError(errorType ProxyErrorType) {
	switch errorType {
	case ProxyErrorConfig:
		atomic.AddUint64(&m.configErrors, 1)
	case ProxyErrorTimeout:
		atomic.AddUint64(&m.timeoutErrors, 1)
	case ProxyErrorConnection:
		atomic.AddUint64(&m.connectionErrors, 1)
	case ProxyErrorDNS:
		atomic.AddUint64(&m.dnsErrors, 1)
	}

	m.logger.Warn("proxy error recorded",
		"error_type", errorType,
		"total_config_errors", atomic.LoadUint64(&m.configErrors),
		"total_timeout_errors", atomic.LoadUint64(&m.timeoutErrors),
		"total_connection_errors", atomic.LoadUint64(&m.connectionErrors),
		"total_dns_errors", atomic.LoadUint64(&m.dnsErrors))
}

// RecordConfigSwitch records when configuration switches between proxy modes
func (m *ProxyMetrics) RecordConfigSwitch(fromEnvoy, toEnvoy bool) {
	atomic.AddUint64(&m.configSwitchCount, 1)
	atomic.StoreInt64(&m.lastSwitchTime, time.Now().Unix())

	m.logger.Info("proxy configuration switch recorded",
		"from_envoy", fromEnvoy,
		"to_envoy", toEnvoy,
		"total_switches", atomic.LoadUint64(&m.configSwitchCount),
		"switch_timestamp", time.Now().Format(time.RFC3339))
}

// GetMetricsSummary returns current metrics summary for monitoring
func (m *ProxyMetrics) GetMetricsSummary() ProxyMetricsSummary {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	totalRequests := atomic.LoadUint64(&m.totalRequests)
	envoyRequests := atomic.LoadUint64(&m.envoyRequests)
	directRequests := atomic.LoadUint64(&m.directRequests)
	
	var envoyAvgLatency, directAvgLatency, dnsAvgLatency float64
	
	if envoyRequests > 0 {
		envoyAvgLatency = float64(atomic.LoadUint64(&m.envoyLatencySum)) / float64(envoyRequests)
	}
	
	if directRequests > 0 {
		directAvgLatency = float64(atomic.LoadUint64(&m.directLatencySum)) / float64(directRequests)
	}

	dnsCount := atomic.LoadUint64(&m.dnsResolutionCount)
	if dnsCount > 0 {
		dnsAvgLatency = float64(atomic.LoadUint64(&m.dnsResolutionSum)) / float64(dnsCount)
	}

	// Calculate success rates
	var envoySuccessRate, directSuccessRate float64
	if envoyRequests > 0 {
		envoySuccessRate = float64(atomic.LoadUint64(&m.envoySuccessful)) / float64(envoyRequests) * 100
	}
	if directRequests > 0 {
		directSuccessRate = float64(atomic.LoadUint64(&m.directSuccessful)) / float64(directRequests) * 100
	}

	// Calculate moving averages
	var envoyMovingAvg, directMovingAvg time.Duration
	if len(m.recentEnvoyTimes) > 0 {
		var sum time.Duration
		for _, d := range m.recentEnvoyTimes {
			sum += d
		}
		envoyMovingAvg = sum / time.Duration(len(m.recentEnvoyTimes))
	}
	if len(m.recentDirectTimes) > 0 {
		var sum time.Duration
		for _, d := range m.recentDirectTimes {
			sum += d
		}
		directMovingAvg = sum / time.Duration(len(m.recentDirectTimes))
	}

	return ProxyMetricsSummary{
		TotalRequests:       totalRequests,
		EnvoyRequests:       envoyRequests,
		DirectRequests:      directRequests,
		EnvoySuccessful:     atomic.LoadUint64(&m.envoySuccessful),
		DirectSuccessful:    atomic.LoadUint64(&m.directSuccessful),
		EnvoyFailures:       atomic.LoadUint64(&m.envoyFailures),
		DirectFailures:      atomic.LoadUint64(&m.directFailures),
		EnvoyAvgLatencyMs:   envoyAvgLatency,
		DirectAvgLatencyMs:  directAvgLatency,
		DNSAvgLatencyMs:     dnsAvgLatency,
		EnvoySuccessRate:    envoySuccessRate,
		DirectSuccessRate:   directSuccessRate,
		EnvoyMovingAvgMs:    float64(envoyMovingAvg.Milliseconds()),
		DirectMovingAvgMs:   float64(directMovingAvg.Milliseconds()),
		ConfigErrors:        atomic.LoadUint64(&m.configErrors),
		TimeoutErrors:       atomic.LoadUint64(&m.timeoutErrors),
		ConnectionErrors:    atomic.LoadUint64(&m.connectionErrors),
		DNSErrors:          atomic.LoadUint64(&m.dnsErrors),
		ConfigSwitchCount:   atomic.LoadUint64(&m.configSwitchCount),
		UptimeSeconds:       time.Since(m.startTime).Seconds(),
		CollectionStartTime: m.startTime,
	}
}

// logPerformanceMetrics logs comprehensive performance metrics
func (m *ProxyMetrics) logPerformanceMetrics() {
	summary := m.GetMetricsSummary()
	
	m.logger.Info("proxy performance metrics",
		"total_requests", summary.TotalRequests,
		"envoy_requests", summary.EnvoyRequests,
		"direct_requests", summary.DirectRequests,
		"envoy_success_rate_percent", summary.EnvoySuccessRate,
		"direct_success_rate_percent", summary.DirectSuccessRate,
		"envoy_avg_latency_ms", summary.EnvoyAvgLatencyMs,
		"direct_avg_latency_ms", summary.DirectAvgLatencyMs,
		"dns_avg_latency_ms", summary.DNSAvgLatencyMs,
		"envoy_moving_avg_ms", summary.EnvoyMovingAvgMs,
		"direct_moving_avg_ms", summary.DirectMovingAvgMs,
		"config_errors", summary.ConfigErrors,
		"timeout_errors", summary.TimeoutErrors,
		"connection_errors", summary.ConnectionErrors,
		"dns_errors", summary.DNSErrors,
		"config_switches", summary.ConfigSwitchCount,
		"uptime_seconds", summary.UptimeSeconds)

	// Performance comparison analysis
	if summary.EnvoyRequests > 10 && summary.DirectRequests > 10 {
		latencyDiff := summary.EnvoyAvgLatencyMs - summary.DirectAvgLatencyMs
		latencyDiffPercent := (latencyDiff / summary.DirectAvgLatencyMs) * 100

		m.logger.Info("proxy performance comparison",
			"envoy_vs_direct_latency_diff_ms", latencyDiff,
			"envoy_vs_direct_latency_diff_percent", latencyDiffPercent,
			"performance_impact", m.getPerformanceImpactAssessment(latencyDiffPercent))
	}
}

// getPerformanceImpactAssessment provides human-readable performance assessment
func (m *ProxyMetrics) getPerformanceImpactAssessment(diffPercent float64) string {
	switch {
	case diffPercent < -10:
		return "envoy_significantly_faster"
	case diffPercent < -5:
		return "envoy_faster"
	case diffPercent >= -5 && diffPercent <= 5:
		return "comparable_performance"
	case diffPercent > 5 && diffPercent <= 20:
		return "envoy_slower"
	case diffPercent > 20:
		return "envoy_significantly_slower"
	default:
		return "unknown"
	}
}

// ProxyErrorType defines types of proxy-related errors for categorization
type ProxyErrorType string

const (
	ProxyErrorConfig     ProxyErrorType = "config_error"
	ProxyErrorTimeout    ProxyErrorType = "timeout_error"  
	ProxyErrorConnection ProxyErrorType = "connection_error"
	ProxyErrorDNS        ProxyErrorType = "dns_error"
)

// DomainMetrics tracks metrics for a specific domain
type DomainMetrics struct {
	Domain            string  `json:"domain"`
	TotalRequests     uint64  `json:"total_requests"`
	SuccessfulRequests uint64  `json:"successful_requests"`
	FailedRequests    uint64  `json:"failed_requests"`
	BotDetectionErrors uint64  `json:"bot_detection_errors"` // 403/blocked responses
	TimeoutErrors     uint64  `json:"timeout_errors"`
	ConnectionErrors  uint64  `json:"connection_errors"`
	DNSErrors         uint64  `json:"dns_errors"`
	ConfigErrors      uint64  `json:"config_errors"`
	LatencySum        uint64  `json:"latency_sum_ms"`
	LastRequestTime   int64   `json:"last_request_time"` // Unix timestamp
	FirstErrorTime    int64   `json:"first_error_time"`  // Unix timestamp
	ConsecutiveErrors uint64  `json:"consecutive_errors"`
	
	// Success rate calculation
	SuccessRate       float64 `json:"success_rate_percent"`
	AvgLatencyMs      float64 `json:"avg_latency_ms"`
}

// GetHealthScore calculates health score for this domain
func (dm *DomainMetrics) GetHealthScore() float64 {
	if dm.TotalRequests == 0 {
		return 100.0
	}
	
	score := 100.0
	
	// Penalize low success rates
	if dm.SuccessRate < 95 {
		score -= (95 - dm.SuccessRate) * 2
	}
	
	// Penalize high bot detection rate
	if dm.TotalRequests > 0 {
		botDetectionRate := float64(dm.BotDetectionErrors) / float64(dm.TotalRequests) * 100
		score -= botDetectionRate * 3 // Bot detection is critical
	}
	
	// Penalize consecutive errors
	if dm.ConsecutiveErrors > 5 {
		score -= float64(dm.ConsecutiveErrors-5) * 2
	}
	
	// Penalize high latency (> 10 seconds average)
	if dm.AvgLatencyMs > 10000 {
		score -= (dm.AvgLatencyMs - 10000) / 200
	}
	
	if score < 0 {
		score = 0
	}
	return score
}

// IsBotDetectionSuspected returns true if domain shows signs of bot detection
func (dm *DomainMetrics) IsBotDetectionSuspected() bool {
	if dm.TotalRequests < 5 {
		return false // Too few requests to determine
	}
	
	// High bot detection error rate
	botDetectionRate := float64(dm.BotDetectionErrors) / float64(dm.TotalRequests)
	if botDetectionRate > 0.5 { // More than 50% bot detection errors
		return true
	}
	
	// Many consecutive errors
	if dm.ConsecutiveErrors >= 10 {
		return true
	}
	
	return false
}

// DomainMetricsSummary provides aggregated domain statistics
type DomainMetricsSummary struct {
	TotalDomains           int                        `json:"total_domains"`
	HealthyDomains         int                        `json:"healthy_domains"`
	ProblematicDomains     int                        `json:"problematic_domains"`
	BotDetectionDomains    int                        `json:"bot_detection_domains"`
	TopErrorDomains        []*DomainMetrics           `json:"top_error_domains"`
	DomainBreakdown        map[string]*DomainMetrics  `json:"domain_breakdown"`
	OverallDomainHealthScore float64                  `json:"overall_domain_health_score"`
}

// ProxyMetricsSummary contains a snapshot of proxy metrics
type ProxyMetricsSummary struct {
	TotalRequests       uint64    `json:"total_requests"`
	EnvoyRequests       uint64    `json:"envoy_requests"`
	DirectRequests      uint64    `json:"direct_requests"`
	EnvoySuccessful     uint64    `json:"envoy_successful"`
	DirectSuccessful    uint64    `json:"direct_successful"`
	EnvoyFailures       uint64    `json:"envoy_failures"`
	DirectFailures      uint64    `json:"direct_failures"`
	EnvoyAvgLatencyMs   float64   `json:"envoy_avg_latency_ms"`
	DirectAvgLatencyMs  float64   `json:"direct_avg_latency_ms"`
	DNSAvgLatencyMs     float64   `json:"dns_avg_latency_ms"`
	EnvoySuccessRate    float64   `json:"envoy_success_rate_percent"`
	DirectSuccessRate   float64   `json:"direct_success_rate_percent"`
	EnvoyMovingAvgMs    float64   `json:"envoy_moving_avg_ms"`
	DirectMovingAvgMs   float64   `json:"direct_moving_avg_ms"`
	ConfigErrors        uint64    `json:"config_errors"`
	TimeoutErrors       uint64    `json:"timeout_errors"`
	ConnectionErrors    uint64    `json:"connection_errors"`
	DNSErrors          uint64    `json:"dns_errors"`
	ConfigSwitchCount   uint64    `json:"config_switch_count"`
	UptimeSeconds       float64   `json:"uptime_seconds"`
	CollectionStartTime time.Time `json:"collection_start_time"`
}

// GetHealthScore calculates an overall health score for the proxy system
func (s *ProxyMetricsSummary) GetHealthScore() float64 {
	if s.TotalRequests == 0 {
		return 100.0 // No requests yet, assume healthy
	}

	score := 100.0

	// Penalize low success rates
	if s.EnvoyRequests > 0 && s.EnvoySuccessRate < 95 {
		score -= (95 - s.EnvoySuccessRate) * 2
	}
	if s.DirectRequests > 0 && s.DirectSuccessRate < 95 {
		score -= (95 - s.DirectSuccessRate) * 2
	}

	// Penalize high error rates
	totalErrors := s.ConfigErrors + s.TimeoutErrors + s.ConnectionErrors + s.DNSErrors
	if totalErrors > 0 && s.TotalRequests > 0 {
		errorRate := float64(totalErrors) / float64(s.TotalRequests) * 100
		score -= errorRate * 3
	}

	// Penalize excessive latency (> 5 seconds average)
	if s.EnvoyAvgLatencyMs > 5000 {
		score -= (s.EnvoyAvgLatencyMs - 5000) / 100
	}
	if s.DirectAvgLatencyMs > 5000 {
		score -= (s.DirectAvgLatencyMs - 5000) / 100
	}

	// Ensure score stays within bounds
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score
}

// Global metrics instance for singleton access
var (
	globalProxyMetrics *ProxyMetrics
	metricsOnce        sync.Once
)

// RecordDomainRequest records a domain-specific request with comprehensive tracking
func (m *ProxyMetrics) RecordDomainRequest(requestURL string, duration time.Duration, success bool, errorType ProxyErrorType) {
	domain := m.extractDomain(requestURL)
	if domain == "" {
		return // Skip if domain cannot be extracted
	}

	m.domainMutex.Lock()
	defer m.domainMutex.Unlock()

	domainMetrics, exists := m.domainErrors[domain]
	if !exists {
		domainMetrics = &DomainMetrics{
			Domain: domain,
		}
		m.domainErrors[domain] = domainMetrics
	}

	// Update request counters
	atomic.AddUint64(&domainMetrics.TotalRequests, 1)
	atomic.AddUint64(&domainMetrics.LatencySum, uint64(duration.Milliseconds()))
	atomic.StoreInt64(&domainMetrics.LastRequestTime, time.Now().Unix())

	if success {
		atomic.AddUint64(&domainMetrics.SuccessfulRequests, 1)
		atomic.StoreUint64(&domainMetrics.ConsecutiveErrors, 0) // Reset consecutive errors on success
	} else {
		atomic.AddUint64(&domainMetrics.FailedRequests, 1)
		atomic.AddUint64(&domainMetrics.ConsecutiveErrors, 1)

		// Set first error time if this is the first error
		if atomic.LoadInt64(&domainMetrics.FirstErrorTime) == 0 {
			atomic.StoreInt64(&domainMetrics.FirstErrorTime, time.Now().Unix())
		}

		// Categorize error types
		switch errorType {
		case ProxyErrorConfig:
			atomic.AddUint64(&domainMetrics.ConfigErrors, 1)
		case ProxyErrorTimeout:
			atomic.AddUint64(&domainMetrics.TimeoutErrors, 1)
		case ProxyErrorConnection:
			atomic.AddUint64(&domainMetrics.ConnectionErrors, 1)
		case ProxyErrorDNS:
			atomic.AddUint64(&domainMetrics.DNSErrors, 1)
		}

		// Special handling for bot detection (HTTP 403/blocked responses)
		if m.isBotDetectionError(errorType, requestURL) {
			atomic.AddUint64(&domainMetrics.BotDetectionErrors, 1)
		}
	}

	// Update calculated metrics
	m.updateDomainCalculatedMetrics(domainMetrics)

	// Log domain-specific metrics periodically
	if domainMetrics.TotalRequests%25 == 0 {
		m.logDomainMetrics(domainMetrics)
	}
}

// extractDomain extracts domain from URL string
func (m *ProxyMetrics) extractDomain(requestURL string) string {
	if requestURL == "" {
		return ""
	}

	parsedURL, err := url.Parse(requestURL)
	if err != nil {
		m.logger.Warn("failed to parse URL for domain extraction", "url", requestURL, "error", err)
		return ""
	}

	return parsedURL.Hostname()
}

// isBotDetectionError determines if an error is likely due to bot detection
func (m *ProxyMetrics) isBotDetectionError(errorType ProxyErrorType, requestURL string) bool {
	// Bot detection typically manifests as:
	// 1. Connection errors (blocked at firewall level)
	// 2. Consistent failures from same domain
	return errorType == ProxyErrorConnection || errorType == ProxyErrorTimeout
}

// updateDomainCalculatedMetrics updates calculated fields for domain metrics
func (m *ProxyMetrics) updateDomainCalculatedMetrics(dm *DomainMetrics) {
	totalRequests := atomic.LoadUint64(&dm.TotalRequests)
	if totalRequests > 0 {
		successfulRequests := atomic.LoadUint64(&dm.SuccessfulRequests)
		dm.SuccessRate = float64(successfulRequests) / float64(totalRequests) * 100

		latencySum := atomic.LoadUint64(&dm.LatencySum)
		dm.AvgLatencyMs = float64(latencySum) / float64(totalRequests)
	}
}

// logDomainMetrics logs comprehensive domain-specific metrics
func (m *ProxyMetrics) logDomainMetrics(dm *DomainMetrics) {
	m.logger.Info("domain-specific metrics",
		"domain", dm.Domain,
		"total_requests", dm.TotalRequests,
		"success_rate_percent", dm.SuccessRate,
		"avg_latency_ms", dm.AvgLatencyMs,
		"bot_detection_errors", dm.BotDetectionErrors,
		"consecutive_errors", dm.ConsecutiveErrors,
		"timeout_errors", dm.TimeoutErrors,
		"connection_errors", dm.ConnectionErrors,
		"dns_errors", dm.DNSErrors,
		"health_score", dm.GetHealthScore(),
		"bot_detection_suspected", dm.IsBotDetectionSuspected())
}

// GetDomainMetricsSummary returns comprehensive domain-specific metrics summary
func (m *ProxyMetrics) GetDomainMetricsSummary() *DomainMetricsSummary {
	m.domainMutex.RLock()
	defer m.domainMutex.RUnlock()

	summary := &DomainMetricsSummary{
		DomainBreakdown:      make(map[string]*DomainMetrics),
		TopErrorDomains:      make([]*DomainMetrics, 0),
	}

	var totalHealthScore float64
	var healthyDomains, problematicDomains, botDetectionDomains int

	// Process each domain
	for domain, metrics := range m.domainErrors {
		// Create a copy of metrics with updated calculated fields
		domainCopy := *metrics
		m.updateDomainCalculatedMetrics(&domainCopy)
		
		summary.DomainBreakdown[domain] = &domainCopy
		summary.TotalDomains++

		healthScore := domainCopy.GetHealthScore()
		totalHealthScore += healthScore

		if healthScore >= 80 {
			healthyDomains++
		} else {
			problematicDomains++
			// Add to top error domains for further analysis
			summary.TopErrorDomains = append(summary.TopErrorDomains, &domainCopy)
		}

		if domainCopy.IsBotDetectionSuspected() {
			botDetectionDomains++
		}
	}

	summary.HealthyDomains = healthyDomains
	summary.ProblematicDomains = problematicDomains
	summary.BotDetectionDomains = botDetectionDomains

	if summary.TotalDomains > 0 {
		summary.OverallDomainHealthScore = totalHealthScore / float64(summary.TotalDomains)
	} else {
		summary.OverallDomainHealthScore = 100.0
	}

	// Sort top error domains by error count (most problematic first)
	if len(summary.TopErrorDomains) > 1 {
		for i := 0; i < len(summary.TopErrorDomains)-1; i++ {
			for j := i + 1; j < len(summary.TopErrorDomains); j++ {
				if summary.TopErrorDomains[i].FailedRequests < summary.TopErrorDomains[j].FailedRequests {
					summary.TopErrorDomains[i], summary.TopErrorDomains[j] = summary.TopErrorDomains[j], summary.TopErrorDomains[i]
				}
			}
		}
	}

	// Limit to top 10 most problematic domains
	if len(summary.TopErrorDomains) > 10 {
		summary.TopErrorDomains = summary.TopErrorDomains[:10]
	}

	return summary
}

// GetGlobalProxyMetrics returns the singleton proxy metrics instance
func GetGlobalProxyMetrics(logger *slog.Logger) *ProxyMetrics {
	metricsOnce.Do(func() {
		globalProxyMetrics = NewProxyMetrics(logger)
	})
	return globalProxyMetrics
}