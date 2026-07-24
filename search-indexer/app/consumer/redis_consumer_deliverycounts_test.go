package consumer

import (
	"context"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestDeliveryCounts_CoversHighIDMessageBeyondLowIDPELWindow reproduces the
// HIGH finding: deliveryCounts queries XPendingExt with Start:"-", End:"+"
// and Count: len(messages)*2, which -- for a consumer whose PEL has
// accumulated more entries than that -- returns only the earliest
// (lowest-ID) entries. Any message in the batch being looked up whose ID
// falls outside that truncated window is silently missing from the returned
// map, so callers read a zero delivery count for it regardless of its real
// value. See .claude/rules/event-stream-consumer.md ("DLQ条件は再配信が起きて
// 初めて発火する").
func TestDeliveryCounts_CoversHighIDMessageBeyondLowIDPELWindow(t *testing.T) {
	srv := miniredis.RunT(t)

	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	defer rdb.Close()

	const stream = "alt:events:articles"
	const group = "search-indexer-group"
	const consumerName = "consumer-a"

	if err := rdb.XGroupCreateMkStream(ctx, stream, group, "0").Err(); err != nil {
		t.Fatalf("XGroupCreateMkStream: %v", err)
	}

	// Seed three low-ID "noise" messages, delivered once each directly to
	// consumerName so they occupy the front of its PEL (delivery count 1,
	// never reclaimed again).
	for i := 0; i < 3; i++ {
		id, err := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: stream,
			Values: map[string]interface{}{"event_id": fmt.Sprintf("noise-%d", i)},
		}).Result()
		if err != nil {
			t.Fatalf("seed noise XAdd: %v", err)
		}
		if _, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: consumerName,
			Streams:  []string{stream, ">"},
			Count:    1,
		}).Result(); err != nil {
			t.Fatalf("seed noise XReadGroup: %v", err)
		}
		_ = id
	}

	// Seed the target message with a higher stream ID, also delivered to
	// consumerName, then reclaim it repeatedly via XClaim (MinIdle 0) so its
	// real delivery count climbs well past the noise messages' count of 1.
	targetID, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{"event_id": "target"},
	}).Result()
	if err != nil {
		t.Fatalf("seed target XAdd: %v", err)
	}
	if _, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumerName,
		Streams:  []string{stream, ">"},
		Count:    1,
	}).Result(); err != nil {
		t.Fatalf("seed target XReadGroup: %v", err)
	}

	const extraClaims = 5
	for i := 0; i < extraClaims; i++ {
		if _, err := rdb.XClaim(ctx, &redis.XClaimArgs{
			Stream:   stream,
			Group:    group,
			Consumer: consumerName,
			MinIdle:  0,
			Messages: []string{targetID},
		}).Result(); err != nil {
			t.Fatalf("reclaim target XClaim %d: %v", i, err)
		}
	}
	wantCount := int64(1 + extraClaims) // 1 initial delivery + extraClaims reclaims

	cfg := Config{
		RedisURL:     fmt.Sprintf("redis://%s", srv.Addr()),
		GroupName:    group,
		ConsumerName: consumerName,
		StreamKey:    stream,
		Enabled:      true,
	}
	c, err := NewConsumer(cfg, &recordingHandler{}, newQuietLogger())
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	defer c.Stop()

	counts := c.deliveryCounts(ctx, []redis.XMessage{{ID: targetID}})

	got, ok := counts[targetID]
	if !ok || got != wantCount {
		t.Fatalf("deliveryCounts()[target] = (value=%d, ok=%v), want (%d, true) -- the low-ID noise messages crowded the target out of the truncated XPendingExt window (real delivery count via redis: %d)",
			got, ok, wantCount, wantCount)
	}
}
