package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSharedCache_Get_FreshAndExpired(t *testing.T) {
	var loads int32
	now := time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC)

	cache := NewSharedCache[string, string](time.Minute, time.Minute, func(ctx context.Context, key string) (string, error) {
		atomic.AddInt32(&loads, 1)
		return "value-" + key, nil
	})
	cache.now = func() time.Time { return now }

	got, err := cache.Get(context.Background(), "a")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "value-a" {
		t.Fatalf("Get() = %q, want value-a", got)
	}

	got, err = cache.Get(context.Background(), "a")
	if err != nil {
		t.Fatalf("Get() second error = %v", err)
	}
	if got != "value-a" {
		t.Fatalf("Get() second = %q, want value-a", got)
	}
	if atomic.LoadInt32(&loads) != 1 {
		t.Fatalf("loads = %d, want 1", loads)
	}

	now = now.Add(3 * time.Minute)
	got, err = cache.Get(context.Background(), "a")
	if err != nil {
		t.Fatalf("Get() expired error = %v", err)
	}
	if got != "value-a" {
		t.Fatalf("Get() expired = %q, want value-a", got)
	}
	if atomic.LoadInt32(&loads) != 2 {
		t.Fatalf("loads = %d, want 2 after expiration", loads)
	}
}

func TestSharedCache_Get_StaleWhileRevalidate(t *testing.T) {
	var loads int32
	now := time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC)
	refreshStarted := make(chan struct{}, 1)
	refreshRelease := make(chan struct{})

	cache := NewSharedCache[string, string](time.Minute, time.Minute, func(ctx context.Context, key string) (string, error) {
		call := atomic.AddInt32(&loads, 1)
		if call == 1 {
			return "initial", nil
		}
		refreshStarted <- struct{}{}
		<-refreshRelease
		return "refreshed", nil
	})
	cache.now = func() time.Time { return now }

	if _, err := cache.Get(context.Background(), "a"); err != nil {
		t.Fatalf("prime cache error = %v", err)
	}

	now = now.Add(90 * time.Second)
	got, err := cache.Get(context.Background(), "a")
	if err != nil {
		t.Fatalf("stale Get() error = %v", err)
	}
	if got != "initial" {
		t.Fatalf("stale Get() = %q, want initial", got)
	}

	select {
	case <-refreshStarted:
	case <-time.After(time.Second):
		t.Fatal("background refresh did not start")
	}
	close(refreshRelease)

	// The background goroutine still needs to run c.Set() after
	// refreshRelease unblocks it, so poll for the store to land instead of
	// guessing a sleep duration long enough to cover it.
	deadline := time.Now().Add(time.Second)
	var refreshedVal string
	var state CacheState
	for {
		refreshedVal, state = cache.Peek("a")
		if state == CacheStateFresh || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if state != CacheStateFresh {
		t.Fatalf("Peek() state = %v, want fresh", state)
	}
	if refreshedVal != "refreshed" {
		t.Fatalf("Peek() value = %q, want refreshed", refreshedVal)
	}
}

func TestSharedCache_Get_SingleflightDeduplicates(t *testing.T) {
	const goroutines = 8

	var loads int32
	var attempted int32
	release := make(chan struct{})
	cache := NewSharedCache[string, string](time.Minute, time.Minute, func(ctx context.Context, key string) (string, error) {
		atomic.AddInt32(&loads, 1)
		<-release
		return "shared", nil
	})

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			atomic.AddInt32(&attempted, 1)
			_, _ = cache.Get(context.Background(), "same")
		}()
	}

	// Wait deterministically until every goroutine has actually reached the
	// call to Get() before releasing the loader, instead of guessing a
	// sleep duration long enough for the scheduler to have run all of
	// them. Only a real scheduling starvation trips the timeout.
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&attempted) < goroutines {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for all %d goroutines to reach Get(), got %d", goroutines, atomic.LoadInt32(&attempted))
		}
		time.Sleep(time.Millisecond)
	}

	close(release)
	wg.Wait()

	if atomic.LoadInt32(&loads) != 1 {
		t.Fatalf("loads = %d, want 1", loads)
	}
}

// TestSharedCache_ConcurrentSetInvalidate exercises Set/Invalidate/Peek from
// many goroutines concurrently: run with -race, its main purpose is to
// surface any unsynchronized map access. It also asserts each key ends up in
// the state its own goroutine last wrote, so a regression that drops or
// corrupts writes (e.g. a lock ordering bug that silently loses a Set) fails
// the test even without -race.
func TestSharedCache_ConcurrentSetInvalidate(t *testing.T) {
	const n = 50
	cache := NewSharedCache[int, int](time.Minute, time.Minute, func(ctx context.Context, key int) (int, error) {
		return key * 2, nil
	})

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cache.Set(i, i)
			cache.Invalidate(i)
			cache.Set(i, i+1)
			_, _ = cache.Peek(i)
		}(i)
	}
	wg.Wait()

	// Each key i is only ever touched by goroutine i, and its last write is
	// Set(i, i+1), so the final state is deterministic despite the
	// concurrency above.
	for i := 0; i < n; i++ {
		got, state := cache.Peek(i)
		if state != CacheStateFresh {
			t.Errorf("key %d: Peek() state = %v, want fresh", i, state)
			continue
		}
		if got != i+1 {
			t.Errorf("key %d: Peek() value = %d, want %d", i, got, i+1)
		}
	}
}
