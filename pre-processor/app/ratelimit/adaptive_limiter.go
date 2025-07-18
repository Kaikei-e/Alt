// ABOUTME: This file implements adaptive rate limiting with domain-specific controls and performance tracking
// ABOUTME: Provides intelligent throttling based on success/failure rates and configurable domain limits
package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"pre-processor/config"
)

// DomainMetrics tracks performance metrics for a specific domain
type DomainMetrics struct {
	TotalRequests   int64         `json:"total_requests"`
	SuccessCount    int64         `json:"success_count"`
	FailureCount    int64         `json:"failure_count"`
	SuccessRate     float64       `json:"success_rate"`
	AvgResponseTime time.Duration `json:"avg_response_time"`
	LastRequestTime time.Time     `json:"last_request_time"`
	CurrentInterval time.Duration `json:"current_interval"`
}

// domainLimiter manages rate limiting for a single domain
type domainLimiter struct {
	lastRequest    time.Time
	interval       time.Duration
	baseInterval   time.Duration
	burstTokens    int
	maxBurstTokens int
	mu             sync.Mutex

	// Adaptive metrics
	totalRequests     int64
	successCount      int64
	failureCount      int64
	totalResponseTime time.Duration
	lastRequestTime   time.Time
}

// AdaptiveLimiter provides intelligent rate limiting with domain-specific controls
type AdaptiveLimiter struct {
	defaultInterval time.Duration
	domainIntervals map[string]time.Duration
	burstSize       int
	enableAdaptive  bool
	domainLimiters  map[string]*domainLimiter
	metrics         map[string]*DomainMetrics
	mu              sync.RWMutex
	logger          *slog.Logger

	// Adaptive parameters
	adaptationFactor float64
	minInterval      time.Duration
	maxInterval      time.Duration
	successThreshold float64
	failureThreshold float64
	adaptationWindow int64
}

// NewAdaptiveLimiter creates a new adaptive rate limiter
func NewAdaptiveLimiter(cfg config.RateLimitConfig, logger *slog.Logger) (*AdaptiveLimiter, error) {
	if cfg.DefaultInterval <= 0 {
		return nil, errors.New("default interval must be positive")
	}

	if cfg.BurstSize <= 0 {
		return nil, errors.New("burst size must be positive")
	}

	limiter := &AdaptiveLimiter{
		defaultInterval: cfg.DefaultInterval,
		domainIntervals: cfg.DomainIntervals,
		burstSize:       cfg.BurstSize,
		enableAdaptive:  cfg.EnableAdaptive,
		domainLimiters:  make(map[string]*domainLimiter),
		metrics:         make(map[string]*DomainMetrics),
		logger:          logger,

		// Adaptive parameters
		adaptationFactor: 0.1,                      // 10% adjustment per adaptation
		minInterval:      cfg.DefaultInterval / 4,  // Minimum 25% of default
		maxInterval:      cfg.DefaultInterval * 10, // Maximum 10x default
		successThreshold: 0.9,                      // 90% success rate to reduce interval
		failureThreshold: 0.7,                      // Below 70% success rate to increase interval
		adaptationWindow: 10,                       // Adapt after every 10 requests
	}

	if limiter.domainIntervals == nil {
		limiter.domainIntervals = make(map[string]time.Duration)
	}

	logger.Info("adaptive rate limiter initialized",
		"default_interval", cfg.DefaultInterval,
		"burst_size", cfg.BurstSize,
		"adaptive_enabled", cfg.EnableAdaptive,
		"domain_intervals", len(cfg.DomainIntervals))

	return limiter, nil
}

// Wait blocks until the request can proceed for the given domain
func (al *AdaptiveLimiter) Wait(domain string) {
	if err := al.WaitWithContext(context.Background(), domain); err != nil {
		al.logger.Error("rate limit wait failed", "domain", domain, "error", err)
	}
}

// WaitWithContext blocks until the request can proceed or context is cancelled
func (al *AdaptiveLimiter) WaitWithContext(ctx context.Context, domain string) error {
	limiter := al.getLimiter(domain)

	limiter.mu.Lock()

	now := time.Now()
	elapsed := now.Sub(limiter.lastRequest)

	// Check if we have burst tokens available
	if elapsed >= limiter.interval {
		// Refill burst tokens based on time elapsed
		tokensToAdd := int(elapsed / limiter.interval)
		limiter.burstTokens += tokensToAdd
		if limiter.burstTokens > limiter.maxBurstTokens {
			limiter.burstTokens = limiter.maxBurstTokens
		}
	}

	// If we have tokens, consume one and proceed
	if limiter.burstTokens > 0 {
		limiter.burstTokens--
		limiter.lastRequest = now
		limiter.mu.Unlock()
		return nil
	}

	// Calculate wait time
	waitTime := limiter.interval - elapsed
	if waitTime <= 0 {
		limiter.lastRequest = now
		limiter.mu.Unlock()
		return nil
	}

	// Unlock before waiting
	limiter.mu.Unlock()

	// Wait with context cancellation support
	timer := time.NewTimer(waitTime)
	defer timer.Stop()

	select {
	case <-timer.C:
		limiter.mu.Lock()
		limiter.lastRequest = time.Now()
		limiter.mu.Unlock()
		return nil
	case <-ctx.Done():
		return fmt.Errorf("rate limit wait cancelled: %w", ctx.Err())
	}
}

// RecordSuccess records a successful request for adaptive behavior
func (al *AdaptiveLimiter) RecordSuccess(domain string, responseTime time.Duration) {
	if !al.enableAdaptive {
		return
	}

	limiter := al.getLimiter(domain)

	al.mu.Lock()
	defer al.mu.Unlock()

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	limiter.totalRequests++
	limiter.successCount++
	limiter.totalResponseTime += responseTime
	limiter.lastRequestTime = time.Now()

	// Update metrics
	al.updateMetrics(domain, limiter)

	// Adapt interval if we have enough data
	if limiter.totalRequests%al.adaptationWindow == 0 {
		al.adaptInterval(domain, limiter)
	}

	al.logger.Debug("recorded success",
		"domain", domain,
		"response_time", responseTime,
		"total_requests", limiter.totalRequests,
		"success_rate", float64(limiter.successCount)/float64(limiter.totalRequests))
}

// RecordFailure records a failed request for adaptive behavior
func (al *AdaptiveLimiter) RecordFailure(domain string, responseTime time.Duration) {
	if !al.enableAdaptive {
		return
	}

	limiter := al.getLimiter(domain)

	al.mu.Lock()
	defer al.mu.Unlock()

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	limiter.totalRequests++
	limiter.failureCount++
	limiter.totalResponseTime += responseTime
	limiter.lastRequestTime = time.Now()

	// Update metrics
	al.updateMetrics(domain, limiter)

	// Adapt interval if we have enough data
	if limiter.totalRequests%al.adaptationWindow == 0 {
		al.adaptInterval(domain, limiter)
	}

	al.logger.Debug("recorded failure",
		"domain", domain,
		"response_time", responseTime,
		"total_requests", limiter.totalRequests,
		"success_rate", float64(limiter.successCount)/float64(limiter.totalRequests))
}

// GetMetrics returns current metrics for a domain
func (al *AdaptiveLimiter) GetMetrics(domain string) *DomainMetrics {
	al.mu.RLock()
	defer al.mu.RUnlock()

	metrics, exists := al.metrics[domain]
	if !exists {
		return nil
	}

	// Return a copy to avoid race conditions
	copy := *metrics
	return &copy
}

// Cleanup removes old domain limiters to prevent memory leaks
func (al *AdaptiveLimiter) Cleanup() {
	al.mu.Lock()
	defer al.mu.Unlock()

	now := time.Now()
	cleanupThreshold := 24 * time.Hour // Remove domains unused for 24 hours

	for domain, limiter := range al.domainLimiters {
		limiter.mu.Lock()
		lastUsed := limiter.lastRequestTime
		limiter.mu.Unlock()

		if now.Sub(lastUsed) > cleanupThreshold {
			delete(al.domainLimiters, domain)
			delete(al.metrics, domain)
			al.logger.Debug("cleaned up unused domain limiter", "domain", domain)
		}
	}
}

// getLimiter gets or creates a domain-specific limiter
func (al *AdaptiveLimiter) getLimiter(domain string) *domainLimiter {
	al.mu.RLock()
	limiter, exists := al.domainLimiters[domain]
	al.mu.RUnlock()

	if exists {
		return limiter
	}

	al.mu.Lock()
	defer al.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists := al.domainLimiters[domain]; exists {
		return limiter
	}

	// Determine interval for this domain
	interval := al.defaultInterval
	if domainInterval, exists := al.domainIntervals[domain]; exists {
		interval = domainInterval
	}

	limiter = &domainLimiter{
		lastRequest:     time.Time{},
		interval:        interval,
		baseInterval:    interval,
		burstTokens:     al.burstSize,
		maxBurstTokens:  al.burstSize,
		lastRequestTime: time.Now(),
	}

	al.domainLimiters[domain] = limiter
	al.metrics[domain] = &DomainMetrics{
		CurrentInterval: interval,
		LastRequestTime: time.Now(),
	}

	al.logger.Debug("created new domain limiter",
		"domain", domain,
		"interval", interval,
		"burst_size", al.burstSize)

	return limiter
}

// updateMetrics updates the metrics for a domain
func (al *AdaptiveLimiter) updateMetrics(domain string, limiter *domainLimiter) {
	metrics := al.metrics[domain]
	if metrics == nil {
		return
	}

	metrics.TotalRequests = limiter.totalRequests
	metrics.SuccessCount = limiter.successCount
	metrics.FailureCount = limiter.failureCount
	metrics.LastRequestTime = limiter.lastRequestTime
	metrics.CurrentInterval = limiter.interval

	if limiter.totalRequests > 0 {
		metrics.SuccessRate = float64(limiter.successCount) / float64(limiter.totalRequests)
		metrics.AvgResponseTime = time.Duration(limiter.totalResponseTime.Nanoseconds() / limiter.totalRequests)
	}
}

// adaptInterval adjusts the rate limiting interval based on performance
func (al *AdaptiveLimiter) adaptInterval(domain string, limiter *domainLimiter) {
	if limiter.totalRequests < al.adaptationWindow {
		return
	}

	successRate := float64(limiter.successCount) / float64(limiter.totalRequests)
	oldInterval := limiter.interval

	if successRate >= al.successThreshold {
		// High success rate: reduce interval (increase rate)
		adjustment := time.Duration(float64(limiter.interval) * al.adaptationFactor)
		limiter.interval -= adjustment
		if limiter.interval < al.minInterval {
			limiter.interval = al.minInterval
		}
	} else if successRate < al.failureThreshold {
		// Low success rate: increase interval (decrease rate)
		adjustment := time.Duration(float64(limiter.interval) * al.adaptationFactor)
		limiter.interval += adjustment
		if limiter.interval > al.maxInterval {
			limiter.interval = al.maxInterval
		}
	}

	if limiter.interval != oldInterval {
		al.logger.Info("adapted rate limit interval",
			"domain", domain,
			"old_interval", oldInterval,
			"new_interval", limiter.interval,
			"success_rate", successRate,
			"total_requests", limiter.totalRequests)
	}
}
