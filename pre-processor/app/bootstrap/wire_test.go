package bootstrap

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

// TestBuildRedisConsumer_DisabledIsNotAnError verifies the deliberate
// CONSUMER_ENABLED=false opt-out path succeeds (no construction/start attempted
// against a real Redis) — this is the "explicit config flag" no-op, not a
// silent failure.
func TestBuildRedisConsumer_DisabledIsNotAnError(t *testing.T) {
	t.Setenv("CONSUMER_ENABLED", "false")

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, err := buildRedisConsumer(context.Background(), nil, nil, nil, log)
	if err != nil {
		t.Fatalf("buildRedisConsumer with CONSUMER_ENABLED=false returned error: %v", err)
	}
	if c == nil {
		t.Fatal("buildRedisConsumer returned nil consumer for the disabled no-op path")
	}
}

// TestBuildRedisConsumer_ConstructionFailurePropagatesError reproduces the
// HIGH finding: a malformed REDIS_STREAMS_URL previously caused
// buildRedisConsumer to log-and-swallow the error and return nil, which the
// caller only logged — the event-driven summarization consumer died silently.
// It must now surface the error so BuildDependencies can fail startup.
func TestBuildRedisConsumer_ConstructionFailurePropagatesError(t *testing.T) {
	t.Setenv("CONSUMER_ENABLED", "true")
	t.Setenv("REDIS_STREAMS_URL", "not-a-valid-redis-url")

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, err := buildRedisConsumer(context.Background(), nil, nil, nil, log)
	if err == nil {
		t.Fatal("buildRedisConsumer with a malformed REDIS_STREAMS_URL returned nil error, want propagated construction error")
	}
	if c != nil {
		t.Fatal("buildRedisConsumer returned a non-nil consumer alongside an error")
	}
}

// TestBuildRedisConsumer_StartFailurePropagatesError reproduces the second
// half of the HIGH finding: when the consumer is enabled but Redis is
// unreachable, Start() previously failed silently (logged only) while the
// caller kept running with a half-initialized consumer.
func TestBuildRedisConsumer_StartFailurePropagatesError(t *testing.T) {
	t.Setenv("CONSUMER_ENABLED", "true")
	// Valid URL syntax but nothing listens here — Start()'s ensureConsumerGroup
	// call must fail against an unreachable broker.
	t.Setenv("REDIS_STREAMS_URL", "redis://127.0.0.1:1")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, err := buildRedisConsumer(ctx, nil, nil, nil, log)
	if err == nil {
		t.Fatal("buildRedisConsumer against an unreachable Redis returned nil error, want propagated start error")
	}
	if c != nil {
		t.Fatal("buildRedisConsumer returned a non-nil consumer alongside a start error")
	}
}
