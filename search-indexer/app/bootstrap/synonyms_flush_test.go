package bootstrap

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"search-indexer/logger"
)

type fakeSynonymsFlusher struct {
	calls atomic.Int32
	err   error
}

func (f *fakeSynonymsFlusher) FlushSynonyms(ctx context.Context) error {
	f.calls.Add(1)
	return f.err
}

// TestRunSynonymsFlushLoop_FlushesImmediatelyThenOnInterval guards the fix
// for PM-2026-047 action item #2: registerBatchSynonyms only marks the
// synonyms union dirty, and this loop is the sole place that turns a dirty
// union into a Meilisearch PUT. It must not wait a full interval after
// process start before the first flush.
func TestRunSynonymsFlushLoop_FlushesImmediatelyThenOnInterval(t *testing.T) {
	logger.Init()
	f := &fakeSynonymsFlusher{}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		runSynonymsFlushLoop(ctx, f, 20*time.Millisecond)
	}()
	// Join the loop goroutine before returning: runSynonymsFlushLoop reads
	// the shared logger.Logger global on every tick, and leaving it running
	// past this test's return raced it against the next test's logger.Init()
	// write under `go test -race` (loop goroutine outlives cancel() by a
	// few microseconds while the runtime schedules the next test).
	t.Cleanup(func() {
		cancel()
		<-done
	})

	deadline := time.After(2 * time.Second)
	for f.calls.Load() < 3 {
		select {
		case <-deadline:
			t.Fatalf("flush call count = %d after timeout, want >= 3", f.calls.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestRunSynonymsFlushLoop_ErrorDoesNotStopLoop keeps flushing on subsequent
// ticks even when Meilisearch is briefly unreachable.
func TestRunSynonymsFlushLoop_ErrorDoesNotStopLoop(t *testing.T) {
	logger.Init()
	f := &fakeSynonymsFlusher{err: errors.New("meilisearch unreachable")}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		runSynonymsFlushLoop(ctx, f, 10*time.Millisecond)
	}()
	// See TestRunSynonymsFlushLoop_FlushesImmediatelyThenOnInterval: join the
	// loop goroutine before returning so it cannot race the next test's
	// logger.Init() against the shared logger.Logger global.
	t.Cleanup(func() {
		cancel()
		<-done
	})

	deadline := time.After(2 * time.Second)
	for f.calls.Load() < 3 {
		select {
		case <-deadline:
			t.Fatalf("flush call count = %d after timeout, want >= 3 despite errors", f.calls.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestRunSynonymsFlushLoop_RespectsCancellation guards shutdown: the loop
// must exit promptly instead of leaking a goroutine past ctx cancellation.
func TestRunSynonymsFlushLoop_RespectsCancellation(t *testing.T) {
	logger.Init()
	f := &fakeSynonymsFlusher{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		runSynonymsFlushLoop(ctx, f, time.Hour)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runSynonymsFlushLoop did not honor a pre-cancelled context")
	}
}
