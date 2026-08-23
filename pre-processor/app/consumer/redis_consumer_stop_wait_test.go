package consumer

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// blockingHandler blocks inside HandleEvent until release is closed, after
// signalling (once) that it has begun. This pins the consume loop mid-handler
// so a test can observe whether Stop waits for the in-flight XAck.
type blockingHandler struct {
	once    sync.Once
	started chan struct{}
	release chan struct{}
}

func (h *blockingHandler) HandleEvent(_ context.Context, _ Event) error {
	h.once.Do(func() { close(h.started) })
	<-h.release
	return nil
}

// TestConsumer_Stop_WaitsForInflightAckBeforeClosingClient reproduces GO-1:
// Stop closed the Redis client without waiting for the consume loop to exit,
// so a message whose handler had already succeeded lost its XAck against the
// closed client and stayed in the PEL (redelivered on restart). Stop must
// block until the loop drains, letting the XAck land, before closing the
// client.
func TestConsumer_Stop_WaitsForInflightAckBeforeClosingClient(t *testing.T) {
	srv := miniredis.RunT(t)
	ctx := context.Background()

	seed := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	defer func() { _ = seed.Close() }()

	const stream = "alt:events:articles"
	const group = "pre-processor-group"

	if err := seed.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{"event_id": "e1", "payload": "{}"},
	}).Err(); err != nil {
		t.Fatalf("seed XAdd: %v", err)
	}
	if err := seed.XGroupCreateMkStream(ctx, stream, group, "0").Err(); err != nil {
		t.Fatalf("seed XGroupCreateMkStream: %v", err)
	}

	handler := &blockingHandler{started: make(chan struct{}), release: make(chan struct{})}

	cfg := Config{
		RedisURL:      fmt.Sprintf("redis://%s", srv.Addr()),
		GroupName:     group,
		ConsumerName:  "c1",
		StreamKey:     stream,
		BatchSize:     10,
		BlockTimeout:  200 * time.Millisecond,
		ClaimIdleTime: time.Hour, // keep the reclaim loop dormant during the test
		Enabled:       true,
	}

	c, err := NewConsumer(cfg, handler, newQuietLogger())
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait until the handler is mid-flight (message read, XAck not yet issued).
	select {
	case <-handler.started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler never started; consumer did not read the seeded message")
	}

	stopReturned := make(chan struct{})
	go func() {
		c.Stop()
		close(stopReturned)
	}()

	// Stop must block: the consume loop is stuck in HandleEvent and Stop has to
	// wait for it before closing the client.
	select {
	case <-stopReturned:
		close(handler.release)
		t.Fatal("Stop returned while a handler was still in flight: it closed the client without waiting for the loop, losing the XAck")
	case <-time.After(150 * time.Millisecond):
	}

	// Let the handler finish so its XAck runs against the still-open client.
	close(handler.release)

	select {
	case <-stopReturned:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop did not return after the loop drained")
	}

	pending, err := seed.XPending(ctx, stream, group).Result()
	if err != nil {
		t.Fatalf("XPending: %v", err)
	}
	if pending.Count != 0 {
		t.Fatalf("PEL still has %d pending entries: Stop closed the client before the in-flight XAck landed", pending.Count)
	}
}
