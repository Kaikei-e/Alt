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

	time.Sleep(20 * time.Millisecond)
	got, state := cache.Peek("a")
	if state != CacheStateFresh {
		t.Fatalf("Peek() state = %v, want fresh", state)
	}
	if got != "refreshed" {
		t.Fatalf("Peek() value = %q, want refreshed", got)
	}
}

func TestSharedCache_Get_SingleflightDeduplicates(t *testing.T) {
	var loads int32
	release := make(chan struct{})
	cache := NewSharedCache[string, string](time.Minute, time.Minute, func(ctx context.Context, key string) (string, error) {
		atomic.AddInt32(&loads, 1)
		<-release
		return "shared", nil
	})

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = cache.Get(context.Background(), "same")
		}()
	}

	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	if atomic.LoadInt32(&loads) != 1 {
		t.Fatalf("loads = %d, want 1", loads)
	}
}

func TestSharedCache_ConcurrentSetInvalidate(t *testing.T) {
	cache := NewSharedCache[int, int](time.Minute, time.Minute, func(ctx context.Context, key int) (int, error) {
		return key * 2, nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
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
}
