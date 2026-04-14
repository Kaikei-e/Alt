package middleware

import (
	"math"
	"net/http"
	"strconv"

	"golang.org/x/time/rate"

	"search-indexer/logger"
)

// RateLimiter wraps a single golang.org/x/time/rate Limiter to defend
// search-indexer against request floods from trusted-but-misbehaving
// internal callers. Since every caller is already authenticated with
// X-Service-Token, a global token bucket is sufficient — per-caller
// identification would require propagating caller identity, which
// ADR-000717 defers.
type RateLimiter struct {
	limiter *rate.Limiter
}

// NewRateLimiter constructs a limiter that refills r tokens/second and allows
// bursts up to burst tokens.
func NewRateLimiter(r rate.Limit, burst int) *RateLimiter {
	return &RateLimiter{limiter: rate.NewLimiter(r, burst)}
}

// Middleware returns an http.Handler wrapper that rejects requests when the
// bucket is empty, attaching a Retry-After hint based on the current refill
// rate.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.limiter.Allow() {
			retryAfter := retryAfterSeconds(rl.limiter.Limit())
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			logger.Logger.WarnContext(r.Context(), "rate limit exceeded",
				"path", r.URL.Path, "remote_addr", r.RemoteAddr)
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func retryAfterSeconds(lim rate.Limit) int {
	if lim <= 0 {
		return 1
	}
	sec := 1.0 / float64(lim)
	if sec < 1 {
		return 1
	}
	return int(math.Ceil(sec))
}
