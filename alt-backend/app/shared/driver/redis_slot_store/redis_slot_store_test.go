package redis_slot_store

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return New(client), mr
}

func TestStore_AcquireSlot(t *testing.T) {
	t.Run("the first caller in the interval wins", func(t *testing.T) {
		store, mr := newTestStore(t)

		acquired, retryAfter, err := store.AcquireSlot(context.Background(), "external_api:example.com", "alt-backend/1", 5*time.Second)
		require.NoError(t, err)
		assert.True(t, acquired)
		assert.Zero(t, retryAfter)

		// The owner is written for debuggability: GET on a contended key must
		// answer "which process is holding this host".
		got, err := mr.Get(KeyPrefix + "external_api:example.com")
		require.NoError(t, err)
		assert.Equal(t, "alt-backend/1", got)
	})

	t.Run("a second caller is told how long is left", func(t *testing.T) {
		store, _ := newTestStore(t)

		acquired, _, err := store.AcquireSlot(context.Background(), "external_api:example.com", "alt-harvester/1", 5*time.Second)
		require.NoError(t, err)
		require.True(t, acquired)

		acquired, retryAfter, err := store.AcquireSlot(context.Background(), "external_api:example.com", "alt-backend/1", 5*time.Second)
		require.NoError(t, err)
		assert.False(t, acquired, "the same host must not be handed to two processes inside one interval")
		assert.Greater(t, retryAfter, time.Duration(0))
		assert.LessOrEqual(t, retryAfter, 5*time.Second)
	})

	t.Run("the slot frees itself when the TTL runs out", func(t *testing.T) {
		store, mr := newTestStore(t)

		acquired, _, err := store.AcquireSlot(context.Background(), "external_api:example.com", "alt-harvester/1", 5*time.Second)
		require.NoError(t, err)
		require.True(t, acquired)

		// No explicit release exists on purpose: a process that crashes
		// mid-fetch must not hold a host forever.
		mr.FastForward(5 * time.Second)

		acquired, _, err = store.AcquireSlot(context.Background(), "external_api:example.com", "alt-backend/1", 5*time.Second)
		require.NoError(t, err)
		assert.True(t, acquired)
	})

	t.Run("different namespaces do not share a slot", func(t *testing.T) {
		store, _ := newTestStore(t)

		acquired, _, err := store.AcquireSlot(context.Background(), "external_api:cdn.example.com", "alt-backend/1", 5*time.Second)
		require.NoError(t, err)
		require.True(t, acquired)

		// The image proxy runs at 1s against the same CDN host; a 5s
		// external-api slot must not throttle it, and its 1s slot must not
		// satisfy the external-api promise.
		acquired, _, err = store.AcquireSlot(context.Background(), "image_proxy:cdn.example.com", "alt-backend/1", time.Second)
		require.NoError(t, err)
		assert.True(t, acquired)
	})

	t.Run("an unreachable server surfaces as an error, not a free slot", func(t *testing.T) {
		store, mr := newTestStore(t)
		mr.Close()

		acquired, _, err := store.AcquireSlot(context.Background(), "external_api:example.com", "alt-backend/1", 5*time.Second)
		require.Error(t, err, "a store that cannot answer must say so; the caller decides to degrade")
		assert.False(t, acquired)
	})
}

func TestNewFromURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{name: "redis url", rawURL: "redis://redis-streams:6379/3"},
		{name: "empty url is a caller bug, not a silent local mode", rawURL: "", wantErr: true},
		{name: "garbage url fails at startup", rawURL: "://nope", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewFromURL(tt.rawURL)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, store)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, store)
			t.Cleanup(func() { _ = store.Close() })
		})
	}
}
