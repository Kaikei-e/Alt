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

func TestHostRateLimiter_SleepFailureRollsBackReservation(t *testing.T) {
	var mu sync.Mutex
	now := time.Unix(0, 0)
	sleepCalls := 0

	lim := NewHostRateLimiter(5 * time.Second)
	lim.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	lim.sleep = func(_ context.Context, _ time.Duration) error {
		sleepCalls++
		return context.Canceled
	}

	// First call reserves immediately (no sleep).
	if err := lim.Wait(context.Background(), "host"); err != nil {
		t.Fatalf("first Wait: %v", err)
	}

	// Second call should sleep, fail, and roll back the reserved slot.
	if err := lim.Wait(context.Background(), "host"); !errors.Is(err, context.Canceled) {
		t.Fatalf("second Wait want Canceled, got %v", err)
	}
	if sleepCalls != 1 {
		t.Fatalf("expected one sleep attempt, got %d", sleepCalls)
	}

	// Third call with a working sleep must still wait the full interval from
	// the successful first call — not from the cancelled reservation.
	slept := time.Duration(0)
	lim.sleep = func(_ context.Context, d time.Duration) error {
		slept = d
		mu.Lock()
		now = now.Add(d)
		mu.Unlock()
		return nil
	}
	if err := lim.Wait(context.Background(), "host"); err != nil {
		t.Fatalf("third Wait: %v", err)
	}
	if slept != 5*time.Second {
		t.Fatalf("after cancelled wait, next Wait should sleep full interval, slept=%s", slept)
	}
}
