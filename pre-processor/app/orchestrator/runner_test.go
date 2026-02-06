package orchestrator

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
}

func TestJobRunner_StartAndStop(t *testing.T) {
	t.Run("should start and stop cleanly", func(t *testing.T) {
		var callCount atomic.Int32
		runner := NewJobRunner(JobConfig{
			Name:     "test-job",
			Interval: 10 * time.Millisecond,
		}, func(ctx context.Context) error {
			callCount.Add(1)
			return nil
		}, testLogger())

		ctx := context.Background()
		runner.Start(ctx)

		// Wait for at least one execution
		time.Sleep(50 * time.Millisecond)
		runner.Stop()

		assert.Greater(t, callCount.Load(), int32(0))
	})
}

func TestJobRunner_RunImmediately(t *testing.T) {
	t.Run("should run immediately when configured", func(t *testing.T) {
		var callCount atomic.Int32
		runner := NewJobRunner(JobConfig{
			Name:           "immediate-job",
			Interval:       1 * time.Hour, // Long interval to ensure only immediate run
			RunImmediately: true,
		}, func(ctx context.Context) error {
			callCount.Add(1)
			return nil
		}, testLogger())

		ctx := context.Background()
		runner.Start(ctx)
		time.Sleep(50 * time.Millisecond)
		runner.Stop()

		assert.Equal(t, int32(1), callCount.Load())
	})
}

func TestJobRunner_Backoff(t *testing.T) {
	t.Run("should backoff on configured errors", func(t *testing.T) {
		errOverloaded := errors.New("overloaded")
		var callCount atomic.Int32

		runner := NewJobRunner(JobConfig{
			Name:            "backoff-job",
			Interval:        10 * time.Millisecond,
			InitialBackoff:  50 * time.Millisecond,
			MaxBackoff:      100 * time.Millisecond,
			BackoffOnErrors: []error{errOverloaded},
		}, func(ctx context.Context) error {
			callCount.Add(1)
			return errOverloaded
		}, testLogger())

		ctx := context.Background()
		runner.Start(ctx)

		// With 10ms interval, without backoff we'd see many calls in 100ms
		// With backoff starting at 50ms, we should see fewer calls
		time.Sleep(100 * time.Millisecond)
		runner.Stop()

		// Should have at most 3-4 calls due to backoff
		assert.LessOrEqual(t, callCount.Load(), int32(4))
	})
}

func TestJobRunner_PanicRecovery(t *testing.T) {
	t.Run("should recover from panics", func(t *testing.T) {
		runner := NewJobRunner(JobConfig{
			Name:     "panic-job",
			Interval: 10 * time.Millisecond,
		}, func(ctx context.Context) error {
			panic("test panic")
		}, testLogger())

		ctx := context.Background()
		runner.Start(ctx)

		// Should not crash
		time.Sleep(30 * time.Millisecond)
		runner.Stop()
	})
}

func TestJobRunner_ContextCancellation(t *testing.T) {
	t.Run("should stop when context is canceled", func(t *testing.T) {
		var callCount atomic.Int32
		runner := NewJobRunner(JobConfig{
			Name:     "cancel-job",
			Interval: 10 * time.Millisecond,
		}, func(ctx context.Context) error {
			callCount.Add(1)
			return nil
		}, testLogger())

		ctx, cancel := context.WithCancel(context.Background())
		runner.Start(ctx)
		time.Sleep(50 * time.Millisecond)

		beforeCancel := callCount.Load()
		cancel()
		time.Sleep(30 * time.Millisecond)

		// No more executions after cancel
		afterCancel := callCount.Load()
		assert.LessOrEqual(t, afterCancel-beforeCancel, int32(1))
	})
}

func TestJobRunner_NextBackoff(t *testing.T) {
	runner := NewJobRunner(JobConfig{
		InitialBackoff: 30 * time.Second,
		MaxBackoff:     5 * time.Minute,
	}, nil, testLogger())

	t.Run("should return initial backoff when current is 0", func(t *testing.T) {
		assert.Equal(t, 30*time.Second, runner.nextBackoff(0))
	})

	t.Run("should double backoff", func(t *testing.T) {
		assert.Equal(t, 60*time.Second, runner.nextBackoff(30*time.Second))
	})

	t.Run("should cap at max backoff", func(t *testing.T) {
		assert.Equal(t, 5*time.Minute, runner.nextBackoff(4*time.Minute))
	})
}

func TestJobGroup(t *testing.T) {
	t.Run("should start and stop all runners", func(t *testing.T) {
		var count1, count2 atomic.Int32

		ctx := context.Background()
		group := NewJobGroup(ctx, testLogger())
		group.Add(NewJobRunner(JobConfig{
			Name:     "job-1",
			Interval: 10 * time.Millisecond,
		}, func(ctx context.Context) error {
			count1.Add(1)
			return nil
		}, testLogger()))

		group.Add(NewJobRunner(JobConfig{
			Name:     "job-2",
			Interval: 10 * time.Millisecond,
		}, func(ctx context.Context) error {
			count2.Add(1)
			return nil
		}, testLogger()))

		time.Sleep(50 * time.Millisecond)
		group.StopAll()

		require.Greater(t, count1.Load(), int32(0))
		require.Greater(t, count2.Load(), int32(0))
	})
}
