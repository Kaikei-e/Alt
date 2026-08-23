package orchestrator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestJobGroup_AddAfterStopAll_DoesNotStart reproduces the orphan-goroutine
// half of the GO-4 finding: because Add started its runner outside the group
// lock, a runner added after StopAll had already snapshotted the group would
// still be Start()ed and then never stopped. With the stopped flag, an Add
// after StopAll must be a no-op — the runner never runs.
func TestJobGroup_AddAfterStopAll_DoesNotStart(t *testing.T) {
	ctx := context.Background()
	group := NewJobGroup(ctx, testLogger())

	group.StopAll()

	var count atomic.Int32
	group.Add(NewJobRunner(JobConfig{
		Name:           "after-stop",
		Interval:       time.Hour,
		RunImmediately: true, // would fire synchronously in run() if started
	}, func(ctx context.Context) error {
		count.Add(1)
		return nil
	}, testLogger()))

	// Give any (erroneously) spawned goroutine a chance to run its immediate tick.
	time.Sleep(50 * time.Millisecond)

	if got := count.Load(); got != 0 {
		t.Fatalf("runner added after StopAll executed %d times, want 0 (orphaned goroutine)", got)
	}
}

// TestJobRunner_StopBeforeStart_IsNoOp verifies a JobRunner whose Stop wins
// the race against Start does not later spawn an unstoppable goroutine.
func TestJobRunner_StopBeforeStart_IsNoOp(t *testing.T) {
	var count atomic.Int32
	runner := NewJobRunner(JobConfig{
		Name:           "stop-first",
		Interval:       time.Hour,
		RunImmediately: true,
	}, func(ctx context.Context) error {
		count.Add(1)
		return nil
	}, testLogger())

	runner.Stop()                      // stop before start
	runner.Start(context.Background()) // must be ignored

	time.Sleep(50 * time.Millisecond)

	if got := count.Load(); got != 0 {
		t.Fatalf("runner started after Stop executed %d times, want 0", got)
	}
}

// TestJobGroup_ConcurrentAddAndStopAll drives Add and StopAll concurrently so
// the -race detector flags the data race on JobRunner.cancel and the wg.Add /
// wg.Wait ordering that the finding calls out. Run with `go test -race`.
func TestJobGroup_ConcurrentAddAndStopAll(t *testing.T) {
	for iter := 0; iter < 20; iter++ {
		ctx := context.Background()
		group := NewJobGroup(ctx, testLogger())

		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				group.Add(NewJobRunner(JobConfig{
					Name:     "concurrent",
					Interval: time.Millisecond,
				}, func(ctx context.Context) error { return nil }, testLogger()))
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			group.StopAll()
		}()
		wg.Wait()

		// Final StopAll to reap anything that started after the racing one.
		group.StopAll()
	}
}
