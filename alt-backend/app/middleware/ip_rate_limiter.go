package middleware

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// rateLimiter holds rate limiter and associated metadata
type rateLimiter struct {
	limiter   *rate.Limiter
	blockedAt time.Time
	lastSeen  time.Time  // last request observed for this IP; drives inactivity eviction
	mu        sync.Mutex // Protects blockedAt/lastSeen fields
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

	// Check if IP is currently blocked (with proper synchronization). Also
	// stamps lastSeen so CleanupExpiredLimiters can evict by inactivity
	// instead of only evicting entries that were blocked at least once
	// (an IP that never trips the rate limit still needs its limiter
	// reclaimed once it goes cold).
	limiter.mu.Lock()
	limiter.lastSeen = time.Now()
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

// CleanupExpiredLimiters removes rate limiters that have gone cold — no
// request seen from that IP for at least maxAge. The previous implementation
// only evicted entries that had been blocked at least once, so the (much
// more common) case of an IP that sends a few requests and never trips the
// rate limit was never reclaimed: those limiters accumulated forever under
// normal traffic, not just under attack (did not actually bound
// memory). Eviction is keyed on lastSeen instead, which every request path
// through checkRateLimit stamps regardless of whether the IP gets blocked.
func CleanupExpiredLimiters(limiters map[string]*rateLimiter, mu *sync.RWMutex, maxAge time.Duration) {
	mu.Lock()
	defer mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for ip, limiter := range limiters {
		limiter.mu.Lock()
		shouldDelete := limiter.lastSeen.Before(cutoff)
		limiter.mu.Unlock()

		if shouldDelete {
			delete(limiters, ip)
		}
	}
}
