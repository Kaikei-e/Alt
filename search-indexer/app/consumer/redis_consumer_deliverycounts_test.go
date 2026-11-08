package consumer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type xpendingFailHook struct {
	mu  sync.Mutex
	err error
}

func (h *xpendingFailHook) setErr(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.err = err
}

func (h *xpendingFailHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h *xpendingFailHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if strings.EqualFold(cmd.Name(), "xpending") {
			h.mu.Lock()
			err := h.err
			h.mu.Unlock()
			if err != nil {
				cmd.SetErr(err)
				return err
			}
		}
		return next(ctx, cmd)
	}
}

func (h *xpendingFailHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

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

func TestReclaimPending_SkipsMessageWhenDeliveryCountLookupFails(t *testing.T) {
	srv := miniredis.RunT(t)

	ctx := context.Background()
	seedClient := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	defer seedClient.Close()

	msgID := seedStuckPendingMessage(t, ctx, seedClient)

	const consumerName = "consumer-a"
	for i := 0; i < 2; i++ {
		if _, err := seedClient.XClaim(ctx, &redis.XClaimArgs{
			Stream:   reclaimTestStream,
			Group:    reclaimTestGroup,
			Consumer: consumerName,
			MinIdle:  0,
			Messages: []string{msgID},
		}).Result(); err != nil {
			t.Fatalf("seed XClaim %d: %v", i, err)
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
		MaxDeliveries: 2,
		Enabled:       true,
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	handler := &recordingHandler{}

	c, err := NewConsumer(cfg, handler, logger)
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	defer c.Stop()

	hook := &xpendingFailHook{}
	hook.setErr(errors.New("simulated XPendingExt failure"))
	c.client.AddHook(hook)

	virtualNow := time.Now().Add(claimIdleTime + time.Second)
	srv.SetTime(virtualNow)

	if err := c.reclaimPending(ctx); err != nil {
		t.Fatalf("reclaimPending with XPendingExt error: %v", err)
	}

	if got := handler.count(); got != 0 {
		t.Fatalf("handler invoked %d times when XPendingExt failed, want 0 (message with unknown delivery count must not be processed as retry count 0)", got)
	}

	dlqEntries, err := seedClient.XRange(ctx, cfg.DLQStreamKey, "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange DLQ (after failed lookup): %v", err)
	}
	if len(dlqEntries) != 0 {
		t.Fatalf("DLQ stream has %d entries when XPendingExt failed, want 0", len(dlqEntries))
	}

	pending, err := seedClient.XPending(ctx, reclaimTestStream, reclaimTestGroup).Result()
	if err != nil {
		t.Fatalf("XPending (after failed lookup): %v", err)
	}
	if pending.Count != 1 {
		t.Fatalf("PEL has %d pending entries after failed lookup, want 1 (message must remain pending, not ACKed)", pending.Count)
	}

	if !strings.Contains(logBuf.String(), "level=ERROR") || !strings.Contains(logBuf.String(), "failed to look up delivery count") {
		t.Fatalf("expected ERROR log for failed delivery-count lookup, got logs: %s", logBuf.String())
	}

	// Once XPendingExt succeeds on the next reclaim pass, the message (now well
	// past MaxDeliveries) must be routed to the DLQ and ACKed as before.
	hook.setErr(nil)
	virtualNow = virtualNow.Add(claimIdleTime + time.Second)
	srv.SetTime(virtualNow)

	if err := c.reclaimPending(ctx); err != nil {
		t.Fatalf("reclaimPending after XPendingExt recovery: %v", err)
	}

	if got := handler.count(); got != 0 {
		t.Fatalf("handler invoked %d times after XPendingExt recovery for poison message, want 0", got)
	}

	dlqEntries, err = seedClient.XRange(ctx, cfg.DLQStreamKey, "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange DLQ (after recovery): %v", err)
	}
	if len(dlqEntries) != 1 {
		t.Fatalf("DLQ stream has %d entries after XPendingExt recovery, want 1", len(dlqEntries))
	}
	if dlqEntries[0].Values["dlq_original_id"] != msgID {
		t.Fatalf("DLQ entry dlq_original_id = %v, want %q", dlqEntries[0].Values["dlq_original_id"], msgID)
	}

	pending, err = seedClient.XPending(ctx, reclaimTestStream, reclaimTestGroup).Result()
	if err != nil {
		t.Fatalf("XPending (after recovery): %v", err)
	}
	if pending.Count != 0 {
		t.Fatalf("PEL still has %d pending entries after DLQ routing, want 0", pending.Count)
	}
}
