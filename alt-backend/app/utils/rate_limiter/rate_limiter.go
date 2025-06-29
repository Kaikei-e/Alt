package rate_limiter

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type HostRateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	interval time.Duration
}

func NewHostRateLimiter(interval time.Duration) *HostRateLimiter {
	return &HostRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		interval: interval,
	}
}

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

	limiter = rate.NewLimiter(rate.Every(h.interval), 1)
	h.limiters[host] = limiter
	return limiter
}
