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
	// currentIntervals tracks each host's post-backoff interval so repeated
	// 429s double from the host's own current interval (true exponential
	// backoff) instead of always doubling the configured base interval.
	// A host absent from this map is still at the base interval.
	currentIntervals map[string]time.Duration
	mu               sync.RWMutex
	interval         time.Duration
	burst            int
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
		limiters:         make(map[string]*rate.Limiter),
		currentIntervals: make(map[string]time.Duration),
		interval:         interval,
		burst:            b,
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
// It respects the Retry-After duration if provided, otherwise doubles the
// host's current interval (the base interval on the first hit, then whatever
// the previous backoff left it at). The backoff is capped at 1 hour maximum.
// Call RecordSuccess once the host responds normally again to decay the
// backoff back to the base interval.
func (h *HostRateLimiter) RecordRateLimitHit(host string, retryAfter time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Use Retry-After header if provided, otherwise double the host's
	// current interval (falls back to the base interval if not yet backed off).
	backoff := retryAfter
	if backoff == 0 {
		current, backedOff := h.currentIntervals[host]
		if !backedOff {
			current = h.interval
		}
		backoff = current * 2
	}

	// Cap at 1 hour max
	if backoff > time.Hour {
		backoff = time.Hour
	}

	h.currentIntervals[host] = backoff
	h.limiters[host] = rate.NewLimiter(rate.Every(backoff), h.burst)
}

// RecordSuccess decays a host's rate limiter back to the configured base
// interval. Call this after a successful (non-429) response so a host that
// was previously backed off recovers instead of staying throttled forever.
// It is a no-op if the host was never backed off.
func (h *HostRateLimiter) RecordSuccess(host string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, backedOff := h.currentIntervals[host]; !backedOff {
		return
	}

	delete(h.currentIntervals, host)
	h.limiters[host] = rate.NewLimiter(rate.Every(h.interval), h.burst)
}
