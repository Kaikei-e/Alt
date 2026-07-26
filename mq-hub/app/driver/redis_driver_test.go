package driver

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mq-hub/domain"
)

// xaddCaptureHook intercepts XADD commands and records their raw arguments
// instead of forwarding them to the server. miniredis (github.com/alicebob/miniredis/v2)
// does not implement the Redis 8.2+ XADD MODE grammar (ACKED/KEEPREF/DELREF) and
// misparses a MODE-bearing XADD as an invalid stream ID, so tests that need to
// verify the constructed command use this hook rather than a real round trip.
type xaddCaptureHook struct {
	captured [][]interface{}
}

func (h *xaddCaptureHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h *xaddCaptureHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() != "xadd" {
			return next(ctx, cmd)
		}
		h.captured = append(h.captured, cmd.Args())
		if strCmd, ok := cmd.(*redis.StringCmd); ok {
			strCmd.SetVal("0-0")
		}
		return nil
	}
}

func (h *xaddCaptureHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		passthrough := make([]redis.Cmder, 0, len(cmds))
		for _, cmd := range cmds {
			if cmd.Name() != "xadd" {
				passthrough = append(passthrough, cmd)
				continue
			}
			h.captured = append(h.captured, cmd.Args())
			if strCmd, ok := cmd.(*redis.StringCmd); ok {
				strCmd.SetVal("0-0")
			}
		}
		if len(passthrough) > 0 {
			return next(ctx, passthrough)
		}
		return nil
	}
}

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

// pipelinePartialFailHook lets an individual XADD command inside a pipeline
// succeed against miniredis as normal, but then overrides its Cmder.Err() to
// simulate the case RedisDriver.PublishBatch exists to handle: Exec()
// reports no pipeline-level error, yet one specific command's reply was an
// error (e.g. a per-key limit rejection). See RedisDriver.PublishBatch doc:
// Exec() alone cannot tell callers which XADD failed, only inspecting each
// cmd.Err() can.
type pipelinePartialFailHook struct {
	failEventID string
}

func (h *pipelinePartialFailHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *pipelinePartialFailHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return next
}

func (h *pipelinePartialFailHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		err := next(ctx, cmds)
		for _, cmd := range cmds {
			if cmd.Name() != "xadd" {
				continue
			}
			args := cmd.Args()
			for i, a := range args {
				if s, ok := a.(string); ok && s == "event_id" && i+1 < len(args) && args[i+1] == h.failEventID {
					if strCmd, ok := cmd.(*redis.StringCmd); ok {
						strCmd.SetErr(fmt.Errorf("simulated per-entry rejection for %s", h.failEventID))
					}
				}
			}
		}
		return err
	}
}

func TestRedisDriver_PublishBatch_PartialFailure(t *testing.T) {
	t.Run("reports exactly which index failed without losing the others' message IDs", func(t *testing.T) {
		mr := NewMiniredis(t)
		driver, err := NewRedisDriver(mr.Addr())
		require.NoError(t, err)
		defer func() {
			driver.Close()
			mr.Close()
		}()

		driver.client.AddHook(&pipelinePartialFailHook{failEventID: "evt-1"})

		ctx := context.Background()
		events := []*domain.Event{
			{EventID: "evt-0", EventType: domain.EventTypeArticleCreated, Source: "test", CreatedAt: time.Now()},
			{EventID: "evt-1", EventType: domain.EventTypeArticleCreated, Source: "test", CreatedAt: time.Now()},
			{EventID: "evt-2", EventType: domain.EventTypeArticleCreated, Source: "test", CreatedAt: time.Now()},
		}

		messageIDs, err := driver.PublishBatch(ctx, domain.StreamKeyArticles, events)

		require.Error(t, err)
		var partialErr *domain.PartialPublishError
		require.ErrorAs(t, err, &partialErr)
		assert.Equal(t, 3, partialErr.TotalEvents)
		require.Len(t, partialErr.Failures, 1)
		assert.Equal(t, 1, partialErr.Failures[0].Index)
		assert.Contains(t, partialErr.Failures[0].Err.Error(), "evt-1")

		require.Len(t, messageIDs, 3)
		assert.NotEmpty(t, messageIDs[0], "index 0 succeeded and must keep its message ID")
		assert.Empty(t, messageIDs[1], "index 1 failed and must not report a fabricated message ID")
		assert.NotEmpty(t, messageIDs[2], "index 2 succeeded and must keep its message ID")
	})
}

func TestRedisDriver_Publish_WithMaxLen(t *testing.T) {
	t.Run("trims approximately via MAXLEN in ACKED mode", func(t *testing.T) {
		mr := NewMiniredis(t)
		driver, err := NewRedisDriverWithOptions(mr.Addr(), &RedisDriverOptions{
			StreamMaxLen: 5,
		})
		require.NoError(t, err)
		defer func() {
			driver.Close()
			mr.Close()
		}()

		hook := &xaddCaptureHook{}
		driver.client.AddHook(hook)

		ctx := context.Background()
		event := &domain.Event{
			EventID:   "evt-0",
			EventType: domain.EventTypeArticleCreated,
			Source:    "test",
			CreatedAt: time.Now(),
		}
		_, err = driver.Publish(ctx, domain.StreamKeyArticles, event)
		require.NoError(t, err)

		require.Len(t, hook.captured, 1)
		args := hook.captured[0]
		assert.Contains(t, args, "maxlen")
		assert.Contains(t, args, "~")
		assert.Contains(t, args, int64(5))
		// MODE ACKED trims only entries every consumer group has read and
		// acked, so a stalled/backlogged consumer's unread entries are never
		// silently evicted by MAXLEN trimming.
		assert.Contains(t, args, "ACKED")
	})

	t.Run("no trimming when StreamMaxLen is 0", func(t *testing.T) {
		mr := NewMiniredis(t)
		driver, err := NewRedisDriver(mr.Addr())
		require.NoError(t, err)
		defer func() {
			driver.Close()
			mr.Close()
		}()

		ctx := context.Background()
		for i := 0; i < 10; i++ {
			event := &domain.Event{
				EventID:   fmt.Sprintf("evt-%d", i),
				EventType: domain.EventTypeArticleCreated,
				Source:    "test",
				CreatedAt: time.Now(),
			}
			_, err := driver.Publish(ctx, domain.StreamKeyArticles, event)
			require.NoError(t, err)
		}

		info, err := driver.GetStreamInfo(ctx, domain.StreamKeyArticles)
		require.NoError(t, err)
		assert.Equal(t, int64(10), info.Length)
	})
}

func TestRedisDriver_PublishBatch_WithMaxLen(t *testing.T) {
	t.Run("trims approximately via MAXLEN in ACKED mode", func(t *testing.T) {
		mr := NewMiniredis(t)
		driver, err := NewRedisDriverWithOptions(mr.Addr(), &RedisDriverOptions{
			StreamMaxLen: 5,
		})
		require.NoError(t, err)
		defer func() {
			driver.Close()
			mr.Close()
		}()

		hook := &xaddCaptureHook{}
		driver.client.AddHook(hook)

		ctx := context.Background()
		events := make([]*domain.Event, 10)
		for i := 0; i < 10; i++ {
			events[i] = &domain.Event{
				EventID:   fmt.Sprintf("evt-%d", i),
				EventType: domain.EventTypeArticleCreated,
				Source:    "test",
				CreatedAt: time.Now(),
			}
		}

		_, err = driver.PublishBatch(ctx, domain.StreamKeyArticles, events)
		require.NoError(t, err)

		require.Len(t, hook.captured, 10)
		for _, args := range hook.captured {
			assert.Contains(t, args, "maxlen")
			assert.Contains(t, args, "~")
			assert.Contains(t, args, int64(5))
			assert.Contains(t, args, "ACKED")
		}
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

	t.Run("returns error when Redis is unavailable", func(t *testing.T) {
		mr := NewMiniredis(t)
		driver, err := NewRedisDriver(mr.Addr())
		require.NoError(t, err)
		defer driver.Close()

		// Shut Redis down so the connection genuinely fails, instead of
		// mocking a "not connected" state. HealthCheck's "unhealthy" path
		// depends on this actually surfacing an error. Bound the wait with a
		// short ctx deadline so the test doesn't pay for go-redis's full
		// dial-retry backoff.
		mr.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()

		err = driver.Ping(ctx)

		require.Error(t, err)
	})
}

// TestRedisDriver_SubscribeWithTimeout exercises the XREAD-based reply path,
// including parseEventFromMessage's field parsing, which no other test
// covers.
func TestRedisDriver_SubscribeWithTimeout(t *testing.T) {
	t.Run("receives and parses a published event", func(t *testing.T) {
		driver, cleanup := setupTestDriver(t)
		defer cleanup()

		ctx := context.Background()
		event := &domain.Event{
			EventID:   "reply-evt-1",
			EventType: domain.EventTypeTagGenerationCompleted,
			Source:    "tag-generator",
			CreatedAt: time.Now().Truncate(time.Millisecond),
			Payload:   []byte(`{"success": true}`),
			Metadata:  map[string]string{"correlation_id": "corr-1"},
		}
		replyStream := domain.StreamKey("alt:replies:tags:corr-1")
		_, err := driver.Publish(ctx, replyStream, event)
		require.NoError(t, err)

		got, err := driver.SubscribeWithTimeout(ctx, replyStream, 2*time.Second)

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, event.EventID, got.EventID)
		assert.Equal(t, event.EventType, got.EventType)
		assert.Equal(t, event.Source, got.Source)
		assert.Equal(t, event.CreatedAt.UTC(), got.CreatedAt.UTC())
		assert.Equal(t, event.Payload, got.Payload)
		assert.Equal(t, "corr-1", got.Metadata["correlation_id"])
	})

	t.Run("returns ErrReplyTimeout when no message arrives before the deadline", func(t *testing.T) {
		driver, cleanup := setupTestDriver(t)
		defer cleanup()

		ctx := context.Background()
		start := time.Now()

		_, err := driver.SubscribeWithTimeout(ctx, domain.StreamKey("alt:replies:tags:never-published"), 200*time.Millisecond)

		elapsed := time.Since(start)
		require.ErrorIs(t, err, domain.ErrReplyTimeout)
		assert.GreaterOrEqual(t, elapsed, 200*time.Millisecond)
	})
}

func TestRedisDriver_DeleteStream(t *testing.T) {
	t.Run("removes an existing stream", func(t *testing.T) {
		driver, cleanup := setupTestDriver(t)
		defer cleanup()

		ctx := context.Background()
		event := &domain.Event{
			EventID:   "evt-1",
			EventType: domain.EventTypeArticleCreated,
			Source:    "test",
			CreatedAt: time.Now(),
		}
		_, err := driver.Publish(ctx, domain.StreamKeyArticles, event)
		require.NoError(t, err)

		err = driver.DeleteStream(ctx, domain.StreamKeyArticles)
		require.NoError(t, err)

		_, err = driver.GetStreamInfo(ctx, domain.StreamKeyArticles)
		require.Error(t, err, "stream must actually be gone after DeleteStream")
	})

	t.Run("is a no-op when the stream does not exist", func(t *testing.T) {
		driver, cleanup := setupTestDriver(t)
		defer cleanup()

		err := driver.DeleteStream(context.Background(), domain.StreamKey("alt:replies:tags:never-existed"))
		require.NoError(t, err)
	})
}

func TestRedisDriver_Expire(t *testing.T) {
	t.Run("sets a TTL on an existing stream", func(t *testing.T) {
		mr := NewMiniredis(t)
		driver, err := NewRedisDriver(mr.Addr())
		require.NoError(t, err)
		defer func() {
			driver.Close()
			mr.Close()
		}()

		ctx := context.Background()
		event := &domain.Event{
			EventID:   "evt-1",
			EventType: domain.EventTypeArticleCreated,
			Source:    "test",
			CreatedAt: time.Now(),
		}
		_, err = driver.Publish(ctx, domain.StreamKeyArticles, event)
		require.NoError(t, err)

		err = driver.Expire(ctx, domain.StreamKeyArticles, 5*time.Minute)
		require.NoError(t, err)

		ttl := mr.TTL(domain.StreamKeyArticles.String())
		assert.Greater(t, ttl, time.Duration(0), "TTL must actually be set on the key, not silently ignored")
		assert.LessOrEqual(t, ttl, 5*time.Minute)
	})

	t.Run("is a no-op when the key does not exist yet", func(t *testing.T) {
		driver, cleanup := setupTestDriver(t)
		defer cleanup()

		err := driver.Expire(context.Background(), domain.StreamKey("alt:replies:tags:never-existed"), 5*time.Minute)
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
