package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// recordingHandler records every event it receives and always succeeds, so a
// successfully-reclaimed message should be handed back for reprocessing.
type recordingHandler struct {
	mu     sync.Mutex
	events []Event
	err    error
}

func (h *recordingHandler) HandleEvent(_ context.Context, event Event) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, event)
	return h.err
}

func (h *recordingHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.events)
}

const reclaimTestStream = "alt:events:articles"
const reclaimTestGroup = "search-indexer-group"

// seedStuckPendingMessage adds one message to the stream and delivers it to a
// "ghost" consumer via XReadGroup without ever ACKing it -- simulating a
// consumer that crashed after XREADGROUP but before finishing/ACKing the
// message. It returns the delivered message ID.
func seedStuckPendingMessage(t *testing.T, ctx context.Context, rdb *redis.Client) string {
	t.Helper()

	id, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: reclaimTestStream,
		Values: map[string]interface{}{
			"event_id":   "evt-1",
			"event_type": "IndexArticle",
			"source":     "test",
			"payload":    `{"article_id":"abc"}`,
		},
	}).Result()
	if err != nil {
		t.Fatalf("seed XAdd: %v", err)
	}

	if err := rdb.XGroupCreateMkStream(ctx, reclaimTestStream, reclaimTestGroup, "0").Err(); err != nil {
		t.Fatalf("seed XGroupCreateMkStream: %v", err)
	}

	streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    reclaimTestGroup,
		Consumer: "ghost-consumer",
		Streams:  []string{reclaimTestStream, ">"},
		Count:    10,
	}).Result()
	if err != nil {
		t.Fatalf("seed XReadGroup: %v", err)
	}
	if len(streams) != 1 || len(streams[0].Messages) != 1 {
		t.Fatalf("expected exactly one delivered message, got %+v", streams)
	}

	return id
}

type nilWriter struct{}

func (nilWriter) Write(p []byte) (int, error) { return len(p), nil }

func newQuietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(nilWriter{}, nil))
}

// TestReclaimPending_ClaimsAndProcessesIdleMessage reproduces the HIGH
// finding: Config.ClaimIdleTime was never consumed by an XAUTOCLAIM loop, so
// a message stuck in the PEL after a consumer crash was never redelivered.
func TestReclaimPending_ClaimsAndProcessesIdleMessage(t *testing.T) {
	srv := miniredis.RunT(t)

	ctx := context.Background()
	seedClient := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	defer seedClient.Close()

	msgID := seedStuckPendingMessage(t, ctx, seedClient)

	claimIdleTime := 30 * time.Second

	cfg := Config{
		RedisURL:      fmt.Sprintf("redis://%s", srv.Addr()),
		GroupName:     reclaimTestGroup,
		ConsumerName:  "consumer-a",
		StreamKey:     reclaimTestStream,
		BatchSize:     10,
		BlockTimeout:  time.Second,
		ClaimIdleTime: claimIdleTime,
		Enabled:       true,
	}

	handler := &recordingHandler{}
	logger := newQuietLogger()

	c, err := NewConsumer(cfg, handler, logger)
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	defer c.Stop()

	// Not yet idle long enough -- reclaim must not steal it from ghost-consumer.
	if err := c.reclaimPending(ctx); err != nil {
		t.Fatalf("reclaimPending (pre-idle): %v", err)
	}
	if got := handler.count(); got != 0 {
		t.Fatalf("handler invoked %d times before the message was idle long enough, want 0", got)
	}

	// Advance miniredis's virtual clock past ClaimIdleTime so the pending
	// entry becomes eligible for reclaim.
	srv.SetTime(time.Now().Add(claimIdleTime + time.Second))

	if err := c.reclaimPending(ctx); err != nil {
		t.Fatalf("reclaimPending: %v", err)
	}

	if got := handler.count(); got != 1 {
		t.Fatalf("handler invoked %d times after reclaim, want 1", got)
	}
	if handler.events[0].MessageID != msgID {
		t.Fatalf("reclaimed message ID = %q, want %q", handler.events[0].MessageID, msgID)
	}
}

// TestConsumer_Start_RunsReclaimLoopPeriodically verifies Start() wires up an
// actual periodic XAUTOCLAIM sweep (not just an unused helper method) so
// PEL-stuck messages left behind by a crashed consumer are eventually
// redelivered without any manual intervention.
func TestConsumer_Start_RunsReclaimLoopPeriodically(t *testing.T) {
	srv := miniredis.RunT(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	seedClient := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	defer seedClient.Close()

	seedStuckPendingMessage(t, ctx, seedClient)

	claimIdleTime := 50 * time.Millisecond

	cfg := Config{
		RedisURL:       fmt.Sprintf("redis://%s", srv.Addr()),
		GroupName:      reclaimTestGroup,
		ConsumerName:   "consumer-a",
		StreamKey:      reclaimTestStream,
		BatchSize:      10,
		BlockTimeout:   100 * time.Millisecond,
		ClaimIdleTime:  claimIdleTime,
		ReaperInterval: 20 * time.Millisecond,
		Enabled:        true,
	}

	handler := &recordingHandler{}
	logger := newQuietLogger()

	c, err := NewConsumer(cfg, handler, logger)
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Stop()

	// The reclaim loop ticks in real time, so advance miniredis's virtual
	// clock once up front -- every subsequent real-time tick sees the entry
	// as idle regardless of how much real wall-clock time has elapsed.
	srv.SetTime(time.Now().Add(claimIdleTime + time.Second))

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if handler.count() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := handler.count(); got != 1 {
		t.Fatalf("handler invoked %d times within deadline, want 1 (reclaim loop did not run)", got)
	}
}

// TestReclaimPending_RoutesPoisonMessageToDLQAfterMaxDeliveries reproduces
// the pair of HIGH findings in dlq.go: shouldSendToDLQ compares RetryCount,
// but nothing ever redelivered a message, so RetryCount never advanced and
// DLQ routing could never fire. Once reclaimPending performs a real
// XAUTOCLAIM (which increments the delivery counter as a side effect), a
// message that keeps getting reclaimed must eventually cross MaxDeliveries
// and be routed to the DLQ stream instead of looping forever.
func TestReclaimPending_RoutesPoisonMessageToDLQAfterMaxDeliveries(t *testing.T) {
	srv := miniredis.RunT(t)

	ctx := context.Background()
	seedClient := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	defer seedClient.Close()

	msgID := seedStuckPendingMessage(t, ctx, seedClient)

	claimIdleTime := 10 * time.Millisecond

	cfg := Config{
		RedisURL:      fmt.Sprintf("redis://%s", srv.Addr()),
		GroupName:     reclaimTestGroup,
		ConsumerName:  "consumer-a",
		StreamKey:     reclaimTestStream,
		BatchSize:     10,
		BlockTimeout:  time.Second,
		ClaimIdleTime: claimIdleTime,
		DLQStreamKey:  "alt:events:articles:dlq",
		MaxDeliveries: 2,
		Enabled:       true,
	}

	// A handler that always fails: every reclaim redelivers it, incrementing
	// RetryCount, but it never leaves the PEL through normal success.
	handler := &recordingHandler{err: fmt.Errorf("poison message: always fails")}
	logger := newQuietLogger()

	c, err := NewConsumer(cfg, handler, logger)
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	defer c.Stop()

	// Reclaim repeatedly, monotonically advancing the virtual clock well
	// past ClaimIdleTime relative to the *previous* virtual time each
	// round (not real wall-clock time, which barely moves across a fast
	// test loop) until MaxDeliveries (2) is exceeded.
	virtualNow := time.Now()
	for i := 0; i < 4; i++ {
		virtualNow = virtualNow.Add(claimIdleTime + time.Second)
		srv.SetTime(virtualNow)
		if err := c.reclaimPending(ctx); err != nil {
			t.Fatalf("reclaimPending iteration %d: %v", i, err)
		}
	}

	// The poison message must have been ejected to the DLQ stream...
	dlqEntries, err := seedClient.XRange(ctx, cfg.DLQStreamKey, "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange DLQ: %v", err)
	}
	if len(dlqEntries) != 1 {
		t.Fatalf("DLQ stream has %d entries, want 1", len(dlqEntries))
	}
	if dlqEntries[0].Values["dlq_original_id"] != msgID {
		t.Fatalf("DLQ entry dlq_original_id = %v, want %q", dlqEntries[0].Values["dlq_original_id"], msgID)
	}

	// ...and ACKed out of the main stream's PEL so it stops being redelivered.
	pending, err := seedClient.XPending(ctx, reclaimTestStream, reclaimTestGroup).Result()
	if err != nil {
		t.Fatalf("XPending: %v", err)
	}
	if pending.Count != 0 {
		t.Fatalf("PEL still has %d pending entries after DLQ routing, want 0", pending.Count)
	}
}

// TestNewConsumer_WiresAckerIntoHandler verifies NewConsumer automatically
// injects itself as the Acknowledger into any handler implementing
// AckSetter, so the ack-after-durable-write wiring can't be silently
// forgotten by a future call site.
func TestNewConsumer_WiresAckerIntoHandler(t *testing.T) {
	srv := miniredis.RunT(t)

	cfg := Config{
		RedisURL:     fmt.Sprintf("redis://%s", srv.Addr()),
		GroupName:    reclaimTestGroup,
		ConsumerName: "consumer-a",
		StreamKey:    reclaimTestStream,
		Enabled:      true,
	}

	handler := &ackSetterSpy{}
	c, err := NewConsumer(cfg, handler, newQuietLogger())
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	defer c.Stop()

	if handler.acker == nil {
		t.Fatal("NewConsumer did not wire an Acknowledger into the handler via AckSetter")
	}
	if handler.acker != Acknowledger(c) {
		t.Fatal("NewConsumer wired an Acknowledger other than itself")
	}
}

type ackSetterSpy struct {
	acker Acknowledger
}

func (s *ackSetterSpy) HandleEvent(context.Context, Event) error { return nil }
func (s *ackSetterSpy) SetAcker(a Acknowledger)                  { s.acker = a }
