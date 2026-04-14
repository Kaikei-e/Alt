package utils

import (
	"context"
	"sync"
	"time"
)

// HostRateLimiter enforces a minimum interval between successive calls to the
// same host. It is deliberately simple: a per-host last-call timestamp gated
// by a mutex. Internal callers (driver/summarizer_api.go) invoke Wait before
// issuing the outbound request so the CLAUDE.md "5 second floor between
// external API calls" rule is enforced centrally instead of scattered.
type HostRateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	lastCall map[string]time.Time
	now      func() time.Time
	sleep    func(context.Context, time.Duration) error
}

// NewHostRateLimiter returns a limiter with the supplied interval. An interval
// of zero disables rate limiting (Wait becomes a no-op).
func NewHostRateLimiter(interval time.Duration) *HostRateLimiter {
	return &HostRateLimiter{
		interval: interval,
		lastCall: make(map[string]time.Time),
		now:      time.Now,
		sleep:    ctxSleep,
	}
}

// Wait blocks until the next call to host is allowed. It honours ctx so
// caller cancellation takes precedence over the interval.
func (h *HostRateLimiter) Wait(ctx context.Context, host string) error {
	if h == nil || h.interval <= 0 {
		return nil
	}

	h.mu.Lock()
	last, ok := h.lastCall[host]
	now := h.now()
	var wait time.Duration
	if ok {
		if elapsed := now.Sub(last); elapsed < h.interval {
			wait = h.interval - elapsed
		}
	}
	// Reserve the slot by recording the projected call time so concurrent
	// waiters don't all fire at the same instant once we release the lock.
	h.lastCall[host] = now.Add(wait)
	h.mu.Unlock()

	if wait <= 0 {
		return nil
	}
	return h.sleep(ctx, wait)
}

func ctxSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// hostRateLimiter is the process-wide default used by driver-layer callers so
// there is a single limiter per pre-processor instance regardless of how many
// clients are constructed.
var (
	hostRateLimiterOnce sync.Once
	hostRateLimiter     *HostRateLimiter
)

// DefaultHostRateLimiter returns the process-wide limiter, initialising it on
// first use with the supplied interval. Subsequent calls ignore the interval
// argument; callers pass the configured RateLimit.DefaultInterval at wire
// time from bootstrap.
func DefaultHostRateLimiter(interval time.Duration) *HostRateLimiter {
	hostRateLimiterOnce.Do(func() {
		hostRateLimiter = NewHostRateLimiter(interval)
	})
	return hostRateLimiter
}
