package driver

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mq-hub/domain"
)

// TestRedisDriverScanReplyStreamsWithoutTTL locks in the safety-net sweep that
// bounds temporary request-reply streams (ReplyStreamPrefix + correlationID).
//
// GenerateTagsForArticle's own cleanup (Expire+Delete) runs on a detached
// context, so it no longer no-ops on request timeout -- but a worker that
// replies LATE (XADD after that cleanup already deleted the key) recreates the
// stream with NO expiry, and the length-cap trim pass only covers the fixed
// AllStreamKeys(), never these per-correlation keys. Such a key would then live
// forever. This scan surfaces exactly those keys so the sweep can re-apply a TTL.
func TestRedisDriverScanReplyStreamsWithoutTTL(t *testing.T) {
	const prefix = "alt:replies:tags:"

	newDriver := func(t *testing.T) *RedisDriver {
		t.Helper()
		mr := NewMiniredis(t)
		t.Cleanup(mr.Close)
		d, err := NewRedisDriver(mr.Addr())
		require.NoError(t, err)
		return d
	}

	xadd := func(t *testing.T, d *RedisDriver, key string) {
		t.Helper()
		err := d.client.XAdd(context.Background(), &redis.XAddArgs{
			Stream: key,
			Values: map[string]any{"reply": "1"},
		}).Err()
		require.NoError(t, err)
	}

	t.Run("returns reply-stream keys that have no expiry", func(t *testing.T) {
		d := newDriver(t)
		// A late-reply-recreated reply stream: exists, no TTL.
		xadd(t, d, prefix+"corr-leaked")

		keys, err := d.ScanReplyStreamsWithoutTTL(context.Background(), prefix)

		require.NoError(t, err)
		assert.Equal(t, []domain.StreamKey{domain.StreamKey(prefix + "corr-leaked")}, keys)
	})

	t.Run("skips reply streams that already have a TTL", func(t *testing.T) {
		d := newDriver(t)
		xadd(t, d, prefix+"corr-bounded")
		require.NoError(t, d.client.Expire(context.Background(), prefix+"corr-bounded", 5*time.Minute).Err())

		keys, err := d.ScanReplyStreamsWithoutTTL(context.Background(), prefix)

		require.NoError(t, err)
		assert.Empty(t, keys)
	})

	t.Run("ignores keys outside the reply-stream prefix", func(t *testing.T) {
		d := newDriver(t)
		xadd(t, d, domain.StreamKeyArticles.String()) // a real, permanent stream
		xadd(t, d, prefix+"corr-leaked")

		keys, err := d.ScanReplyStreamsWithoutTTL(context.Background(), prefix)

		require.NoError(t, err)
		assert.Equal(t, []domain.StreamKey{domain.StreamKey(prefix + "corr-leaked")}, keys)
	})

	t.Run("returns nothing when there are no reply streams", func(t *testing.T) {
		d := newDriver(t)

		keys, err := d.ScanReplyStreamsWithoutTTL(context.Background(), prefix)

		require.NoError(t, err)
		assert.Empty(t, keys)
	})
}
