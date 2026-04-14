package utils

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// MED-3b: the limiter must block the second call long enough to satisfy the
// configured interval while leaving the first call unblocked.
func TestHostRateLimiter_EnforcesInterval(t *testing.T) {
	var mu sync.Mutex
	now := time.Unix(0, 0)
	slept := time.Duration(0)

	lim := NewHostRateLimiter(5 * time.Second)
	lim.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	lim.sleep = func(_ context.Context, d time.Duration) error {
		mu.Lock()
		slept += d
		now = now.Add(d)
		mu.Unlock()
		return nil
	}

	// First call: no wait.
	if err := lim.Wait(context.Background(), "news-creator"); err != nil {
		t.Fatalf("first call returned error: %v", err)
	}
	if slept != 0 {
		t.Fatalf("first call should not sleep, slept=%s", slept)
	}

	// Second call immediately after: must wait ~5s.
	if err := lim.Wait(context.Background(), "news-creator"); err != nil {
		t.Fatalf("second call returned error: %v", err)
	}
	if slept != 5*time.Second {
		t.Fatalf("second call should sleep 5s, slept=%s", slept)
	}
}

func TestHostRateLimiter_SeparatesHosts(t *testing.T) {
	lim := NewHostRateLimiter(5 * time.Second)
	slept := time.Duration(0)
	lim.sleep = func(_ context.Context, d time.Duration) error { slept += d; return nil }

	_ = lim.Wait(context.Background(), "host-a")
	_ = lim.Wait(context.Background(), "host-b")

	if slept != 0 {
		t.Fatalf("calls to different hosts must not block each other, slept=%s", slept)
	}
}

func TestHostRateLimiter_ZeroIntervalIsNoop(t *testing.T) {
	lim := NewHostRateLimiter(0)
	start := time.Now()
	_ = lim.Wait(context.Background(), "x")
	_ = lim.Wait(context.Background(), "x")
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("zero interval must be a no-op")
	}
}

func TestHostRateLimiter_ContextCancel(t *testing.T) {
	lim := NewHostRateLimiter(50 * time.Millisecond)
	// Warm up so the next Wait will need to sleep.
	_ = lim.Wait(context.Background(), "x")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before sleeping

	err := lim.Wait(ctx, "x")
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
