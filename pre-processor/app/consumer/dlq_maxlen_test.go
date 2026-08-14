package consumer

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestReclaimPending_BoundsDLQStreamLength reproduces the finding: the DLQ
// XADD carried no MAXLEN, and nothing else trims that stream -- mq-hub's
// periodic XTRIM pass only walks domain.AllStreamKeys(), which lists the four
// live streams and no DLQ, and no service consumes a DLQ. redis-streams runs
// maxmemory 1gb under noeviction where XADD is denyoom, so an unbounded DLQ
// full of whole original payloads eventually rejects every producer's publish
// -- and publish-time trimming cannot then shrink anything back, because it
// only runs as part of a successful XADD (compose/mq.yaml records that
// self-locking condition).
func TestReclaimPending_BoundsDLQStreamLength(t *testing.T) {
	srv := miniredis.RunT(t)

	ctx := context.Background()
	seedClient := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	defer func() { _ = seedClient.Close() }()

	if err := seedClient.XGroupCreateMkStream(ctx, reclaimTestStream, reclaimTestGroup, "0").Err(); err != nil {
		t.Fatalf("seed XGroupCreateMkStream: %v", err)
	}

	const poisonCount = 10
	for i := 0; i < poisonCount; i++ {
		if err := seedClient.XAdd(ctx, &redis.XAddArgs{
			Stream: reclaimTestStream,
			Values: map[string]interface{}{
				"event_id": fmt.Sprintf("poison-%d", i),
				"payload":  `{"article_id":"abc"}`,
			},
		}).Err(); err != nil {
			t.Fatalf("seed poison XAdd %d: %v", i, err)
		}
	}

	// Deliver to a ghost consumer that never ACKs, so every entry is already
	// at one delivery when the reclaim sweep claims it.
	if _, err := seedClient.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    reclaimTestGroup,
		Consumer: "ghost-consumer",
		Streams:  []string{reclaimTestStream, ">"},
		Count:    poisonCount,
	}).Result(); err != nil {
		t.Fatalf("seed XReadGroup: %v", err)
	}

	claimIdleTime := 10 * time.Millisecond
	const dlqMaxLen = 3

	cfg := Config{
		RedisURL:      fmt.Sprintf("redis://%s", srv.Addr()),
		GroupName:     reclaimTestGroup,
		ConsumerName:  "consumer-a",
		StreamKey:     reclaimTestStream,
		BatchSize:     poisonCount,
		BlockTimeout:  time.Second,
		ClaimIdleTime: claimIdleTime,
		DLQStreamKey:  "alt:events:articles:dlq",
		MaxDeliveries: 1,
		DLQMaxLen:     dlqMaxLen,
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

	// miniredis trims exactly and ignores the "~" modifier; real Redis keeps
	// at least the cap. Either way the point is that a cap is attached at all,
	// so the DLQ cannot grow without bound.
	dlqLen, err := seedClient.XLen(ctx, cfg.DLQStreamKey).Result()
	if err != nil {
		t.Fatalf("XLen DLQ: %v", err)
	}
	if dlqLen == 0 {
		t.Fatalf("DLQ stream is empty: the %d poison messages were never routed", poisonCount)
	}
	if dlqLen > dlqMaxLen {
		t.Fatalf("DLQ stream length = %d after routing %d poison messages, want <= %d (the XADD carries no MAXLEN, so nothing bounds this stream)", dlqLen, poisonCount, dlqMaxLen)
	}
}

// TestDLQMaxLen_UnsetConfigStillBounded pins the fallback: bootstrap/wire.go
// builds Config as a struct literal, so a field it does not know about stays
// zero. A zero cap must resolve to the package default rather than to
// "unbounded" -- an unbounded DLQ is the failure mode this whole cap exists to
// prevent, so it must never be reachable by forgetting to set a field.
func TestDLQMaxLen_UnsetConfigStillBounded(t *testing.T) {
	t.Parallel()

	if got := (Config{}).effectiveDLQMaxLen(); got != defaultDLQMaxLen {
		t.Fatalf("Config{}.effectiveDLQMaxLen() = %d, want %d", got, defaultDLQMaxLen)
	}
	if got := (Config{DLQMaxLen: 42}).effectiveDLQMaxLen(); got != 42 {
		t.Fatalf("Config{DLQMaxLen: 42}.effectiveDLQMaxLen() = %d, want 42", got)
	}
	if cfg := DefaultConfig(); cfg.DLQMaxLen <= 0 {
		t.Fatalf("DefaultConfig().DLQMaxLen = %d, want a positive cap", cfg.DLQMaxLen)
	}
}
