// Package rate_limiter provides host-based rate limiting for external API calls.
// It ensures compliance with rate limits by throttling requests to each unique host.
package rate_limiter

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// HostRateLimiter implements per-host rate limiting using token bucket algorithm.
// It maintains separate rate limiters for each unique host to prevent exceeding
// external API rate limits.
type HostRateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	interval time.Duration
	burst    int
}

// NewHostRateLimiter creates a new HostRateLimiter with the specified interval
// between requests to the same host. The interval should be at least 5 seconds
// for external API compliance. An optional burst parameter controls how many
// requests can be made immediately before rate limiting kicks in (default: 1).
func NewHostRateLimiter(interval time.Duration, burst ...int) *HostRateLimiter {
	b := 1
	if len(burst) > 0 && burst[0] > 0 {
		b = burst[0]
	}
	return &HostRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		interval: interval,
		burst:    b,
	}
}

// WaitForHost blocks until the rate limiter allows a request to the host
// extracted from the URL. It returns an error if the URL is invalid or
// if the context is cancelled.
func (h *HostRateLimiter) WaitForHost(ctx context.Context, urlStr string) error {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return err
	}

	host := parsedURL.Host
	if host == "" {
		return &url.Error{Op: "parse", URL: urlStr, Err: errors.New("missing host in URL")}
	}

	limiter := h.getLimiterForHost(host)

	return limiter.Wait(ctx)
}

func (h *HostRateLimiter) getLimiterForHost(host string) *rate.Limiter {
	h.mu.RLock()
	limiter, exists := h.limiters[host]
	h.mu.RUnlock()

	if exists {
		return limiter
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Double-check pattern
	if limiter, exists := h.limiters[host]; exists {
		return limiter
	}

	limiter = rate.NewLimiter(rate.Every(h.interval), h.burst)
	h.limiters[host] = limiter
	return limiter
}

// RecordRateLimitHit increases backoff for a host after a 429 response.
// It respects the Retry-After duration if provided, otherwise doubles the current interval.
// The backoff is capped at 1 hour maximum.
func (h *HostRateLimiter) RecordRateLimitHit(host string, retryAfter time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Use Retry-After header if provided, otherwise double current interval
	backoff := retryAfter
	if backoff == 0 {
		backoff = h.interval * 2
	}

	// Cap at 1 hour max
	if backoff > time.Hour {
		backoff = time.Hour
	}

	h.limiters[host] = rate.NewLimiter(rate.Every(backoff), h.burst)
}
