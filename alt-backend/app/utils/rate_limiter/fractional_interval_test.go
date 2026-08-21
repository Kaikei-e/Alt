package rate_limiter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The default per-host interval is 7.5s, the first one that is not a whole
// number of seconds. Every hop it takes — token bucket, slot TTL, retry-after
// hint, backoff doubling — is time.Duration arithmetic today, and these tests
// exist so that a later "int(d.Seconds())" anywhere on that path fails here
// instead of quietly turning the operator's 7.5s into 7s (rule 2 violated) or
// 8s (rate silently lowered).

const fractionalInterval = 7500 * time.Millisecond

func TestHostRateLimiter_FractionalIntervalReachesTheSlotStoreExactly(t *testing.T) {
	store := newFakeSlotStore()
	limiter := NewCoordinatedHostRateLimiter(fractionalInterval, 3, Coordination{
		Store: store, Namespace: NamespaceExternalAPI, Owner: "alt-backend/1",
	})

	require.NoError(t, limiter.WaitForHost(context.Background(), "https://example.com/feed.xml"))

	_, ttl, keys := store.snapshot()
	assert.Equal(t, fractionalInterval, ttl,
		"the shared slot TTL is the interval verbatim; a whole-second conversion would show up as 7s or 8s")
	assert.NotEqual(t, 7*time.Second, ttl)
	assert.NotEqual(t, 8*time.Second, ttl)
	require.Len(t, keys, 1)
	assert.Equal(t, NamespaceExternalAPI+":example.com", keys[0])
}

func TestHostRateLimiter_RetryAfterForKeepsTheFraction(t *testing.T) {
	limiter := NewHostRateLimiter(fractionalInterval, 3)

	assert.Equal(t, fractionalInterval, limiter.RetryAfterFor("https://example.com/a"),
		"the retry-after hint handed to a client is the interval, not a rounded second")

	// An unparseable URL falls back to the base interval; it must not round either.
	assert.Equal(t, fractionalInterval, limiter.RetryAfterFor("::not a url::"))
}

// slotTTL is what distributed.go hands the arbiter, both at the base interval
// and after a 429 widened it. 7.5s doubled is 15s; a seconds-typed backoff
// would land on 14s or 16s.
func TestHostRateLimiter_SlotTTLKeepsTheFractionThroughBackoff(t *testing.T) {
	limiter := NewHostRateLimiter(fractionalInterval, 3)

	assert.Equal(t, fractionalInterval, limiter.slotTTL("example.com"))

	limiter.RecordRateLimitHit("example.com", 0)
	assert.Equal(t, 15*time.Second, limiter.slotTTL("example.com"),
		"a 429 doubles the host's current interval; 7.5s doubles to exactly 15s")

	limiter.RecordSuccess("example.com")
	assert.Equal(t, fractionalInterval, limiter.slotTTL("example.com"),
		"decay returns to the configured interval, fraction intact")
}

// The in-process token bucket is golang.org/x/time/rate, which takes a
// rate.Limit (events per second) rather than a duration. 1/7.5s is not
// representable exactly in binary floating point, so this asserts the
// behaviour that matters — the spacing between two turns — with a tolerance
// far tighter than the 500ms that separates 7.5s from 7s or 8s.
func TestHostRateLimiter_TokenBucketSpacingIsSevenPointFiveSeconds(t *testing.T) {
	limiter := NewHostRateLimiter(fractionalInterval, 1)
	bucket := limiter.getLimiterForHost("example.com")

	require.True(t, bucket.Allow(), "burst 1 means the first request goes immediately")

	delay := bucket.Reserve().Delay()
	assert.InDelta(t, fractionalInterval.Seconds(), delay.Seconds(), 0.05,
		"the next turn must be ~7.5s away, not 7s and not 8s")
	assert.Greater(t, delay, 7*time.Second+400*time.Millisecond)
	assert.Less(t, delay, 7*time.Second+600*time.Millisecond)
}

// The two internet-facing binaries at the shipping settings: 7.5s, burst 3,
// one shared arbiter, one namespace. Two things are asserted at once, both of
// them things the report on this change claims.
//
//  1. cmd/backend and cmd/harvester coordinate — the harvester's collector
//     cannot fetch a host cmd/backend just fetched, inside the interval.
//  2. burst 3 does not become three requests on the wire. The token bucket is
//     per process and hands out three turns immediately on a cold start, but
//     each of those turns then has to take the shared slot, whose TTL is the
//     full interval. In distributed mode the burst therefore buys concurrency
//     inside this process, not extra requests at the publisher.
func TestHostRateLimiter_BurstDoesNotStackOnTheSharedSlot(t *testing.T) {
	store := newFakeSlotStore()

	newProcess := func(owner string) *HostRateLimiter {
		return NewCoordinatedHostRateLimiter(fractionalInterval, 3, Coordination{
			Store: store, Namespace: NamespaceExternalAPI, Owner: owner,
		})
	}
	backend := newProcess("alt-backend/1")
	harvester := newProcess("alt-harvester/1")

	// First turn: the local bucket is full (burst 3) and the slot is free.
	require.NoError(t, backend.WaitForHost(context.Background(), "https://example.com/a"))

	// Second turn in the same process. The token bucket still has burst
	// capacity, so anything that stops it now is the shared slot.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	require.Error(t, backend.WaitForHost(ctx, "https://example.com/b"),
		"burst 3 must not put a second request on the wire inside the interval when an arbiter is configured")

	// And the other binary is held by the same slot.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	require.Error(t, harvester.WaitForHost(ctx2, "https://example.com/c"),
		"cmd/harvester must see cmd/backend's request; that is what makes the interval a promise about the host")

	// When the slot expires, the next caller gets in — one request per 7.5s.
	store.advance(fractionalInterval)
	require.NoError(t, harvester.WaitForHost(context.Background(), "https://example.com/c"))
}
