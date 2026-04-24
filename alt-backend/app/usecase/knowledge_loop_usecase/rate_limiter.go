package knowledge_loop_usecase

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// LoopRateLimiter enforces the two canonical Knowledge Loop rate limits
// defined in the canonical contract §8.4:
//
//  1. Per-entry Observe throttle — `(user_id, entry_key, lens_mode_id)`
//     produces at most one observed event per 60 seconds. Prevents the 1.5s
//     dwell detector from flooding the event log when a tile stays in view.
//  2. User global ceiling — a user may emit at most 600 Loop events per
//     minute across all entries. Caps pathological clients that rotate entry
//     keys to sidestep the per-entry throttle.
//
// Both buckets are in-memory and per-process. The minor consequence is that a
// multi-pod deployment could drift up to N pods × 600/min on the global
// ceiling, which is acceptable for the dwell-driven write path: each
// knowledge_events row still has `(user_id, client_transition_id)` idempotency
// in sovereign, and the global ceiling is a defense-in-depth measure rather
// than an accounting layer.
//
// ADR-000840 deliberately deferred this to a follow-up PR; this file is that
// follow-up. The rate limiter itself does not mutate projection state and is
// safe under immutable-design-guard F3 (read-only write-path gate).
type LoopRateLimiter struct {
	now func() time.Time

	mu sync.Mutex
	// observeLastAt[user+lens+entry] = last allowed observe time.
	observeLastAt map[string]time.Time
	// globalCounter tracks (user, minute bucket) → count.
	globalCounter map[globalKey]int
}

type globalKey struct {
	user   uuid.UUID
	minute int64 // unix-seconds / 60
}

const (
	observeMinInterval = 60 * time.Second
	userMinuteCeiling  = 600
)

// NewLoopRateLimiter constructs an in-memory rate limiter. Tests may pass a
// fake clock; production code passes time.Now.
func NewLoopRateLimiter(now func() time.Time) *LoopRateLimiter {
	if now == nil {
		now = time.Now
	}
	return &LoopRateLimiter{
		now:           now,
		observeLastAt: make(map[string]time.Time),
		globalCounter: make(map[globalKey]int),
	}
}

// AllowObserve checks both the per-entry throttle and the user global ceiling.
// Returns (true, "") when the observe may fire, or (false, reason) otherwise.
// The reason string is suitable for `Retry-After` hints and structured warn
// logs (canonical contract §8.4).
//
// An allowed observe also increments the user global counter so pathological
// clients cannot sidestep the ceiling by rotating entry keys.
func (r *LoopRateLimiter) AllowObserve(userID uuid.UUID, lensModeID, entryKey string, at time.Time) (bool, string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Step 1: per-entry window.
	key := userID.String() + "|" + lensModeID + "|" + entryKey
	if last, ok := r.observeLastAt[key]; ok {
		if at.Sub(last) < observeMinInterval {
			return false, "observe_throttle"
		}
	}

	// Step 2: user global ceiling. We check before recording so a rejection
	// here does not burn a counter slot the caller could retry into later.
	gkey := globalKey{user: userID, minute: at.Unix() / 60}
	if r.globalCounter[gkey] >= userMinuteCeiling {
		return false, "user_global_ceiling"
	}

	// Permit: record both the per-entry timestamp and the global counter.
	r.observeLastAt[key] = at
	r.globalCounter[gkey]++
	return true, ""
}

// AllowGlobal is the global-only entry point. Used by transition paths that
// are not per-entry throttled (e.g. Decide → Act) but still count against the
// user minute ceiling. Symmetric to AllowObserve but skips the per-entry gate.
func (r *LoopRateLimiter) AllowGlobal(userID uuid.UUID, at time.Time) (bool, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	gkey := globalKey{user: userID, minute: at.Unix() / 60}
	if r.globalCounter[gkey] >= userMinuteCeiling {
		return false, "user_global_ceiling"
	}
	r.globalCounter[gkey]++
	return true, ""
}
