package middleware

import (
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
	Enabled          bool                  `json:"enabled"`
	RateLimit        int                   `json:"rate_limit"`        // Requests per window
	BurstLimit       int                   `json:"burst_limit"`       // Burst capacity
	WindowSize       time.Duration         `json:"window_size"`       // Rate limit window
	BlockDuration    time.Duration         `json:"block_duration"`    // How long to block after rate limit
	WhitelistedPaths []string              `json:"whitelisted_paths"` // Paths to skip rate limiting
	CircuitBreaker   CircuitBreakerConfig  `json:"circuit_breaker"`   // Circuit breaker configuration
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
		return fmt.Errorf("rate_limit must be greater than 0")
	}

	if c.BurstLimit <= 0 {
		return fmt.Errorf("burst_limit must be greater than 0")
	}

	if c.BurstLimit < c.RateLimit {
		return fmt.Errorf("burst_limit must be >= rate_limit")
	}

	if c.WindowSize <= 0 {
		return fmt.Errorf("window_size must be greater than 0")
	}

	if c.BlockDuration <= 0 {
		return fmt.Errorf("block_duration must be greater than 0")
	}

	return nil
}

// rateLimiter holds rate limiter and associated metadata
type rateLimiter struct {
	limiter   *rate.Limiter
	blockedAt time.Time
}

// circuitBreaker implements circuit breaker pattern
type circuitBreaker struct {
	config           CircuitBreakerConfig
	failures         int
	lastFailureTime  time.Time
	state            circuitState
	mu               sync.RWMutex
}

type circuitState int

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

// DOSProtectionMiddleware returns a middleware that provides DoS protection
func DOSProtectionMiddleware(config DOSProtectionConfig) echo.MiddlewareFunc {
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

	var cb *circuitBreaker
	if config.CircuitBreaker.Enabled {
		cb = &circuitBreaker{
			config: config.CircuitBreaker,
			state:  circuitClosed,
		}
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip whitelisted paths
			if isWhitelistedPath(c.Request().URL.Path, config.WhitelistedPaths) {
				return next(c)
			}

			// Check circuit breaker first
			if cb != nil && cb.shouldBlock() {
				return echo.NewHTTPError(http.StatusServiceUnavailable, "Service temporarily unavailable")
			}

			// Get client IP
			clientIP := getClientIP(c)
			if clientIP == "" {
				clientIP = "unknown"
			}

			// Check rate limiting
			if !checkRateLimit(clientIP, config, limiters, &limiterMu) {
				return echo.NewHTTPError(http.StatusTooManyRequests, "Too many requests")
			}

			// Execute the request
			err := next(c)

			// Update circuit breaker
			if cb != nil {
				if err != nil {
					cb.recordFailure()
				} else {
					cb.recordSuccess()
				}
			}

			return err
		}
	}
}

// getClientIP extracts client IP from various headers
func getClientIP(c echo.Context) string {
	// Check X-Real-IP header first
	if ip := c.Request().Header.Get("X-Real-IP"); ip != "" {
		if net.ParseIP(ip) != nil {
			return ip
		}
	}

	// Check X-Forwarded-For header
	if xff := c.Request().Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		for _, ip := range ips {
			ip = strings.TrimSpace(ip)
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}

	// Fallback to RemoteAddr
	if ip, _, err := net.SplitHostPort(c.Request().RemoteAddr); err == nil {
		if net.ParseIP(ip) != nil {
			return ip
		}
	}

	return ""
}

// isWhitelistedPath checks if the path is whitelisted
func isWhitelistedPath(path string, whitelistedPaths []string) bool {
	for _, whitelistedPath := range whitelistedPaths {
		if path == whitelistedPath || strings.HasPrefix(path, whitelistedPath) {
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
			limiter = &rateLimiter{
				limiter: rate.NewLimiter(
					rate.Every(config.WindowSize/time.Duration(config.RateLimit)),
					config.BurstLimit,
				),
			}
			limiters[clientIP] = limiter
		}
		mu.Unlock()
	}

	// Check if IP is currently blocked
	if !limiter.blockedAt.IsZero() {
		if time.Since(limiter.blockedAt) < config.BlockDuration {
			return false
		}
		// Unblock the IP
		limiter.blockedAt = time.Time{}
	}

	// Check rate limit
	if !limiter.limiter.Allow() {
		// Block the IP
		limiter.blockedAt = time.Now()
		return false
	}

	return true
}

// Circuit breaker methods
func (cb *circuitBreaker) shouldBlock() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case circuitClosed:
		return false
	case circuitOpen:
		if time.Since(cb.lastFailureTime) > cb.config.RecoveryTimeout {
			cb.mu.RUnlock()
			cb.mu.Lock()
			cb.state = circuitHalfOpen
			cb.mu.Unlock()
			cb.mu.RLock()
			return false
		}
		return true
	case circuitHalfOpen:
		return false
	}
	return false
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
		if !limiter.blockedAt.IsZero() && limiter.blockedAt.Before(cutoff) {
			delete(limiters, ip)
		}
	}
}