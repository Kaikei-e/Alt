package driver

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSingleflightCoalescer_DedupesConcurrentMiss asserts that N goroutines
// hammering the same key while one Meilisearch call is in flight only trigger
// the underlying compute once. RAG was observed sending 7 identical SearchArticles
// calls in <300ms in production traces (2026-05-24 03:31); without dedupe each
// call pays the full hybrid-search cost.
func TestSingleflightCoalescer_DedupesConcurrentMiss(t *testing.T) {
	d := &MeilisearchDriver{}
	var calls atomic.Int32
	fn := func() (cacheEntry, error) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond) // hold the in-flight window open
		return cacheEntry{EstimatedTotal: 7}, nil
	}

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	var totalsMu sync.Mutex
	var totals []int64
	for range N {
		go func() {
			defer wg.Done()
			e, err := d.singleflightSearch(context.Background(), "k", fn)
			if err != nil {
				t.Errorf("unexpected err: %v", err)
				return
			}
			totalsMu.Lock()
			totals = append(totals, e.EstimatedTotal)
			totalsMu.Unlock()
		}()
	}
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("compute calls = %d, want 1", got)
	}
	for _, v := range totals {
		if v != 7 {
			t.Errorf("got EstimatedTotal=%d, want 7", v)
		}
	}
}

// TestSingleflightCoalescer_CtxCancelDoesNotStarveOthers ensures one
// caller's context cancellation does not abort the underlying compute
// for other waiters sharing the same key.
func TestSingleflightCoalescer_CtxCancelDoesNotStarveOthers(t *testing.T) {
	d := &MeilisearchDriver{}
	var calls atomic.Int32
	fn := func() (cacheEntry, error) {
		calls.Add(1)
		time.Sleep(100 * time.Millisecond)
		return cacheEntry{EstimatedTotal: 9}, nil
	}

	ctxA, cancelA := context.WithCancel(context.Background())
	ctxB := context.Background()

	resultB := make(chan cacheEntry, 1)
	errB := make(chan error, 1)
	go func() {
		e, err := d.singleflightSearch(ctxB, "shared", fn)
		resultB <- e
		errB <- err
	}()

	// Give B time to enter singleflight before A starts and cancels.
	time.Sleep(20 * time.Millisecond)

	resultA := make(chan error, 1)
	go func() {
		_, err := d.singleflightSearch(ctxA, "shared", fn)
		resultA <- err
	}()

	// Cancel A quickly. B must still complete normally.
	time.Sleep(10 * time.Millisecond)
	cancelA()

	select {
	case err := <-resultA:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("A: expected ctx canceled, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("A: did not return")
	}

	select {
	case e := <-resultB:
		if e.EstimatedTotal != 9 {
			t.Errorf("B: EstimatedTotal = %d, want 9", e.EstimatedTotal)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("B: did not return")
	}

	if got := calls.Load(); got != 1 {
		t.Errorf("compute calls = %d, want 1 (singleflight should dedupe)", got)
	}
}
