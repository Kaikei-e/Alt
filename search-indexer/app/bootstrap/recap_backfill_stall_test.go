package bootstrap

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"search-indexer/domain"
	"search-indexer/usecase"
)

// stalledRecapRepo always returns a full page of documents that share one
// ExecutedAt with HasMore=true, reproducing recap-worker's inclusive, untied
// `since` bound when a single job's tie group is larger than one page: the
// cursor can never advance, so the batch is served again on every fetch. A
// safety cap trips hotLoop and cancels the context if the loop calls it far
// more often than the stall guard should ever allow -- that is the failure the
// guard exists to prevent, and lets this test terminate against buggy code
// instead of spinning forever.
type stalledRecapRepo struct {
	calls     int64
	hotLoop   atomic.Bool
	cancel    context.CancelFunc
	safetyCap int64
}

func (r *stalledRecapRepo) GetIndexableGenres(ctx context.Context, since string, limit int) ([]domain.RecapDocument, bool, error) {
	if atomic.AddInt64(&r.calls, 1) > r.safetyCap {
		r.hotLoop.Store(true)
		if r.cancel != nil {
			r.cancel()
		}
		return nil, false, nil
	}
	// A page of docs all sharing the same ExecutedAt, truncated (HasMore=true).
	docs := []domain.RecapDocument{
		{ID: "job1__a", JobID: "job1", ExecutedAt: "2026-08-22T00:00:05Z", Genre: "a"},
		{ID: "job1__b", JobID: "job1", ExecutedAt: "2026-08-22T00:00:05Z", Genre: "b"},
	}
	return docs, true, nil
}

type noopRecapSearchEngine struct{}

func (noopRecapSearchEngine) EnsureRecapIndex(ctx context.Context) error { return nil }
func (noopRecapSearchEngine) IndexRecapDocuments(ctx context.Context, docs []domain.RecapDocument) error {
	return nil
}
func (noopRecapSearchEngine) SearchRecaps(ctx context.Context, query string, limit int) ([]domain.RecapDocument, int64, error) {
	return nil, 0, nil
}

// TestRunRecapIndexLoop_DoesNotHotSpinOnStalledCursor asserts that Phase 1
// gives up (advancing to Phase 2, which then blocks on its poll interval)
// instead of looping forever on a cursor that never advances. Without the
// stall guard the loop calls GetIndexableGenres tens of thousands of times
// with zero delay and this test trips the safety cap.
func TestRunRecapIndexLoop_DoesNotHotSpinOnStalledCursor(t *testing.T) {
	// Shrink the inter-iteration pause so the (bounded) stall run is fast.
	prevPause := recapBackfillStallPause
	recapBackfillStallPause = time.Millisecond
	t.Cleanup(func() { recapBackfillStallPause = prevPause })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo := &stalledRecapRepo{cancel: cancel, safetyCap: 5000}
	uc := usecase.NewIndexRecapsUsecase(repo, noopRecapSearchEngine{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runRecapIndexLoop(ctx, uc)
	}()

	// Give Phase 1 time to exhaust the stall budget and reach Phase 2 (which
	// blocks on config.RecapIndexInterval), then stop the loop.
	time.Sleep(300 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runRecapIndexLoop did not return after context cancel")
	}

	if repo.hotLoop.Load() {
		t.Fatal("Phase 1 hot-spun on a stalled cursor: GetIndexableGenres exceeded the safety cap")
	}
	// Phase 1 should stop after ~recapBackfillMaxStalls no-progress iterations,
	// then Phase 2 makes at most a handful of polls before the interval sleep.
	// A generous bound still catches an unbounded hot loop.
	if calls := atomic.LoadInt64(&repo.calls); calls > recapBackfillMaxStalls+50 {
		t.Fatalf("GetIndexableGenres called %d times; want it bounded by the stall guard (~%d)", calls, recapBackfillMaxStalls)
	}
}
