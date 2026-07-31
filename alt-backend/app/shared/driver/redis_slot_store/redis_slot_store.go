// Package redis_slot_store is the Redis-backed arbiter behind
// utils/rate_limiter's cross-process per-host interval.
//
// It exists because ADR-000954 put two internet-facing binaries in one
// deployment (cmd/backend fetches synchronously for the user, cmd/harvester
// fetches on a schedule), and a token bucket in one process cannot see the
// other's requests. CLAUDE.md rule 2 is a promise to the remote host, so the
// arbiter has to sit outside both processes.
//
// The whole protocol is two Redis commands. A slot is a key that exists for
// one interval:
//
//	SET  <prefix><namespace>:<host> <owner> PX <interval> NX   -> won the slot
//	PTTL <prefix><namespace>:<host>                            -> how long to wait
//
// There is no release. Slots expire, which is what makes a process that dies
// mid-fetch harmless: its slot lapses on its own, and until it does the host
// is protected exactly as if the process were still working (its request may
// well have arrived).
package redis_slot_store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// KeyPrefix namespaces every slot key. The production instance is
// redis-streams, which also carries mq-hub's event streams; the prefix keeps
// the two sets of keys legible to an operator running KEYS or SCAN.
const KeyPrefix = "host_rate_limiter:v1:"

const (
	// dialTimeout / opTimeout keep a wedged or overloaded Redis from turning
	// into a fetch that hangs. A slot decision is worth a few milliseconds, not
	// a stalled scheduler tick — past these, the caller degrades to its local
	// interval and says so.
	dialTimeout = 2 * time.Second
	opTimeout   = 1 * time.Second
)

// Store implements rate_limiter.SlotStore against Redis.
type Store struct {
	client redis.UniversalClient
}

// New wraps an already-configured client. Used by the tests (miniredis) and by
// any caller that wants to share a connection pool.
func New(client redis.UniversalClient) *Store {
	return &Store{client: client}
}

// NewFromURL builds a Store from a redis:// URL.
//
// An empty URL is an error rather than a Store that quietly does nothing:
// "coordination is off" is a decision the composition root makes explicitly
// (HOST_RATE_LIMITER_REDIS_URL unset -> rate_limiter.ModeLocal, logged at
// startup), and it must never be something this constructor infers
// (CLAUDE.md rules 8 and 9).
func NewFromURL(rawURL string) (*Store, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("redis slot store: empty URL; the caller must decide between distributed and local mode, not this constructor")
	}

	opt, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("redis slot store: parse url: %w", err)
	}

	opt.DialTimeout = dialTimeout
	opt.ReadTimeout = opTimeout
	opt.WriteTimeout = opTimeout
	// One slot decision per fetch is a trickle; a large pool would only hold
	// idle connections open on a box that runs everything on one host.
	if opt.PoolSize == 0 {
		opt.PoolSize = 4
	}

	return New(redis.NewClient(opt)), nil
}

// AcquireSlot implements rate_limiter.SlotStore.
func (s *Store) AcquireSlot(ctx context.Context, key, owner string, ttl time.Duration) (bool, time.Duration, error) {
	fullKey := KeyPrefix + key

	acquired, err := s.client.SetNX(ctx, fullKey, owner, ttl).Result()
	if err != nil {
		return false, 0, fmt.Errorf("acquire slot %q: %w", fullKey, err)
	}
	if acquired {
		return true, 0, nil
	}

	remaining, err := s.client.PTTL(ctx, fullKey).Result()
	if err != nil {
		// The slot is somebody else's; we simply could not read how long is
		// left. Report no hint rather than an error — the caller already knows
		// it has to wait, and a full interval is its safe default.
		return false, 0, nil
	}
	if remaining < 0 {
		// -1 (no expiry) and -2 (key gone) both mean "no usable hint". The key
		// can vanish between the SET and the PTTL, which is a race we resolve
		// by letting the caller retry.
		return false, 0, nil
	}

	return false, remaining, nil
}

// Close releases the connection pool.
func (s *Store) Close() error {
	if closer, ok := s.client.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			return fmt.Errorf("close redis slot store: %w", err)
		}
	}
	return nil
}

// Ping reports whether the arbiter is reachable. The composition roots call it
// once at startup so the mode log states what is actually true, instead of
// stating the configured intent and leaving the first degraded warning to
// arrive minutes later.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping redis slot store: %w", err)
	}
	return nil
}
