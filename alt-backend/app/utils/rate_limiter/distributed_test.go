package rate_limiter

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// fakeSlotStore is an in-memory stand-in for the Redis arbiter. It implements
// exactly the SET NX PX / PTTL contract the real store has: the first caller
// for a key inside the interval wins, everybody else is told how long is left.
type fakeSlotStore struct {
	mu sync.Mutex
	// now is the store's clock. Tests advance it rather than sleeping, so the
	// slot arithmetic is asserted without waiting on wall time.
	now      time.Time
	expiries map[string]time.Time
	owners   map[string]string

	err       error
	acquires  int
	lastTTL   time.Duration
	lastOwner string
	keys      []string
}

func newFakeSlotStore() *fakeSlotStore {
	return &fakeSlotStore{
		now:      time.Unix(0, 0),
		expiries: make(map[string]time.Time),
		owners:   make(map[string]string),
	}
}

func (f *fakeSlotStore) AcquireSlot(ctx context.Context, key, owner string, ttl time.Duration) (bool, time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.acquires++
	f.lastTTL = ttl
	f.lastOwner = owner
	f.keys = append(f.keys, key)

	if f.err != nil {
		return false, 0, f.err
	}

	if expiry, held := f.expiries[key]; held && expiry.After(f.now) {
		return false, expiry.Sub(f.now), nil
	}

	f.expiries[key] = f.now.Add(ttl)
	f.owners[key] = owner
	return true, 0, nil
}

func (f *fakeSlotStore) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

func (f *fakeSlotStore) snapshot() (acquires int, lastTTL time.Duration, keys []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.acquires, f.lastTTL, append([]string(nil), f.keys...)
}

func (f *fakeSlotStore) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// alwaysAllowLimiter removes the in-process interval from a host, so a test
// can assert what the shared store contributes on its own.
func alwaysAllowLimiter() *rate.Limiter {
	return rate.NewLimiter(rate.Inf, 1)
}

// captureLogs swaps the limiter's logger for one writing into a buffer so the
// loud degradation signal can be asserted rather than assumed.
func captureLogs(h *HostRateLimiter) *bytes.Buffer {
	buf := &bytes.Buffer{}
	h.logger = slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return buf
}

func TestNewCoordinatedHostRateLimiter_Mode(t *testing.T) {
	tests := []struct {
		name     string
		store    SlotStore
		wantMode string
	}{
		{
			name:     "a store makes the limiter distributed",
			store:    newFakeSlotStore(),
			wantMode: ModeDistributed,
		},
		{
			name:     "no store is the explicit local mode",
			store:    nil,
			wantMode: ModeLocal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := NewCoordinatedHostRateLimiter(50*time.Millisecond, 1, Coordination{
				Store:     tt.store,
				Namespace: "external_api",
				Owner:     "alt-backend/1",
			})
			require.NotNil(t, limiter)
			assert.Equal(t, tt.wantMode, limiter.Mode())
		})
	}
}

func TestHostRateLimiter_DistributedSlot(t *testing.T) {
	t.Run("acquires the slot on the first call", func(t *testing.T) {
		store := newFakeSlotStore()
		limiter := NewCoordinatedHostRateLimiter(5*time.Second, 1, Coordination{
			Store: store, Namespace: "external_api", Owner: "alt-backend/1",
		})

		require.NoError(t, limiter.WaitForHost(context.Background(), "https://example.com/feed.xml"))

		acquires, ttl, keys := store.snapshot()
		assert.Equal(t, 1, acquires)
		assert.Equal(t, 5*time.Second, ttl, "the slot TTL is the host interval")
		require.Len(t, keys, 1)
		assert.Equal(t, "external_api:example.com", keys[0],
			"the key is namespaced by rate-limit class so a 1s image slot cannot satisfy a 5s feed promise")
	})

	t.Run("a slot held by another process blocks until it expires", func(t *testing.T) {
		store := newFakeSlotStore()
		// The harvester got there first: the slot for example.com is taken.
		taken, _, err := store.AcquireSlot(context.Background(), "external_api:example.com", "alt-harvester/1", 200*time.Millisecond)
		require.NoError(t, err)
		require.True(t, taken)

		// A limiter with no local interval at all: whatever serialisation the
		// caller observes here comes from the shared store, not from
		// golang.org/x/time/rate.
		limiter := NewCoordinatedHostRateLimiter(200*time.Millisecond, 1, Coordination{
			Store: store, Namespace: "external_api", Owner: "alt-backend/1",
		})
		limiter.limiters["example.com"] = alwaysAllowLimiter()

		done := make(chan error, 1)
		go func() {
			done <- limiter.WaitForHost(context.Background(), "https://example.com/a")
		}()

		select {
		case err := <-done:
			t.Fatalf("WaitForHost returned while the slot was held: %v", err)
		case <-time.After(50 * time.Millisecond):
		}

		store.advance(200 * time.Millisecond)

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("WaitForHost never acquired the slot after it expired")
		}
	})

	t.Run("a cancelled context ends the wait with an error", func(t *testing.T) {
		store := newFakeSlotStore()
		_, _, err := store.AcquireSlot(context.Background(), "external_api:example.com", "alt-harvester/1", time.Hour)
		require.NoError(t, err)

		limiter := NewCoordinatedHostRateLimiter(time.Hour, 1, Coordination{
			Store: store, Namespace: "external_api", Owner: "alt-backend/1",
		})
		limiter.limiters["example.com"] = alwaysAllowLimiter()

		ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
		defer cancel()

		err = limiter.WaitForHost(ctx, "https://example.com/a")
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("two processes sharing one store are serialised", func(t *testing.T) {
		store := newFakeSlotStore()
		backend := NewCoordinatedHostRateLimiter(time.Hour, 1, Coordination{
			Store: store, Namespace: "external_api", Owner: "alt-backend/1",
		})
		harvester := NewCoordinatedHostRateLimiter(time.Hour, 1, Coordination{
			Store: store, Namespace: "external_api", Owner: "alt-harvester/1",
		})
		// Both processes are locally idle — the only thing that can hold one
		// back is the other's slot. This is the review's "worst case 2x"
		// scenario expressed as a test.
		backend.limiters["example.com"] = alwaysAllowLimiter()
		harvester.limiters["example.com"] = alwaysAllowLimiter()

		require.NoError(t, backend.WaitForHost(context.Background(), "https://example.com/a"))

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err := harvester.WaitForHost(ctx, "https://example.com/b")
		require.Error(t, err, "the second process must not be let through inside the interval")
	})

	t.Run("the slot TTL follows the host's 429 backoff", func(t *testing.T) {
		store := newFakeSlotStore()
		limiter := NewCoordinatedHostRateLimiter(time.Second, 1, Coordination{
			Store: store, Namespace: "external_api", Owner: "alt-backend/1",
		})
		limiter.RecordRateLimitHit("example.com", 30*time.Second)
		limiter.limiters["example.com"] = alwaysAllowLimiter()

		require.NoError(t, limiter.WaitForHost(context.Background(), "https://example.com/a"))

		_, ttl, _ := store.snapshot()
		assert.Equal(t, 30*time.Second, ttl,
			"a 429 must widen the slot for every process, not only the one that saw it")
	})
}

func TestHostRateLimiter_DegradesLoudly(t *testing.T) {
	store := newFakeSlotStore()
	store.setErr(errors.New("dial tcp: connection refused"))

	limiter := NewCoordinatedHostRateLimiter(10*time.Millisecond, 1, Coordination{
		Store: store, Namespace: "external_api", Owner: "alt-backend/1",
	})
	logs := captureLogs(limiter)

	// Unreachable Redis must not fail the fetch: the local interval is still
	// held, so the caller keeps the pre-split guarantee.
	require.NoError(t, limiter.WaitForHost(context.Background(), "https://example.com/a"))

	assert.Contains(t, logs.String(), "host_rate_limiter.degraded_to_local")
	assert.Contains(t, logs.String(), "connection refused")

	// Rate-limited: a second failure inside the window must not add a line.
	linesAfterFirst := strings.Count(logs.String(), "host_rate_limiter.degraded_to_local")
	require.NoError(t, limiter.WaitForHost(context.Background(), "https://example.com/b"))
	assert.Equal(t, linesAfterFirst, strings.Count(logs.String(), "host_rate_limiter.degraded_to_local"),
		"the degradation warning must be rate limited, not one line per fetch")

	// Recovery is as loud as degradation: without it the log says the
	// coordination broke and never says it came back.
	store.setErr(nil)
	require.NoError(t, limiter.WaitForHost(context.Background(), "https://example.com/c"))
	assert.Contains(t, logs.String(), "host_rate_limiter.recovered_distributed")
}

func TestHostRateLimiter_LocalModeNeverTouchesAStore(t *testing.T) {
	limiter := NewHostRateLimiter(10 * time.Millisecond)
	assert.Equal(t, ModeLocal, limiter.Mode())
	require.NoError(t, limiter.WaitForHost(context.Background(), "https://example.com/a"))
}
