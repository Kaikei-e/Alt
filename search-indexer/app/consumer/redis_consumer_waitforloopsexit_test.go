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

// gatedHandler blocks HandleEvent for a chosen EventID until release is
// closed, signalling on handling once it starts blocking. Every event
// (blocked or not) is appended to processed once HandleEvent proceeds --
// simulating a buffering handler's enqueue step.
type gatedHandler struct {
	gateEventID string
	handling    chan struct{}
	release     chan struct{}

	mu        sync.Mutex
	processed []string
}

func (h *gatedHandler) HandleEvent(_ context.Context, event Event) error {
	if event.EventID == h.gateEventID {
		close(h.handling)
		<-h.release
	}
	h.mu.Lock()
	h.processed = append(h.processed, event.EventID)
	h.mu.Unlock()
	return nil
}

func (h *gatedHandler) snapshot() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.processed...)
}

// TestConsumer_WaitForLoopsExit_BlocksUntilInFlightBatchFinishes reproduces
// the MED shutdown-ordering finding: StopIntake() only closes the shutdown
// signal channel, it does not wait for consumeLoop to actually stop. If a
// buffering handler's flush is issued right after StopIntake() returns
// (the previous bootstrap/app.go ordering), it can race an in-flight
// processMessages() batch still calling HandleEvent for later messages in
// that same batch -- any event enqueued during that window is silently
// missed by the flush. WaitForLoopsExit must block until the in-flight
// batch has fully drained, so a caller that waits on it before flushing is
// guaranteed to see every event the batch enqueued.
func TestConsumer_WaitForLoopsExit_BlocksUntilInFlightBatchFinishes(t *testing.T) {
	srv := miniredis.RunT(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rdb := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	const stream = "alt:events:articles"
	const group = "search-indexer-group"

	if err := rdb.XGroupCreateMkStream(ctx, stream, group, "0").Err(); err != nil {
		t.Fatalf("XGroupCreateMkStream: %v", err)
	}
	// Two messages delivered together in a single XReadGroup batch
	// (BatchSize below is 2): msg1's handler blocks, msg2's does not.
	for _, id := range []string{"msg1", "msg2"} {
		if err := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: stream,
			Values: map[string]interface{}{"event_id": id},
		}).Err(); err != nil {
			t.Fatalf("seed XAdd %s: %v", id, err)
		}
	}

	handler := &gatedHandler{
		gateEventID: "msg1",
		handling:    make(chan struct{}),
		release:     make(chan struct{}),
	}

	cfg := Config{
		RedisURL:     fmt.Sprintf("redis://%s", srv.Addr()),
		GroupName:    group,
		ConsumerName: "consumer-a",
		StreamKey:    stream,
		BatchSize:    2,
		BlockTimeout: time.Second,
		Enabled:      true,
	}

	c, err := NewConsumer(cfg, handler, newQuietLogger())
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Stop()

	select {
	case <-handler.handling:
	case <-time.After(5 * time.Second):
		t.Fatal("handler never started handling msg1 -- consumeLoop did not read the seeded batch")
	}

	// msg1's handler is now blocked mid-batch; msg2 has not been processed
	// yet. StopIntake only signals shutdown, it must not itself wait.
	c.StopIntake()

	waitDone := make(chan struct{})
	go func() {
		c.WaitForLoopsExit()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		t.Fatal("WaitForLoopsExit returned before the in-flight batch finished processing msg2")
	case <-time.After(150 * time.Millisecond):
		// Expected: still blocked because msg1's handler hasn't returned yet.
	}

	close(handler.release) // let msg1's handler finish, unblocking the batch

	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForLoopsExit did not return after the in-flight batch finished")
	}

	got := handler.snapshot()
	if len(got) != 2 {
		t.Fatalf("processed = %v, want both msg1 and msg2 to have been handled before WaitForLoopsExit returned", got)
	}
}
