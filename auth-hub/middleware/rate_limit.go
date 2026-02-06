package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/time/rate"
)

// RateLimitConfig defines per-endpoint rate limit settings.
type RateLimitConfig struct {
	// Rate is the number of requests allowed per second.
	Rate rate.Limit
	// Burst is the maximum burst size.
	Burst int
}

// ipLimiter holds a rate limiter and the last time it was seen.
type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter provides IP-based rate limiting per endpoint group.
type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*ipLimiter
	rate     rate.Limit
	burst    int
}

// NewRateLimiter creates a new per-IP rate limiter.
func NewRateLimiter(r rate.Limit, burst int) *RateLimiter {
	rl := &RateLimiter{
		limiters: make(map[string]*ipLimiter),
		rate:     r,
		burst:    burst,
	}
	go rl.cleanupLoop()
	return rl
}

// getLimiter returns the rate limiter for the given IP, creating one if needed.
func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if l, exists := rl.limiters[ip]; exists {
		l.lastSeen = time.Now()
		return l.limiter
	}

	limiter := rate.NewLimiter(rl.rate, rl.burst)
	rl.limiters[ip] = &ipLimiter{limiter: limiter, lastSeen: time.Now()}
	return limiter
}

// cleanupLoop removes stale entries every 3 minutes.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		for ip, l := range rl.limiters {
			if time.Since(l.lastSeen) > 5*time.Minute {
				delete(rl.limiters, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// Middleware returns an Echo middleware that enforces the rate limit.
func (rl *RateLimiter) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ip := c.RealIP()
			limiter := rl.getLimiter(ip)

			if !limiter.Allow() {
				retryAfter := max(int(1.0/float64(rl.rate)), 1)
				c.Response().Header().Set("Retry-After", strconv.Itoa(retryAfter))
				return echo.NewHTTPError(http.StatusTooManyRequests, "rate limit exceeded")
			}

			return next(c)
		}
	}
}
