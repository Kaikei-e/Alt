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

// waitTimeout bounds how long a test will block on a synchronization channel
// before failing. It is a safety net against a hung goroutine, never the
// mechanism used to decide pass/fail — every wait below blocks on a channel
// send from the code under test and returns as soon as that happens.
const waitTimeout = 2 * time.Second

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
}

// waitForSignal blocks until ch receives a value or waitTimeout elapses, in
// which case the test fails with msg. This replaces time.Sleep-based
// "wait long enough" synchronization with a deterministic blocking receive.
func waitForSignal(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(waitTimeout):
		t.Fatal(msg)
	}
}

func TestJobRunner_StartAndStop(t *testing.T) {
	t.Run("should start and stop cleanly", func(t *testing.T) {
		var callCount atomic.Int32
		called := make(chan struct{}, 1)
		runner := NewJobRunner(JobConfig{
			Name:     "test-job",
			Interval: 5 * time.Millisecond,
		}, func(ctx context.Context) error {
			callCount.Add(1)
			select {
			case called <- struct{}{}:
			default:
			}
			return nil
		}, testLogger())

		ctx := context.Background()
		runner.Start(ctx)

		// Deterministically wait for the first execution instead of sleeping
		// for a guessed duration.
		waitForSignal(t, called, "job did not execute within timeout")
		runner.Stop()

		assert.Greater(t, callCount.Load(), int32(0))
	})
}

func TestJobRunner_RunImmediately(t *testing.T) {
	t.Run("should run immediately when configured", func(t *testing.T) {
		var callCount atomic.Int32
		called := make(chan struct{}, 1)
		runner := NewJobRunner(JobConfig{
			Name:           "immediate-job",
			Interval:       1 * time.Hour, // Long interval to ensure only immediate run
			RunImmediately: true,
		}, func(ctx context.Context) error {
			callCount.Add(1)
			called <- struct{}{}
			return nil
		}, testLogger())

		ctx := context.Background()
		runner.Start(ctx)

		// The immediate run happens synchronously before the ticker is even
		// created, so waiting for this signal (rather than sleeping) is both
		// faster and race-free: by the time we receive it, the call has
		// already completed and Stop can safely follow.
		waitForSignal(t, called, "immediate job did not run within timeout")
		runner.Stop()

		assert.Equal(t, int32(1), callCount.Load(),
			"with a 1h interval, only the immediate run should have fired")
	})
}

func TestJobRunner_Backoff(t *testing.T) {
	t.Run("should backoff on configured errors", func(t *testing.T) {
		errOverloaded := errors.New("overloaded")
		var callCount atomic.Int32
		calls := make(chan struct{}, 64)

		runner := NewJobRunner(JobConfig{
			Name:            "backoff-job",
			Interval:        10 * time.Millisecond,
			InitialBackoff:  50 * time.Millisecond,
			MaxBackoff:      100 * time.Millisecond,
			BackoffOnErrors: []error{errOverloaded},
		}, func(ctx context.Context) error {
			callCount.Add(1)
			select {
			case calls <- struct{}{}:
			default:
			}
			return errOverloaded
		}, testLogger())

		ctx := context.Background()
		runner.Start(ctx)

		// Wait for the first tick (interval 10ms) so we know the ticker is
		// actually running, then observe for a fixed backoff-measurement
		// window. The window itself must be real wall-clock time since the
		// property under test (call rate under backoff) is defined in terms
		// of elapsed time, not a discrete event to block on.
		waitForSignal(t, calls, "job never executed even once")
		time.Sleep(90 * time.Millisecond)
		runner.Stop()

		// With 10ms interval, without backoff we'd see ~9-10 calls in 100ms.
		// With backoff starting at 50ms and doubling to 100ms (capped), we
		// should see only a handful.
		assert.LessOrEqual(t, callCount.Load(), int32(4),
			"backoff should have suppressed most ticks")
		// Lower bound catches the opposite regression: a backoff that never
		// resets/reschedules the ticker (e.g. blocks forever after the first
		// error) would also satisfy the upper bound above while being just
		// as broken.
		assert.GreaterOrEqual(t, callCount.Load(), int32(2),
			"backoff must still allow the job to run again after backing off, not stall forever")
	})
}

func TestJobRunner_NonBackoffErrorResetsInterval(t *testing.T) {
	errOverloaded := errors.New("overloaded")
	errOther := errors.New("other")
	var callCount atomic.Int32
	var phase atomic.Int32
	calls := make(chan struct{}, 64)

	runner := NewJobRunner(JobConfig{
		Name:            "reset-interval-job",
		Interval:        15 * time.Millisecond,
		InitialBackoff:  200 * time.Millisecond,
		MaxBackoff:      200 * time.Millisecond,
		BackoffOnErrors: []error{errOverloaded},
		RunImmediately:  true,
	}, func(ctx context.Context) error {
		n := callCount.Add(1)
		select {
		case calls <- struct{}{}:
		default:
		}
		switch {
		case n == 1:
			phase.Store(1)
			return errOverloaded // enter backoff (200ms)
		case n == 2:
			phase.Store(2)
			return errOther // non-backoff: must restore Interval
		default:
			phase.Store(3)
			return nil
		}
	}, testLogger())

	ctx := context.Background()
	runner.Start(ctx)

	// After immediate overloaded + one backoff tick (~200ms) returning other,
	// subsequent ticks should use Interval (15ms), not stay at 200ms. Block
	// on the actual call signals (bounded by an overall deadline) instead of
	// sleeping in fixed increments and re-checking a counter.
	deadline := time.After(500 * time.Millisecond)
waitLoop:
	for callCount.Load() < 5 {
		select {
		case <-calls:
		case <-deadline:
			break waitLoop
		}
	}
	runner.Stop()

	require.GreaterOrEqual(t, callCount.Load(), int32(5),
		"after non-backoff error, ticker must resume normal interval; phase=%d calls=%d",
		phase.Load(), callCount.Load())
}

func TestJobRunner_PanicRecovery(t *testing.T) {
	t.Run("should recover from panics and keep the ticker running", func(t *testing.T) {
		var callCount atomic.Int32
		calls := make(chan struct{}, 64)

		runner := NewJobRunner(JobConfig{
			Name:     "panic-job",
			Interval: 5 * time.Millisecond,
		}, func(ctx context.Context) error {
			callCount.Add(1)
			select {
			case calls <- struct{}{}:
			default:
			}
			panic("test panic")
		}, testLogger())

		ctx := context.Background()
		runner.Start(ctx)

		// A single panicking invocation proves recover() ran; a second one
		// proves the ticker loop survived that panic and is still scheduling
		// work — the actual behavior this test exists to guard, which the
		// original "sleep then check nothing crashed" version never asserted.
		waitForSignal(t, calls, "job did not run once")
		waitForSignal(t, calls, "job runner did not survive the panic to run a second time")
		runner.Stop()

		assert.GreaterOrEqual(t, callCount.Load(), int32(2),
			"ticker must keep invoking the job after a panic is recovered")
	})
}

func TestJobRunner_ContextCancellation(t *testing.T) {
	t.Run("should stop when context is canceled", func(t *testing.T) {
		var callCount atomic.Int32
		calls := make(chan struct{}, 64)
		runner := NewJobRunner(JobConfig{
			Name:     "cancel-job",
			Interval: 5 * time.Millisecond,
		}, func(ctx context.Context) error {
			callCount.Add(1)
			select {
			case calls <- struct{}{}:
			default:
			}
			return nil
		}, testLogger())

		ctx, cancel := context.WithCancel(context.Background())
		runner.Start(ctx)

		// Deterministically wait for at least one real execution before
		// canceling, instead of sleeping and hoping the ticker fired.
		waitForSignal(t, calls, "job did not execute before cancellation")
		beforeCancel := callCount.Load()
		cancel()

		// Proving absence of further work genuinely requires observing that
		// no event arrives for some real duration; a bounded wait here is
		// the correct tool (there is no event to block on for "nothing
		// happened"), unlike the removed Sleeps above which stood in for an
		// event that could otherwise be waited on directly.
		select {
		case <-calls:
			// One in-flight tick racing the cancellation is tolerated below.
		case <-time.After(30 * time.Millisecond):
		}

		afterCancel := callCount.Load()
		assert.LessOrEqual(t, afterCancel-beforeCancel, int32(1),
			"no more than one in-flight execution should complete after cancel")
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
		called1 := make(chan struct{}, 1)
		called2 := make(chan struct{}, 1)

		ctx := context.Background()
		group := NewJobGroup(ctx, testLogger())
		group.Add(NewJobRunner(JobConfig{
			Name:     "job-1",
			Interval: 5 * time.Millisecond,
		}, func(ctx context.Context) error {
			count1.Add(1)
			select {
			case called1 <- struct{}{}:
			default:
			}
			return nil
		}, testLogger()))

		group.Add(NewJobRunner(JobConfig{
			Name:     "job-2",
			Interval: 5 * time.Millisecond,
		}, func(ctx context.Context) error {
			count2.Add(1)
			select {
			case called2 <- struct{}{}:
			default:
			}
			return nil
		}, testLogger()))

		waitForSignal(t, called1, "job-1 did not execute within timeout")
		waitForSignal(t, called2, "job-2 did not execute within timeout")
		group.StopAll()

		require.Greater(t, count1.Load(), int32(0))
		require.Greater(t, count2.Load(), int32(0))
	})
}
