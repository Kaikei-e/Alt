package rate_limiter

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"alt/utils/randutil"
)

// CLAUDE.md rule 2 ("5-second minimum intervals between external API calls")
// is a promise made to somebody else's server, and the unit it is measured in
// is the host, not the process. ADR-000954 split alt-backend into three
// binaries, two of which fetch from the open internet: cmd/backend does it
// synchronously (feed-registration validation, article archiving) and
// cmd/harvester does it on a schedule (hourly feed collection, og-image
// backfill and warming). Both built their own *HostRateLimiter, and a
// token bucket in process A knows nothing about the requests process B is
// making — so the same publisher could see two requests per interval, and the
// interval the operator configured would be a fiction.
//
// This file restores the invariant across processes without moving the
// synchronous fetches, by putting a shared arbiter (Redis, in production)
// between the two limiters. The local token bucket stays: it is what keeps a
// process from stampeding the arbiter with one round trip per goroutine, and
// it is what still holds when the arbiter is unreachable.

const (
	// ModeDistributed means a SlotStore arbitrates the per-host interval, so
	// the promise holds across every process sharing that store.
	ModeDistributed = "distributed"
	// ModeLocal means the interval is enforced in this process only. It is a
	// deliberate configuration (HOST_RATE_LIMITER_REDIS_URL unset), never an
	// inferred one, and the composition roots log it loudly at startup —
	// otherwise "coordination is off" would be indistinguishable from
	// "somebody forgot to wire it" (CLAUDE.md rules 8 and 9).
	ModeLocal = "local"
)

// The rate-limit classes that exist. A namespace is the set of callers that
// must see each other's requests; two classes with different intervals against
// the same host are deliberately blind to each other (see
// Coordination.Namespace).
const (
	// NamespaceExternalAPI covers everything CLAUDE.md rule 2 is about: feed
	// polling (cmd/harvester's hourly collector, cmd/backend's registration
	// validation) and article fetching (cmd/backend's archive path,
	// cmd/harvester's og-image backfill). These are the four call sites the
	// ADR-000954 review found doubled.
	NamespaceExternalAPI = "external_api"
	// NamespaceImageProxy covers the OGP image fetches, which run at 1s
	// because they are user-triggered CDN reads rather than crawls. Both
	// cmd/backend (serving) and cmd/harvester (warmer/backfill) do them, so
	// they need coordinating too — just not with the feed traffic.
	NamespaceImageProxy = "image_proxy"
)

const (
	// slotRetryFloor keeps a retry loop from spinning when the arbiter reports
	// a sub-millisecond remainder (or clock skew makes it look negative).
	slotRetryFloor = 20 * time.Millisecond
	// degradedLogInterval throttles the degradation warning. The warning has
	// to be loud, but one line per fetch would bury the rest of the log during
	// a Redis outage — so it is emitted once per window with the number of
	// occurrences it stands for.
	degradedLogInterval = 30 * time.Second
)

// SlotStore is the cross-process half of the per-host interval. One slot per
// (namespace, host) exists at a time; whoever creates it may make the request,
// and everybody else waits for it to expire.
//
// The contract is deliberately the one Redis gives directly:
//
//	SET <key> <owner> PX <ttl> NX   -> acquired
//	PTTL <key>                      -> retryAfter
//
// AcquireSlot returns acquired=true when the caller now owns the slot. When it
// returns false, retryAfter is how long the current owner's slot still has to
// run (zero if the store could not say). An error means the store could not be
// reached — callers degrade to their local interval rather than failing the
// request, because a Redis outage must not stop the product from fetching.
//
// Slots are TTL-based and never explicitly released: a process that crashes
// mid-fetch leaves a slot that expires on its own, which is the behaviour we
// want (its request may well have reached the host).
type SlotStore interface {
	AcquireSlot(ctx context.Context, key, owner string, ttl time.Duration) (acquired bool, retryAfter time.Duration, err error)
}

// Coordination configures the cross-process half of a HostRateLimiter.
type Coordination struct {
	// Store is the arbiter. A nil Store is ModeLocal.
	Store SlotStore

	// Namespace separates rate-limit classes that legitimately run at
	// different intervals against the same host. The external-API limiter
	// (5s+) and the image-proxy limiter (1s) must not share slots: a 1s slot
	// would let a 5s caller through four seconds early, and a 5s slot would
	// throttle the image proxy to a fifth of its intended rate. Processes that
	// must coordinate with each other use the same namespace — that is the
	// whole mechanism.
	Namespace string

	// Owner is written into the slot value. It is never read back for
	// correctness (the SET NX is what decides), only for debugging: `GET` on a
	// contended key answers "which process is holding this host right now".
	Owner string
}

// NewCoordinatedHostRateLimiter builds a limiter that enforces its interval
// both in-process and, when coord.Store is non-nil, across every process
// sharing that store and namespace.
//
// It is the same type as NewHostRateLimiter returns, so every existing
// consumer (fetch_article_gateway, fetch_feed_gateway, image_proxy_usecase,
// batch_article_fetcher, the feed collector job) is coordinated by
// construction rather than by each of them opting in.
func NewCoordinatedHostRateLimiter(interval time.Duration, burst int, coord Coordination) *HostRateLimiter {
	h := NewHostRateLimiter(interval, burst)
	h.coord = coord
	return h
}

// Mode reports whether this limiter arbitrates across processes. Composition
// roots log it at startup.
func (h *HostRateLimiter) Mode() string {
	if h.coord.Store == nil {
		return ModeLocal
	}
	return ModeDistributed
}

// slotKey is the arbiter key for a host. Namespacing by rate-limit class is
// load-bearing — see Coordination.Namespace.
func (h *HostRateLimiter) slotKey(host string) string {
	return h.coord.Namespace + ":" + host
}

// waitForSlot blocks until this process owns the shared slot for host, the
// context ends, or the arbiter turns out to be unreachable.
func (h *HostRateLimiter) waitForSlot(ctx context.Context, host string) error {
	if h.coord.Store == nil {
		return nil
	}

	key := h.slotKey(host)

	for {
		ttl := h.slotTTL(host)

		acquired, retryAfter, err := h.coord.Store.AcquireSlot(ctx, key, h.coord.Owner, ttl)
		if err != nil {
			// A cancelled caller is not a broken arbiter; report it as what it is.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return fmt.Errorf("wait for host slot %q: %w", host, ctxErr)
			}
			h.logDegraded(ctx, host, err)
			// Fail degraded, not closed: the in-process interval was already
			// held before we got here, so the caller keeps exactly the
			// guarantee it had before the split.
			return nil
		}

		if acquired {
			h.logRecovered(ctx, host)
			return nil
		}

		wait := retryAfter
		if wait <= 0 || wait > ttl {
			// Either the arbiter could not say (key expired between the SET
			// and the PTTL) or it reported something longer than a full slot,
			// which can only be skew. A full interval is the safe answer in
			// both directions.
			wait = ttl
		}
		if wait < slotRetryFloor {
			wait = slotRetryFloor
		}
		// Jitter keeps two processes that started blocked on the same host
		// from retrying in lockstep forever. crypto/rand failure skips the
		// extra jitter rather than crashing the limiter — the computed wait
		// still holds.
		extra, jitterErr := randutil.JitterInt64(int64(wait / 8))
		if jitterErr != nil {
			h.log().WarnContext(ctx, "host_rate_limiter.jitter_unavailable",
				"host", host,
				"error", jitterErr.Error(),
				"impact", "retrying without extra jitter; processes may align")
		} else {
			wait += time.Duration(extra)
		}

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("wait for host slot %q: %w", host, ctx.Err())
		case <-timer.C:
		}
	}
}

// slotTTL is how long this host's slot should hold — the base interval, or the
// wider one a 429 backoff left behind. Using the backed-off value means a 429
// seen by one process slows every process down, which is the point of having
// the arbiter at all.
func (h *HostRateLimiter) slotTTL(host string) time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if current, backedOff := h.currentIntervals[host]; backedOff {
		return current
	}
	return h.interval
}

func (h *HostRateLimiter) log() *slog.Logger {
	if h.logger != nil {
		return h.logger
	}
	return slog.Default()
}

// logDegraded emits the rule-8 signal that cross-process coordination is not
// happening right now. Losing it silently is the failure mode this whole file
// exists to prevent: the fetches keep working, so nothing else would notice
// that the 5-second promise had quietly become a 5-second-per-process promise.
func (h *HostRateLimiter) logDegraded(ctx context.Context, host string, cause error) {
	h.mu.Lock()
	h.degraded = true
	h.degradedCount++
	count := h.degradedCount
	now := time.Now()
	shouldLog := h.lastDegradedLog.IsZero() || now.Sub(h.lastDegradedLog) >= degradedLogInterval
	if shouldLog {
		h.lastDegradedLog = now
	}
	h.mu.Unlock()

	if !shouldLog {
		return
	}

	h.log().WarnContext(ctx, "host_rate_limiter.degraded_to_local",
		"namespace", h.coord.Namespace,
		"owner", h.coord.Owner,
		"host", host,
		"occurrences", count,
		"error", cause.Error(),
		"impact", "per-host interval is enforced in this process only; another process may fetch the same host inside the interval")
}

// logRecovered closes the degraded window. Emitted only on the transition, so
// it costs nothing on the happy path.
func (h *HostRateLimiter) logRecovered(ctx context.Context, host string) {
	h.mu.Lock()
	if !h.degraded {
		h.mu.Unlock()
		return
	}
	count := h.degradedCount
	h.degraded = false
	h.degradedCount = 0
	h.lastDegradedLog = time.Time{}
	h.mu.Unlock()

	h.log().InfoContext(ctx, "host_rate_limiter.recovered_distributed",
		"namespace", h.coord.Namespace,
		"owner", h.coord.Owner,
		"host", host,
		"degraded_acquisitions", count)
}
