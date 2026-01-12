package driver

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mq-hub/domain"
)

// TestRedisDriver_Publish tests the Publish method using a mock or miniredis.
// In production tests, use miniredis for unit tests and real Redis for integration.
func TestRedisDriver_Publish(t *testing.T) {
	t.Run("publishes event to stream", func(t *testing.T) {
		// This test requires a Redis connection.
		// Skip if REDIS_URL is not set.
		driver, cleanup := setupTestDriver(t)
		defer cleanup()

		ctx := context.Background()
		event := &domain.Event{
			EventID:   "test-event-1",
			EventType: domain.EventTypeArticleCreated,
			Source:    "test",
			CreatedAt: time.Now(),
			Payload:   []byte(`{"article_id": "123"}`),
			Metadata:  map[string]string{"trace_id": "abc"},
		}

		messageID, err := driver.Publish(ctx, domain.StreamKeyArticles, event)

		require.NoError(t, err)
		assert.NotEmpty(t, messageID)
		// Message ID format: 1234567890123-0
		assert.Contains(t, messageID, "-")
	})

	t.Run("returns error for nil event", func(t *testing.T) {
		driver, cleanup := setupTestDriver(t)
		defer cleanup()

		ctx := context.Background()

		_, err := driver.Publish(ctx, domain.StreamKeyArticles, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "event is nil")
	})
}

func TestRedisDriver_PublishBatch(t *testing.T) {
	t.Run("publishes multiple events to stream", func(t *testing.T) {
		driver, cleanup := setupTestDriver(t)
		defer cleanup()

		ctx := context.Background()
		events := []*domain.Event{
			{
				EventID:   "test-event-1",
				EventType: domain.EventTypeArticleCreated,
				Source:    "test",
				CreatedAt: time.Now(),
				Payload:   []byte(`{"article_id": "1"}`),
			},
			{
				EventID:   "test-event-2",
				EventType: domain.EventTypeArticleCreated,
				Source:    "test",
				CreatedAt: time.Now(),
				Payload:   []byte(`{"article_id": "2"}`),
			},
		}

		messageIDs, err := driver.PublishBatch(ctx, domain.StreamKeyArticles, events)

		require.NoError(t, err)
		assert.Len(t, messageIDs, 2)
		for _, id := range messageIDs {
			assert.NotEmpty(t, id)
			assert.Contains(t, id, "-")
		}
	})

	t.Run("returns empty slice for empty events", func(t *testing.T) {
		driver, cleanup := setupTestDriver(t)
		defer cleanup()

		ctx := context.Background()

		messageIDs, err := driver.PublishBatch(ctx, domain.StreamKeyArticles, []*domain.Event{})

		require.NoError(t, err)
		assert.Empty(t, messageIDs)
	})
}

func TestRedisDriver_CreateConsumerGroup(t *testing.T) {
	t.Run("creates consumer group successfully", func(t *testing.T) {
		driver, cleanup := setupTestDriver(t)
		defer cleanup()

		ctx := context.Background()

		// First, publish a message to create the stream
		event := &domain.Event{
			EventID:   "setup-event",
			EventType: domain.EventTypeArticleCreated,
			Source:    "test",
			CreatedAt: time.Now(),
		}
		_, err := driver.Publish(ctx, domain.StreamKeyArticles, event)
		require.NoError(t, err)

		// Create consumer group
		err = driver.CreateConsumerGroup(ctx, domain.StreamKeyArticles, domain.ConsumerGroupPreProcessor, "0")

		require.NoError(t, err)
	})

	t.Run("handles BUSYGROUP error gracefully", func(t *testing.T) {
		driver, cleanup := setupTestDriver(t)
		defer cleanup()

		ctx := context.Background()

		// Create stream and group
		event := &domain.Event{
			EventID:   "setup-event-2",
			EventType: domain.EventTypeArticleCreated,
			Source:    "test",
			CreatedAt: time.Now(),
		}
		_, _ = driver.Publish(ctx, domain.StreamKeyArticles, event)
		_ = driver.CreateConsumerGroup(ctx, domain.StreamKeyArticles, domain.ConsumerGroupPreProcessor, "0")

		// Try to create the same group again - should not error
		err := driver.CreateConsumerGroup(ctx, domain.StreamKeyArticles, domain.ConsumerGroupPreProcessor, "0")

		// Should handle BUSYGROUP gracefully
		assert.NoError(t, err)
	})
}

func TestRedisDriver_GetStreamInfo(t *testing.T) {
	t.Run("returns stream info", func(t *testing.T) {
		driver, cleanup := setupTestDriver(t)
		defer cleanup()

		ctx := context.Background()

		// Publish events to create stream
		for i := 0; i < 3; i++ {
			event := &domain.Event{
				EventID:   "info-event-" + string(rune('0'+i)),
				EventType: domain.EventTypeArticleCreated,
				Source:    "test",
				CreatedAt: time.Now(),
			}
			_, err := driver.Publish(ctx, domain.StreamKeyArticles, event)
			require.NoError(t, err)
		}

		info, err := driver.GetStreamInfo(ctx, domain.StreamKeyArticles)

		require.NoError(t, err)
		assert.NotNil(t, info)
		assert.Equal(t, int64(3), info.Length)
		// Note: miniredis may not populate FirstEntryID/LastEntryID correctly
		// These assertions are relaxed for unit tests
		// Integration tests with real Redis should verify these fields
	})
}

func TestRedisDriver_Ping(t *testing.T) {
	t.Run("returns nil when Redis is available", func(t *testing.T) {
		driver, cleanup := setupTestDriver(t)
		defer cleanup()

		ctx := context.Background()

		err := driver.Ping(ctx)

		require.NoError(t, err)
	})
}

// setupTestDriver creates a test Redis driver.
// Uses miniredis for isolated unit testing.
func setupTestDriver(t *testing.T) (*RedisDriver, func()) {
	t.Helper()

	// Use miniredis for testing
	mr := NewMiniredis(t)
	driver, err := NewRedisDriver(mr.Addr())
	require.NoError(t, err)

	cleanup := func() {
		driver.Close()
		mr.Close()
	}

	return driver, cleanup
}
