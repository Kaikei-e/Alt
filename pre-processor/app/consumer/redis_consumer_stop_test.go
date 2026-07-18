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
	// Second Stop must not panic on a closed channel.
	c.Stop()
}
