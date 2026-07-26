package consumer

import (
	"log/slog"
	"os"
	"testing"
)

func TestConsumer_Stop_Idempotent(t *testing.T) {
	c := &Consumer{
		config:       DefaultConfig(),
		logger:       slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		shutdownChan: make(chan struct{}),
	}

	c.Stop()

	// Verify Stop actually closed shutdownChan (the observable effect
	// consumeLoop/reclaimLoop select on to know to exit) rather than only
	// checking that the call didn't panic.
	select {
	case _, open := <-c.shutdownChan:
		if open {
			t.Fatal("shutdownChan should be closed after Stop, got an open channel with a pending value")
		}
	default:
		t.Fatal("shutdownChan should be closed (non-blocking receive should succeed) after Stop")
	}

	// Second Stop must not panic on a closed channel, and shutdownOnce must
	// make it a true no-op rather than a second close (which would panic).
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("second Stop() call panicked: %v", r)
			}
		}()
		c.Stop()
	}()
}
