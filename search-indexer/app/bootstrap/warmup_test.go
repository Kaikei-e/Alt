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
