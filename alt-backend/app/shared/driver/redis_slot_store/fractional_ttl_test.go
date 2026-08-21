package redis_slot_store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The per-host slot TTL is the configured interval, whose default is 7.5s.
// go-redis picks SET ... EX <seconds> for whole-second durations and
// SET ... PX <milliseconds> otherwise, so a fractional interval only survives
// the wire if the sub-second remainder is preserved. If it ever degraded to EX,
// 7.5s would reach Redis as 7s and every process would be allowed back onto the
// publisher half a second early — a rule 2 violation with no log line.
func TestStore_AcquireSlot_KeepsAFractionalTTL(t *testing.T) {
	store, mr := newTestStore(t)

	const interval = 7500 * time.Millisecond
	acquired, _, err := store.AcquireSlot(context.Background(), "external_api:example.com", "alt-backend/1", interval)
	require.NoError(t, err)
	require.True(t, acquired)

	ttl := mr.TTL(KeyPrefix + "external_api:example.com")
	assert.Equal(t, interval, ttl, "the slot must expire at 7.5s, not at a rounded second")
	assert.NotEqual(t, 7*time.Second, ttl)
	assert.NotEqual(t, 8*time.Second, ttl)

	// And the remaining-time hint the waiter gets back keeps the fraction too.
	mr.FastForward(500 * time.Millisecond)
	acquired, retryAfter, err := store.AcquireSlot(context.Background(), "external_api:example.com", "alt-harvester/1", interval)
	require.NoError(t, err)
	require.False(t, acquired)
	assert.Equal(t, 7*time.Second, retryAfter, "PTTL is millisecond-resolution; 7.5s minus 500ms is exactly 7s")
}
