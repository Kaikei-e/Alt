// Package middleware provides HTTP middleware components for the Alt backend.
// It includes authentication, rate limiting, DoS protection, and other
// cross-cutting concerns for the Echo web framework.
package middleware

import (
	"alt/config"
	"alt/utils/logger"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/time/rate"
)

// DOSProtectionConfig defines configuration for DoS protection middleware
type DOSProtectionConfig struct {
	Enabled          bool                 `json:"enabled"`
	RateLimit        int                  `json:"rate_limit"`        // Requests per window
	BurstLimit       int                  `json:"burst_limit"`       // Burst capacity
	WindowSize       time.Duration        `json:"window_size"`       // Rate limit window
	BlockDuration    time.Duration        `json:"block_duration"`    // How long to block after rate limit
	WhitelistedPaths []string             `json:"whitelisted_paths"` // Paths to skip rate limiting
	CircuitBreaker   CircuitBreakerConfig `json:"circuit_breaker"`   // Circuit breaker configuration
}

// CircuitBreakerConfig defines circuit breaker configuration
type CircuitBreakerConfig struct {
	Enabled          bool          `json:"enabled"`
	FailureThreshold int           `json:"failure_threshold"` // Number of failures before opening circuit
	TimeoutDuration  time.Duration `json:"timeout_duration"`  // Request timeout
	RecoveryTimeout  time.Duration `json:"recovery_timeout"`  // Time before trying to recover
}

// Validate validates the DOSProtectionConfig
func (c *DOSProtectionConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.RateLimit <= 0 {
		return fmt.Errorf("rate limit must be greater than 0")
	}

	if c.BurstLimit <= 0 {
		return fmt.Errorf("burst limit must be greater than 0")
	}

	if c.BurstLimit < c.RateLimit {
		return fmt.Errorf("burst limit must be >= rate limit")
	}

	if c.WindowSize <= 0 {
		return fmt.Errorf("window size must be greater than 0")
	}

	if c.BlockDuration <= 0 {
		return fmt.Errorf("block duration must be greater than 0")
	}

	return nil
}

// rateLimiter holds rate limiter and associated metadata
type rateLimiter struct {
	limiter   *rate.Limiter
	blockedAt time.Time
	mu        sync.Mutex // Protects blockedAt field
}

// circuitBreaker implements circuit breaker pattern.
// The mutex is a plain sync.Mutex (not RWMutex) to avoid the lock-upgrade
// race that the previous implementation had: holding an RLock, releasing it
// to acquire a Lock, and writing the state was a TOCTOU hazard because
// another goroutine could have changed the state in between. Reads of state
// are short, so the RWMutex optimisation was not worth the complexity.
type circuitBreaker struct {
	config          CircuitBreakerConfig
	failures        int
	lastFailureTime time.Time
	state           circuitState
	mu              sync.Mutex
}

type circuitState int

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

// DOSProtectionMiddleware returns a middleware that provides DoS protection.
// Backward-compatible default: trusts X-Real-IP / X-Forwarded-For. New call
// sites should prefer DOSProtectionMiddlewareWithTrust(config, false) when
// not behind a controlled reverse proxy (M-007).
func DOSProtectionMiddleware(config DOSProtectionConfig) echo.MiddlewareFunc {
	return DOSProtectionMiddlewareWithTrust(config, true)
}

// DOSProtectionMiddlewareWithTrust is the trust-aware variant. Use this from
// route registration when the deployment terminates TLS / sanitises XFF on a
// trusted hop.
func DOSProtectionMiddlewareWithTrust(config DOSProtectionConfig, trustForwardedHeaders bool) echo.MiddlewareFunc {
	if err := config.Validate(); err != nil {
		panic(fmt.Sprintf("invalid DoS protection config: %v", err))
	}

	if !config.Enabled {
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return next
		}
	}

	limiters := make(map[string]*rateLimiter)
	limiterMu := sync.RWMutex{}

	// M-006: evict stale rate-limiter entries periodically. Without this the
	// map grows without bound under attack from many spoofed source addresses.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			CleanupExpiredLimiters(limiters, &limiterMu, 30*time.Minute)
		}
	}()

	var cb *circuitBreaker
	if config.CircuitBreaker.Enabled {
		cb = &circuitBreaker{
			config: config.CircuitBreaker,
			state:  circuitClosed,
		}
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()

			// Skip whitelisted paths
			path := c.Request().URL.Path
			if isWhitelistedPath(path, config.WhitelistedPaths) {
				// Log whitelist hits for streaming endpoints for debugging
				if path == "/v1/feeds/summarize/stream" {
					logger.Logger.DebugContext(ctx, "DoS protection: streaming endpoint whitelisted", "path", path)
				}
				return next(c)
			}

			// Check circuit breaker first
			if cb != nil && cb.shouldBlock() {
				return echo.NewHTTPError(http.StatusServiceUnavailable, "Service temporarily unavailable")
			}

			// Get client IP
			clientIP := getClientIPWithTrust(c, trustForwardedHeaders)
			if clientIP == "" {
				clientIP = "unknown"
			}

			// Check rate limiting
			if !checkRateLimit(clientIP, config, limiters, &limiterMu) {
				return echo.NewHTTPError(http.StatusTooManyRequests, "Too many requests")
			}

			// Execute the request
			err := next(c)

			// Update circuit breaker - only count server errors (5xx) as failures
			// Client errors (4xx) are not server-side issues and should not trip the circuit
			if cb != nil {
				if err != nil {
					// Check if it's a server error (5xx)
					if he, ok := err.(*echo.HTTPError); ok && he.Code >= 500 {
						cb.recordFailure()
					} else {
						// Client errors (4xx) should not affect circuit breaker
						cb.recordSuccess()
					}
				} else {
					cb.recordSuccess()
				}
			}

			return err
		}
	}
}

// getClientIP extracts client IP from forwarded headers, treating them as
// trusted. Kept for backward compatibility with existing tests; new callers
// should use getClientIPWithTrust to make the trust decision explicit.
func getClientIP(c echo.Context) string {
	return getClientIPWithTrust(c, true)
}

// getClientIPWithTrust returns the client IP. When trustForwardedHeaders is
// true, X-Real-IP and the leftmost X-Forwarded-For entry are honoured (use
// only when behind a controlled reverse proxy that overwrites those headers).
// When false, headers are ignored and only the connection peer is returned —
// this prevents IP spoofing for rate-limit bypass when alt-backend is reached
// directly (M-007).
func getClientIPWithTrust(c echo.Context, trustForwardedHeaders bool) string {
	if trustForwardedHeaders {
		if ip := c.Request().Header.Get("X-Real-IP"); ip != "" {
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
		if xff := c.Request().Header.Get("X-Forwarded-For"); xff != "" {
			for _, raw := range strings.Split(xff, ",") {
				ip := strings.TrimSpace(raw)
				if net.ParseIP(ip) != nil {
					return ip
				}
			}
		}
	}

	if ip, _, err := net.SplitHostPort(c.Request().RemoteAddr); err == nil {
		if net.ParseIP(ip) != nil {
			return ip
		}
	}
	return ""
}

// isWhitelistedPath checks if the path is whitelisted (M-005).
// An entry ending with `/` is treated as a prefix; everything else must match
// exactly. Substring matching is not allowed because it lets future routes
// accidentally bypass DoS protection (e.g. `/v1/feeds/livestream-config` does
// not slip past `/stream` matching).
func isWhitelistedPath(path string, whitelistedPaths []string) bool {
	for _, whitelistedPath := range whitelistedPaths {
		if strings.HasSuffix(whitelistedPath, "/") {
			if strings.HasPrefix(path, whitelistedPath) {
				return true
			}
			continue
		}
		if path == whitelistedPath {
			return true
		}
	}
	return false
}

// checkRateLimit checks if the request should be rate limited
func checkRateLimit(clientIP string, config DOSProtectionConfig, limiters map[string]*rateLimiter, mu *sync.RWMutex) bool {
	mu.RLock()
	limiter, exists := limiters[clientIP]
	mu.RUnlock()

	if !exists {
		// Create new rate limiter for this IP
		mu.Lock()
		// Double-check pattern
		if limiter, exists = limiters[clientIP]; !exists {
			// Calculate rate as requests per second based on RateLimit and WindowSize
			// For example: 5 requests per minute = 5/60 = 0.083 requests per second
			ratePerSecond := rate.Limit(float64(config.RateLimit) / config.WindowSize.Seconds())
			limiter = &rateLimiter{
				limiter: rate.NewLimiter(ratePerSecond, config.BurstLimit),
			}
			limiters[clientIP] = limiter
		}
		mu.Unlock()
	}

	// Check if IP is currently blocked (with proper synchronization)
	limiter.mu.Lock()
	if !limiter.blockedAt.IsZero() {
		if time.Since(limiter.blockedAt) < config.BlockDuration {
			limiter.mu.Unlock()
			return false
		}
		// Unblock the IP
		limiter.blockedAt = time.Time{}
	}
	limiter.mu.Unlock()

	// Check rate limit
	if !limiter.limiter.Allow() {
		// Block the IP (with proper synchronization)
		limiter.mu.Lock()
		limiter.blockedAt = time.Now()
		limiter.mu.Unlock()
		return false
	}

	return true
}

// Circuit breaker methods.
// shouldBlock performs both the decision and any state transition
// (Open -> HalfOpen after the recovery timeout) within a single critical
// section. This avoids the previous RLock-then-Lock upgrade pattern whose
// gap allowed concurrent goroutines to overwrite a state change made by
// a peer (e.g. HalfOpen -> Closed from recordSuccess).
func (cb *circuitBreaker) shouldBlock() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case circuitClosed:
		return false
	case circuitOpen:
		if time.Since(cb.lastFailureTime) > cb.config.RecoveryTimeout {
			cb.state = circuitHalfOpen
			return false
		}
		return true
	case circuitHalfOpen:
		return false
	default:
		return false
	}
}

func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	if cb.failures >= cb.config.FailureThreshold {
		cb.state = circuitOpen
	}
}

func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == circuitHalfOpen {
		cb.state = circuitClosed
		cb.failures = 0
	} else if cb.state == circuitClosed {
		cb.failures = 0
	}
}

// DefaultDOSProtectionConfig returns a default configuration
func DefaultDOSProtectionConfig() DOSProtectionConfig {
	return DOSProtectionConfig{
		Enabled:       true,
		RateLimit:     100,
		BurstLimit:    200,
		WindowSize:    time.Minute,
		BlockDuration: 5 * time.Minute,
		WhitelistedPaths: []string{
			"/v1/health",
			"/v1/metrics",
		},
		CircuitBreaker: CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 10,
			TimeoutDuration:  30 * time.Second,
			RecoveryTimeout:  60 * time.Second,
		},
	}
}

// CleanupExpiredLimiters removes expired rate limiters to prevent memory leaks
func CleanupExpiredLimiters(limiters map[string]*rateLimiter, mu *sync.RWMutex, maxAge time.Duration) {
	mu.Lock()
	defer mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for ip, limiter := range limiters {
		limiter.mu.Lock()
		shouldDelete := !limiter.blockedAt.IsZero() && limiter.blockedAt.Before(cutoff)
		limiter.mu.Unlock()

		if shouldDelete {
			delete(limiters, ip)
		}
	}
}

// ConvertConfigDOSProtection converts config package DOSProtectionConfig to middleware package DOSProtectionConfig
func ConvertConfigDOSProtection(configDOS config.DOSProtectionConfig) DOSProtectionConfig {
	return DOSProtectionConfig{
		Enabled:          configDOS.Enabled,
		RateLimit:        configDOS.RateLimit,
		BurstLimit:       configDOS.BurstLimit,
		WindowSize:       configDOS.WindowSize,
		BlockDuration:    configDOS.BlockDuration,
		WhitelistedPaths: configDOS.WhitelistedPaths,
		CircuitBreaker: CircuitBreakerConfig{
			Enabled:          configDOS.CircuitBreaker.Enabled,
			FailureThreshold: configDOS.CircuitBreaker.FailureThreshold,
			TimeoutDuration:  configDOS.CircuitBreaker.TimeoutDuration,
			RecoveryTimeout:  configDOS.CircuitBreaker.RecoveryTimeout,
		},
	}
}
