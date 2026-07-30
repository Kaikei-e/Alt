package driver

import (
	"context"
	"strconv"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mq-hub/domain"
)

func TestRedisDriverTrimMaxLenApprox(t *testing.T) {
	const stream = domain.StreamKeyArticles

	newDriver := func(t *testing.T) *RedisDriver {
		t.Helper()
		mr := NewMiniredis(t)
		t.Cleanup(mr.Close)
		d, err := NewRedisDriver(mr.Addr())
		require.NoError(t, err)
		return d
	}

	seed := func(t *testing.T, d *RedisDriver, n int) {
		t.Helper()
		for i := range n {
			err := d.client.XAdd(context.Background(), &redis.XAddArgs{
				Stream: stream.String(),
				Values: map[string]any{"i": strconv.Itoa(i)},
			}).Err()
			require.NoError(t, err)
		}
	}

	xlen := func(t *testing.T, d *RedisDriver, key domain.StreamKey) int64 {
		t.Helper()
		length, err := d.client.XLen(context.Background(), key.String()).Result()
		require.NoError(t, err)
		return length
	}

	t.Run("brings an oversized stream back under the cap", func(t *testing.T) {
		d := newDriver(t)
		seed(t, d, 40)

		deleted, err := d.TrimMaxLenApprox(context.Background(), stream, 10)

		require.NoError(t, err)
		assert.Positive(t, deleted, "an oversized stream must lose entries")
		assert.Equal(t, int64(40), xlen(t, d, stream)+deleted, "every entry is either kept or counted as deleted")
	})

	t.Run("leaves a stream that is already under the cap alone", func(t *testing.T) {
		d := newDriver(t)
		seed(t, d, 5)

		deleted, err := d.TrimMaxLenApprox(context.Background(), stream, 1000)

		require.NoError(t, err)
		assert.Zero(t, deleted)
		assert.Equal(t, int64(5), xlen(t, d, stream))
	})

	// Trimming a stream that was never created must not look like a failure:
	// the maintenance pass runs over every known key, including idle ones.
	t.Run("a missing stream is not an error", func(t *testing.T) {
		d := newDriver(t)

		deleted, err := d.TrimMaxLenApprox(context.Background(), domain.StreamKeyIndex, 100)

		require.NoError(t, err)
		assert.Zero(t, deleted)
	})
}
