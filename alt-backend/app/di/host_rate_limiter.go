package di

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"alt/shared/driver/redis_slot_store"
	"alt/utils/rate_limiter"
)

// HostRateLimiterCoordinator is one process's answer to "who else is fetching
// this host right now".
//
// ADR-000954 split alt-backend into three binaries, two of which reach the
// open internet. The external review's weakness 5: with a process-local
// HostRateLimiter in each, the effective rate against a publisher is up to
// twice what the operator configured — feed-registration validation
// (cmd/backend, synchronous) races the hourly collector (cmd/harvester), and
// article archiving (cmd/backend, synchronous) races og-image backfill
// (cmd/harvester). CLAUDE.md rule 2 is a promise to that publisher's server,
// so the fix cannot be per process.
//
// The alternative considered and rejected was moving cmd/backend's fetches to
// cmd/harvester so only one binary talks to the internet. Both of them are
// synchronous from the user's point of view — the feed the user just added is
// validated before the response, the article they opened is archived on the
// way — and moving them means making them asynchronous, which is a redesign of
// two user-visible flows rather than a rate-limit fix.
//
// This type owns the shared arbiter and hands out limiters that use it. One
// coordinator per process; one limiter per rate-limit class.
type HostRateLimiterCoordinator struct {
	store rate_limiter.SlotStore
	owner string
	// closer is non-nil in distributed mode, for a caller that wants to release
	// the pool on shutdown.
	closer interface{ Close() error }
}

// NewHostRateLimiterCoordinator builds the coordinator and states its mode
// loudly, once, at startup.
//
// redisURL empty is the explicit local mode: the interval is enforced in this
// process only, which is the pre-split guarantee and no worse than what
// ADR-000954 shipped — but it is a decision the operator makes, and the
// startup log says so in as many words. A non-empty URL that cannot be turned
// into a store panics rather than falling back: an operator who set the
// variable believes coordination is on, and quietly giving them the local mode
// is precisely the silent-fallback shape CLAUDE.md rule 8 forbids.
func NewHostRateLimiterCoordinator(binary, redisURL string) *HostRateLimiterCoordinator {
	owner := fmt.Sprintf("%s/%d", binary, os.Getpid())

	if redisURL == "" {
		slog.Warn("host_rate_limiter.mode",
			"mode", rate_limiter.ModeLocal,
			"binary", binary,
			"reason", "HOST_RATE_LIMITER_REDIS_URL is unset",
			"impact", "the per-host interval holds inside this process only; another alt-backend binary fetching the same host can double the effective rate (CLAUDE.md rule 2)")
		return &HostRateLimiterCoordinator{owner: owner}
	}

	store, err := redis_slot_store.NewFromURL(redisURL)
	if err != nil {
		panic(fmt.Sprintf("host rate limiter: HOST_RATE_LIMITER_REDIS_URL is set but unusable (%v); "+
			"unset it to run in local mode deliberately", err))
	}

	slog.Info("host_rate_limiter.mode",
		"mode", rate_limiter.ModeDistributed,
		"binary", binary,
		"owner", owner,
		"url", redisURL)

	// Reachability is reported, not enforced: redis may still be starting when
	// this process does, and a fetch path that refuses to run without its
	// arbiter would be a worse outage than one that runs with the local
	// interval. The per-fetch degradation warning covers the rest.
	pingCtx, cancel := context.WithTimeout(context.Background(), hostRateLimiterPingTimeout)
	defer cancel()
	if err := store.Ping(pingCtx); err != nil {
		slog.Warn("host_rate_limiter.arbiter_unreachable_at_startup",
			"binary", binary,
			"url", redisURL,
			"error", err.Error(),
			"impact", "starting in distributed mode but degrading to the local interval until the arbiter answers")
	}

	return &HostRateLimiterCoordinator{store: store, owner: owner, closer: store}
}

const hostRateLimiterPingTimeout = 2 * time.Second

// Mode is rate_limiter.ModeDistributed or rate_limiter.ModeLocal.
func (c *HostRateLimiterCoordinator) Mode() string {
	if c.store == nil {
		return rate_limiter.ModeLocal
	}
	return rate_limiter.ModeDistributed
}

// Owner identifies this process in the slots it holds.
func (c *HostRateLimiterCoordinator) Owner() string { return c.owner }

// Limiter builds a per-host limiter for one rate-limit class. Callers in
// different processes that pass the same namespace coordinate with each other;
// different namespaces do not (see rate_limiter.Coordination.Namespace).
func (c *HostRateLimiterCoordinator) Limiter(namespace string, interval time.Duration, burst int) *rate_limiter.HostRateLimiter {
	return rate_limiter.NewCoordinatedHostRateLimiter(interval, burst, rate_limiter.Coordination{
		Store:     c.store,
		Namespace: namespace,
		Owner:     c.owner,
	})
}

// Close releases the arbiter connection pool. A no-op in local mode.
func (c *HostRateLimiterCoordinator) Close() error {
	if c.closer == nil {
		return nil
	}
	if err := c.closer.Close(); err != nil {
		return fmt.Errorf("close host rate limiter coordinator: %w", err)
	}
	return nil
}
