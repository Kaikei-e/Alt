package consumer

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

// TestConsumer_StopIntake_IsIdempotent reproduces the MED finding: StopIntake
// used to close(shutdownChan) unconditionally, so calling it more than once
// (e.g. once directly during a handler flush and once again via Stop() during
// the same shutdown) panicked on a double close of an already-closed channel.
func TestConsumer_StopIntake_IsIdempotent(t *testing.T) {
	srv := miniredis.RunT(t)

	cfg := Config{
		RedisURL:     fmt.Sprintf("redis://%s", srv.Addr()),
		GroupName:    reclaimTestGroup,
		ConsumerName: "consumer-a",
		StreamKey:    reclaimTestStream,
		Enabled:      true,
	}

	handler := &recordingHandler{}
	c, err := NewConsumer(cfg, handler, newQuietLogger())
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}

	c.StopIntake()
	c.StopIntake() // must not panic
	c.Stop()       // Stop() calls StopIntake again internally -- also must not panic
}

// TestConsumer_Close_WaitsForLoopsBeforeClosingClient exercises the
// consumeLoop/reclaimLoop shutdown path end-to-end: Close() must not return
// until both background loops have actually observed the shutdown signal and
// exited, instead of closing the shared Redis client while they might still
// have an in-flight XReadGroup/XAutoClaim call racing against it.
func TestConsumer_Close_WaitsForLoopsBeforeClosingClient(t *testing.T) {
	srv := miniredis.RunT(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := Config{
		RedisURL:       fmt.Sprintf("redis://%s", srv.Addr()),
		GroupName:      reclaimTestGroup,
		ConsumerName:   "consumer-a",
		StreamKey:      reclaimTestStream,
		BatchSize:      10,
		BlockTimeout:   20 * time.Millisecond,
		ReaperInterval: 20 * time.Millisecond,
		Enabled:        true,
	}

	handler := &recordingHandler{}
	c, err := NewConsumer(cfg, handler, newQuietLogger())
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	c.StopIntake()

	done := make(chan struct{})
	go func() {
		c.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close() did not return within 5s -- consumeLoop/reclaimLoop may not be exiting, or Close() is deadlocked waiting on them")
	}
}
