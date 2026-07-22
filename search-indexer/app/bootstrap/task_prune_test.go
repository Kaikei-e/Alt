package bootstrap

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"search-indexer/logger"
)

type fakeTaskPruner struct {
	calls       atomic.Int32
	err         error
	gotRetained atomic.Value // time.Duration
}

func (f *fakeTaskPruner) PruneTaskHistory(ctx context.Context, olderThan time.Duration) error {
	f.calls.Add(1)
	f.gotRetained.Store(olderThan)
	return f.err
}

// TestRunTaskPruneLoop_PrunesImmediatelyThenOnInterval guards the fix for the
// 2026-07-22 incident: registerBatchSynonyms's ever-growing full-replace PUT
// filled Meilisearch's task database (no_space_left_on_device), wedging all
// writes for four days. Pruning must not wait a full interval after a restart
// before the first run -- an operator restarting the service to recover from
// exactly this incident needs the prune to happen promptly.
func TestRunTaskPruneLoop_PrunesImmediatelyThenOnInterval(t *testing.T) {
	logger.Init()
	p := &fakeTaskPruner{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runTaskPruneLoop(ctx, p, 20*time.Millisecond, 72*time.Hour)

	deadline := time.After(2 * time.Second)
	for p.calls.Load() < 3 {
		select {
		case <-deadline:
			t.Fatalf("prune call count = %d after timeout, want >= 3", p.calls.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}

	if got := p.gotRetained.Load().(time.Duration); got != 72*time.Hour {
		t.Fatalf("retention passed to PruneTaskHistory = %v, want 72h", got)
	}
}

// TestRunTaskPruneLoop_ErrorDoesNotStopLoop keeps pruning on subsequent ticks
// even when Meilisearch is briefly unreachable -- a transient failure must not
// silently disable the safety net that prevents task-database exhaustion.
func TestRunTaskPruneLoop_ErrorDoesNotStopLoop(t *testing.T) {
	logger.Init()
	p := &fakeTaskPruner{err: errors.New("meilisearch unreachable")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runTaskPruneLoop(ctx, p, 10*time.Millisecond, time.Hour)

	deadline := time.After(2 * time.Second)
	for p.calls.Load() < 3 {
		select {
		case <-deadline:
			t.Fatalf("prune call count = %d after timeout, want >= 3 despite errors", p.calls.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestRunTaskPruneLoop_RespectsCancellation guards shutdown: the loop must
// exit promptly instead of leaking a goroutine past ctx cancellation.
func TestRunTaskPruneLoop_RespectsCancellation(t *testing.T) {
	logger.Init()
	p := &fakeTaskPruner{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		runTaskPruneLoop(ctx, p, time.Hour, 72*time.Hour)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runTaskPruneLoop did not honor a pre-cancelled context")
	}
}
