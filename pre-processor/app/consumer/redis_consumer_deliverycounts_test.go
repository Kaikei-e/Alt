package consumer

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// handledMessageIDs returns the stream IDs the handler was actually invoked
// with, under the handler's own lock.
func handledMessageIDs(h *recordingHandler) []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	ids := make([]string, 0, len(h.events))
	for _, event := range h.events {
		ids = append(ids, event.MessageID)
	}
	return ids
}

// TestReclaimPending_DLQsPoisonMessageDeepInThePEL reproduces the finding:
// deliveryCounts looked the whole batch up with a single XPendingExt sweep
// bounded by Start:"-", End:"+" and Count: len(messages)*2. That window only
// ever covers the oldest few entries of this consumer's PEL, so once the PEL
// is deeper than the window, a message from a later XAUTOCLAIM page is absent
// from the result and its delivery count reads as zero. shouldSendToDLQ then
// never fires for it and every sweep re-runs the poison message's handler --
// a DB round-trip per sweep, forever. See mq-hub/CLAUDE.md rule 5 ("DLQ
// conditions never fire").
func TestReclaimPending_DLQsPoisonMessageDeepInThePEL(t *testing.T) {
	srv := miniredis.RunT(t)

	ctx := context.Background()
	seedClient := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	defer func() { _ = seedClient.Close() }()

	const consumerName = "consumer-a"

	if err := seedClient.XGroupCreateMkStream(ctx, reclaimTestStream, reclaimTestGroup, "0").Err(); err != nil {
		t.Fatalf("seed XGroupCreateMkStream: %v", err)
	}

	// Fill this consumer's PEL with more low-ID entries than the old
	// Count: len(messages)*2 window could ever return, so the poison message
	// seeded after them lands on the third XAUTOCLAIM page.
	const noiseCount = 21
	for i := 0; i < noiseCount; i++ {
		if err := seedClient.XAdd(ctx, &redis.XAddArgs{
			Stream: reclaimTestStream,
			Values: map[string]interface{}{"event_id": fmt.Sprintf("noise-%d", i)},
		}).Err(); err != nil {
			t.Fatalf("seed noise XAdd %d: %v", i, err)
		}
	}

	targetID, err := seedClient.XAdd(ctx, &redis.XAddArgs{
		Stream: reclaimTestStream,
		Values: map[string]interface{}{"event_id": "poison"},
	}).Result()
	if err != nil {
		t.Fatalf("seed poison XAdd: %v", err)
	}

	if _, err := seedClient.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    reclaimTestGroup,
		Consumer: consumerName,
		Streams:  []string{reclaimTestStream, ">"},
		Count:    noiseCount + 1,
	}).Result(); err != nil {
		t.Fatalf("seed XReadGroup: %v", err)
	}

	// Drive only the poison message's delivery counter well past
	// MaxDeliveries; the noise entries stay at one delivery so they keep
	// occupying the front of the PEL instead of draining out of it.
	const extraClaims = 5
	for i := 0; i < extraClaims; i++ {
		if _, err := seedClient.XClaim(ctx, &redis.XClaimArgs{
			Stream:   reclaimTestStream,
			Group:    reclaimTestGroup,
			Consumer: consumerName,
			MinIdle:  0,
			Messages: []string{targetID},
		}).Result(); err != nil {
			t.Fatalf("pump poison XClaim %d: %v", i, err)
		}
	}

	claimIdleTime := 10 * time.Millisecond

	cfg := Config{
		RedisURL:      fmt.Sprintf("redis://%s", srv.Addr()),
		GroupName:     reclaimTestGroup,
		ConsumerName:  consumerName,
		StreamKey:     reclaimTestStream,
		BatchSize:     10,
		BlockTimeout:  time.Second,
		ClaimIdleTime: claimIdleTime,
		DLQStreamKey:  "alt:events:articles:dlq",
		MaxDeliveries: 3,
		Enabled:       true,
	}

	handler := &recordingHandler{err: fmt.Errorf("poison message: always fails")}

	c, err := NewConsumer(cfg, handler, newQuietLogger())
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	defer c.Stop()

	srv.SetTime(time.Now().Add(claimIdleTime + time.Second))

	if err := c.reclaimPending(ctx); err != nil {
		t.Fatalf("reclaimPending: %v", err)
	}

	dlqEntries, err := seedClient.XRange(ctx, cfg.DLQStreamKey, "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange DLQ: %v", err)
	}
	if len(dlqEntries) != 1 {
		t.Fatalf("DLQ stream has %d entries, want 1 (the poison message sits on the third XAUTOCLAIM page, past the truncated XPENDING window)", len(dlqEntries))
	}
	if dlqEntries[0].Values["dlq_original_id"] != targetID {
		t.Fatalf("DLQ entry dlq_original_id = %v, want %q", dlqEntries[0].Values["dlq_original_id"], targetID)
	}

	for _, id := range handledMessageIDs(handler) {
		if id == targetID {
			t.Fatalf("handler was re-invoked for the poison message %q; a message past MaxDeliveries must be DLQ'd, not retried", targetID)
		}
	}

	stillPending, err := seedClient.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: reclaimTestStream,
		Group:  reclaimTestGroup,
		Start:  targetID,
		End:    targetID,
		Count:  1,
	}).Result()
	if err != nil {
		t.Fatalf("XPendingExt: %v", err)
	}
	if len(stillPending) != 0 {
		t.Fatalf("poison message %q is still in the PEL after DLQ routing", targetID)
	}
}
