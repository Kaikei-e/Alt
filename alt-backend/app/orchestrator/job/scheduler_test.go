package job

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// waitSignal blocks until ch fires or timeout elapses, failing the test on
// timeout. Using a channel signaled directly from the job's Fn instead of a
// blind time.Sleep means the test proceeds as soon as the scheduler actually
// runs the job, and only ever waits the full timeout when something is
// genuinely broken.
func waitSignal(t *testing.T, ch <-chan struct{}, timeout time.Duration, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatalf("timed out after %v waiting for %s", timeout, what)
	}
}

func TestJobScheduler_RunsJobOnStart(t *testing.T) {
	var count atomic.Int32
	ran := make(chan struct{}, 1)

	scheduler := NewJobScheduler()
	scheduler.Add(Job{
		Name:     "test-job",
		Interval: time.Hour, // long interval - we only care about the initial run
		Timeout:  time.Second,
		Fn: func(ctx context.Context) error {
			count.Add(1)
			select {
			case ran <- struct{}{}:
			default:
			}
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	waitSignal(t, ran, 2*time.Second, "initial job execution")
	cancel()
	scheduler.Shutdown()

	if got := count.Load(); got < 1 {
		t.Errorf("expected job to run at least once, ran %d times", got)
	}
}

func TestJobScheduler_StopsOnContextCancel(t *testing.T) {
	var count atomic.Int32
	tick := make(chan struct{}, 1)

	scheduler := NewJobScheduler()
	scheduler.Add(Job{
		Name:     "stop-test",
		Interval: 5 * time.Millisecond,
		Timeout:  time.Second,
		Fn: func(ctx context.Context) error {
			count.Add(1)
			select {
			case tick <- struct{}{}:
			default:
			}
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	// Deterministically wait for a couple of ticks to have actually fired
	// before we cancel, instead of guessing a sleep duration.
	waitSignal(t, tick, 2*time.Second, "first tick")
	waitSignal(t, tick, 2*time.Second, "second tick")

	cancel()
	// Shutdown blocks on the scheduler's WaitGroup, so by the time it
	// returns runJob has already observed ctx.Done() and exited for good -
	// no further ticks can fire after this point.
	scheduler.Shutdown()

	countAfterShutdown := count.Load()

	// Guard against a regression where the job keeps running on a
	// goroutine Shutdown doesn't actually wait for. The interval is 5ms,
	// so 100ms is generous enough to catch a real regression fast while
	// staying short in the passing case.
	time.Sleep(100 * time.Millisecond)

	if got := count.Load(); got != countAfterShutdown {
		t.Errorf("job continued running after context cancel and shutdown: count went from %d to %d", countAfterShutdown, got)
	}
}

func TestJobScheduler_JobTimeoutRespected(t *testing.T) {
	var timedOut atomic.Bool
	timeoutHit := make(chan struct{}, 1)

	scheduler := NewJobScheduler()
	scheduler.Add(Job{
		Name:     "timeout-test",
		Interval: time.Hour,
		Timeout:  50 * time.Millisecond,
		Fn: func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				timedOut.Store(true)
				select {
				case timeoutHit <- struct{}{}:
				default:
				}
				return ctx.Err()
			case <-time.After(5 * time.Second):
				return nil
			}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	waitSignal(t, timeoutHit, 2*time.Second, "job timeout to fire")
	cancel()
	scheduler.Shutdown()

	if !timedOut.Load() {
		t.Error("expected job context to be cancelled by timeout")
	}
}

func TestJobScheduler_ShutdownWaitsForJobs(t *testing.T) {
	var completed atomic.Bool
	started := make(chan struct{}, 1)

	scheduler := NewJobScheduler()
	scheduler.Add(Job{
		Name:     "slow-job",
		Interval: time.Hour,
		Timeout:  time.Second,
		Fn: func(ctx context.Context) error {
			select {
			case started <- struct{}{}:
			default:
			}
			time.Sleep(50 * time.Millisecond)
			completed.Store(true)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	// Confirm the job has actually started before we cancel and shut down,
	// instead of guessing how long "starting" takes.
	waitSignal(t, started, 2*time.Second, "job to start")
	cancel()
	scheduler.Shutdown()

	if !completed.Load() {
		t.Error("shutdown did not wait for running job to complete")
	}
}

// Finding [7]: a panic inside a job's Fn (e.g. a Rule-8 nil-guard panic in
// outbox_worker.go, or an unexpected nil dereference) must not crash the
// whole process. Go's default behavior is for an unrecovered goroutine panic
// to take down the entire program; runJob/executeJob must recover it so
// other scheduled jobs and the next tick of this same job keep running.
func TestJobScheduler_RecoversFromJobPanic(t *testing.T) {
	var runs atomic.Int32
	secondRun := make(chan struct{}, 1)

	scheduler := NewJobScheduler()
	scheduler.Add(Job{
		Name:     "panicky-job",
		Interval: 10 * time.Millisecond,
		Timeout:  time.Second,
		Fn: func(ctx context.Context) error {
			n := runs.Add(1)
			if n == 1 {
				panic("simulated rule-8 nil-guard panic")
			}
			select {
			case secondRun <- struct{}{}:
			default:
			}
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	// Wait for the scheduler to recover from the first run's panic and
	// tick again, rather than guessing a sleep duration long enough to
	// cover "panic, then at least one more tick".
	waitSignal(t, secondRun, 2*time.Second, "job to keep running after a panic")
	cancel()
	scheduler.Shutdown()

	if got := runs.Load(); got < 2 {
		t.Errorf("expected job to keep running after a panic (recovered), got %d runs", got)
	}
}

func TestJobScheduler_MultipleJobs(t *testing.T) {
	var countA, countB atomic.Int32
	ranA := make(chan struct{}, 1)
	ranB := make(chan struct{}, 1)

	scheduler := NewJobScheduler()
	scheduler.Add(Job{
		Name:     "job-a",
		Interval: 10 * time.Millisecond,
		Timeout:  time.Second,
		Fn: func(ctx context.Context) error {
			countA.Add(1)
			select {
			case ranA <- struct{}{}:
			default:
			}
			return nil
		},
	})
	scheduler.Add(Job{
		Name:     "job-b",
		Interval: 10 * time.Millisecond,
		Timeout:  time.Second,
		Fn: func(ctx context.Context) error {
			countB.Add(1)
			select {
			case ranB <- struct{}{}:
			default:
			}
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	waitSignal(t, ranA, 2*time.Second, "job-a to run")
	waitSignal(t, ranB, 2*time.Second, "job-b to run")
	cancel()
	scheduler.Shutdown()

	if countA.Load() < 1 || countB.Load() < 1 {
		t.Errorf("expected both jobs to run, got A=%d B=%d", countA.Load(), countB.Load())
	}
}
