package bootstrap

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"search-indexer/domain"
	"search-indexer/logger"
)

type fakeWarmupEngine struct {
	calls    atomic.Int32
	delay    time.Duration
	err      error
	gotQuery atomic.Value // string
}

func (f *fakeWarmupEngine) Search(ctx context.Context, query string, limit int) ([]domain.SearchDocument, error) {
	f.calls.Add(1)
	f.gotQuery.Store(query)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, f.err
}

// TestWarmupSearchEngine_CallsSearchOnce confirms warmup invokes Search exactly
// once. The probe is what brings the qwen3 embedding model into Ollama's
// GPU-resident set so the first user query no longer pays the ~1100ms model-load
// penalty.
func TestWarmupSearchEngine_CallsSearchOnce(t *testing.T) {
	logger.Init()
	eng := &fakeWarmupEngine{}
	warmupSearchEngine(context.Background(), eng)
	if got := eng.calls.Load(); got != 1 {
		t.Fatalf("Search call count = %d, want 1", got)
	}
}

// TestWarmupSearchEngine_ErrorDoesNotPanic_or_Block keeps the goroutine safe
// when news-creator-backend is down or the embedder DNS is failing: startup
// must continue regardless.
func TestWarmupSearchEngine_ErrorDoesNotPanicOrBlock(t *testing.T) {
	logger.Init()
	eng := &fakeWarmupEngine{err: errors.New("embedder unreachable")}
	done := make(chan struct{})
	go func() {
		defer close(done)
		warmupSearchEngine(context.Background(), eng)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("warmupSearchEngine blocked despite error")
	}
}

// TestWarmupSearchEngine_RespectsCancellation guards against the goroutine
// hanging past service shutdown.
func TestWarmupSearchEngine_RespectsCancellation(t *testing.T) {
	logger.Init()
	eng := &fakeWarmupEngine{delay: 5 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		warmupSearchEngine(ctx, eng)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("warmupSearchEngine did not honor cancelled context")
	}
}

// TestRunWarmupLoop_ProbesImmediatelyThenOnInterval guards the fix for the
// 2026-07-22 incident: a single startup-only probe is not enough because
// gemma4 (chat/RAG) and qwen3-embedding (hybrid search) were observed to
// exclusively swap GPU residency on this host, so the embedder goes cold
// again within minutes of the last chat request regardless of
// OLLAMA_KEEP_ALIVE. The loop must re-probe on an interval, starting
// immediately rather than waiting a full interval after process start.
func TestRunWarmupLoop_ProbesImmediatelyThenOnInterval(t *testing.T) {
	logger.Init()
	eng := &fakeWarmupEngine{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runWarmupLoop(ctx, eng, 20*time.Millisecond)

	deadline := time.After(2 * time.Second)
	for eng.calls.Load() < 3 {
		select {
		case <-deadline:
			t.Fatalf("warmup call count = %d after timeout, want >= 3", eng.calls.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestRunWarmupLoop_ErrorDoesNotStopLoop keeps re-probing on subsequent
// ticks even when the embedder is briefly unreachable -- a transient
// failure must not silently disable the safety net that keeps the model
// resident.
func TestRunWarmupLoop_ErrorDoesNotStopLoop(t *testing.T) {
	logger.Init()
	eng := &fakeWarmupEngine{err: errors.New("embedder unreachable")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runWarmupLoop(ctx, eng, 10*time.Millisecond)

	deadline := time.After(2 * time.Second)
	for eng.calls.Load() < 3 {
		select {
		case <-deadline:
			t.Fatalf("warmup call count = %d after timeout, want >= 3 despite errors", eng.calls.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestRunWarmupLoop_RespectsCancellation guards shutdown: the loop must
// exit promptly instead of leaking a goroutine past ctx cancellation.
func TestRunWarmupLoop_RespectsCancellation(t *testing.T) {
	logger.Init()
	eng := &fakeWarmupEngine{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		runWarmupLoop(ctx, eng, time.Hour)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runWarmupLoop did not honor a pre-cancelled context")
	}
}
