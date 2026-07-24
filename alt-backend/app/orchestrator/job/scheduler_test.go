package job

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestJobScheduler_RunsJobOnStart(t *testing.T) {
	var count atomic.Int32

	scheduler := NewJobScheduler()
	scheduler.Add(Job{
		Name:     "test-job",
		Interval: time.Hour, // long interval - we only care about the initial run
		Timeout:  time.Second,
		Fn: func(ctx context.Context) error {
			count.Add(1)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	// Wait for initial execution
	time.Sleep(50 * time.Millisecond)
	cancel()
	scheduler.Shutdown()

	if got := count.Load(); got < 1 {
		t.Errorf("expected job to run at least once, ran %d times", got)
	}
}

func TestJobScheduler_StopsOnContextCancel(t *testing.T) {
	var count atomic.Int32

	scheduler := NewJobScheduler()
	scheduler.Add(Job{
		Name:     "stop-test",
		Interval: 10 * time.Millisecond,
		Timeout:  time.Second,
		Fn: func(ctx context.Context) error {
			count.Add(1)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	// Let it run a few ticks
	time.Sleep(50 * time.Millisecond)
	cancel()
	scheduler.Shutdown()

	// Record count after shutdown
	countAfterShutdown := count.Load()
	time.Sleep(30 * time.Millisecond)

	// Should not increase after shutdown
	if count.Load() != countAfterShutdown {
		t.Error("job continued running after context cancel and shutdown")
	}
}

func TestJobScheduler_JobTimeoutRespected(t *testing.T) {
	var timedOut atomic.Bool

	scheduler := NewJobScheduler()
	scheduler.Add(Job{
		Name:     "timeout-test",
		Interval: time.Hour,
		Timeout:  50 * time.Millisecond,
		Fn: func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				timedOut.Store(true)
				return ctx.Err()
			case <-time.After(5 * time.Second):
				return nil
			}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	// Wait for timeout to fire
	time.Sleep(200 * time.Millisecond)
	cancel()
	scheduler.Shutdown()

	if !timedOut.Load() {
		t.Error("expected job context to be cancelled by timeout")
	}
}

func TestJobScheduler_ShutdownWaitsForJobs(t *testing.T) {
	var completed atomic.Bool

	scheduler := NewJobScheduler()
	scheduler.Add(Job{
		Name:     "slow-job",
		Interval: time.Hour,
		Timeout:  time.Second,
		Fn: func(ctx context.Context) error {
			time.Sleep(50 * time.Millisecond)
			completed.Store(true)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	// Give the job time to start
	time.Sleep(10 * time.Millisecond)
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

	scheduler := NewJobScheduler()
	scheduler.Add(Job{
		Name:     "panicky-job",
		Interval: 20 * time.Millisecond,
		Timeout:  time.Second,
		Fn: func(ctx context.Context) error {
			n := runs.Add(1)
			if n == 1 {
				panic("simulated rule-8 nil-guard panic")
			}
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	// Give it time to panic on the first run and tick at least once more.
	time.Sleep(150 * time.Millisecond)
	cancel()
	scheduler.Shutdown()

	if got := runs.Load(); got < 2 {
		t.Errorf("expected job to keep running after a panic (recovered), got %d runs", got)
	}
}

func TestJobScheduler_MultipleJobs(t *testing.T) {
	var countA, countB atomic.Int32

	scheduler := NewJobScheduler()
	scheduler.Add(Job{
		Name:     "job-a",
		Interval: 10 * time.Millisecond,
		Timeout:  time.Second,
		Fn: func(ctx context.Context) error {
			countA.Add(1)
			return nil
		},
	})
	scheduler.Add(Job{
		Name:     "job-b",
		Interval: 10 * time.Millisecond,
		Timeout:  time.Second,
		Fn: func(ctx context.Context) error {
			countB.Add(1)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	cancel()
	scheduler.Shutdown()

	if countA.Load() < 1 || countB.Load() < 1 {
		t.Errorf("expected both jobs to run, got A=%d B=%d", countA.Load(), countB.Load())
	}
}
