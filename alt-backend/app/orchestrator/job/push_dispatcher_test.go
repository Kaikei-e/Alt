package job

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestDispatchBackoff_ExponentialAndImmediateReset(t *testing.T) {
	b := newDispatchBackoff(dispatchInterval, maxDispatchInterval)

	if got := b.Current(); got != 2*time.Second {
		t.Fatalf("initial Current() = %v, want 2s", got)
	}

	// Empty passes double the interval up to maxDispatchInterval (10s).
	wantEmptySequence := []time.Duration{
		4 * time.Second,
		8 * time.Second,
		10 * time.Second,
		10 * time.Second,
	}
	for i, want := range wantEmptySequence {
		if got := b.Observe(0); got != want {
			t.Errorf("empty pass %d: Observe(0) = %v, want %v", i+1, got, want)
		}
		if got := b.Current(); got != want {
			t.Errorf("empty pass %d: Current() = %v, want %v", i+1, got, want)
		}
	}

	// A non-empty pass resets immediately to 2s.
	if got := b.Observe(3); got != 2*time.Second {
		t.Errorf("Observe(3) = %v, want 2s", got)
	}
	if got := b.Current(); got != 2*time.Second {
		t.Errorf("Current() after reset = %v, want 2s", got)
	}

	// Next empty pass starts doubling from 2s -> 4s again.
	if got := b.Observe(0); got != 4*time.Second {
		t.Errorf("Observe(0) after reset = %v, want 4s", got)
	}
}

func TestJobScheduler_NextInterval_UsesDynamicBackoff(t *testing.T) {
	b := newDispatchBackoff(5*time.Millisecond, 20*time.Millisecond)
	var runs atomic.Int32
	done := make(chan struct{})

	scheduler := NewJobScheduler()
	scheduler.Add(Job{
		Name:         "dynamic-backoff-test",
		Interval:     5 * time.Millisecond,
		Timeout:      time.Second,
		NextInterval: b.Current,
		Fn: func(ctx context.Context) error {
			n := runs.Add(1)
			if n < 3 {
				b.Observe(0)
			} else {
				b.Observe(1)
				select {
				case <-done:
				default:
					close(done)
				}
			}
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	waitSignal(t, done, 2*time.Second, "three dynamic backoff passes")
	cancel()
	scheduler.Shutdown()

	if got := b.Current(); got != 5*time.Millisecond {
		t.Errorf("Current() after non-empty pass = %v, want 5ms", got)
	}
}
