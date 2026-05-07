package job

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeNotification struct {
	err error
}

type fakeListener struct {
	mu            sync.Mutex
	notifications chan fakeNotification
	waitCalls     int
	closed        bool
}

func newFakeListener() *fakeListener {
	return &fakeListener{
		notifications: make(chan fakeNotification, 8),
	}
}

func (l *fakeListener) WaitForNotification(ctx context.Context) error {
	l.mu.Lock()
	l.waitCalls++
	l.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case n := <-l.notifications:
		return n.err
	}
}

func (l *fakeListener) Close(_ context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	return nil
}

func (l *fakeListener) pushNotification() {
	l.notifications <- fakeNotification{}
}

func (l *fakeListener) pushError(err error) {
	l.notifications <- fakeNotification{err: err}
}

func TestKnowledgeProjectorRunner_WakesOnNotification(t *testing.T) {
	listener := newFakeListener()
	processCh := make(chan time.Time, 8)

	runner := NewKnowledgeProjectorRunner(KnowledgeProjectorRunnerConfig{
		PollInterval: 2 * time.Second,
		Timeout:      500 * time.Millisecond,
		Process: func(ctx context.Context) error {
			select {
			case processCh <- time.Now():
			case <-ctx.Done():
			}
			return nil
		},
		ListenerFactory: func(context.Context) (KnowledgeProjectorListener, error) {
			return listener, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	<-processCh // initial drain on start
	start := time.Now()
	listener.pushNotification()

	select {
	case wokeAt := <-processCh:
		assert.Less(t, wokeAt.Sub(start), time.Second)
	case <-time.After(time.Second):
		t.Fatal("runner did not wake promptly on notification")
	}

	cancel()
	require.NoError(t, <-done)
}

func TestKnowledgeProjectorRunner_FallsBackToPollingOnListenerErrors(t *testing.T) {
	listener := newFakeListener()
	var mu sync.Mutex
	processCount := 0

	runner := NewKnowledgeProjectorRunner(KnowledgeProjectorRunnerConfig{
		PollInterval: 40 * time.Millisecond,
		Timeout:      20 * time.Millisecond,
		Process: func(context.Context) error {
			mu.Lock()
			processCount++
			mu.Unlock()
			return nil
		},
		ListenerFactory: func(context.Context) (KnowledgeProjectorListener, error) {
			return listener, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	listener.pushError(errors.New("listen failed"))
	time.Sleep(120 * time.Millisecond)
	cancel()

	require.NoError(t, <-done)

	mu.Lock()
	defer mu.Unlock()
	assert.GreaterOrEqual(t, processCount, 2, "expected initial run plus poll fallback")
}

func TestKnowledgeProjectorRunner_BoundedRetryRateOnPersistentFactoryErrors(t *testing.T) {
	var factoryCalls atomic.Int32

	runner := NewKnowledgeProjectorRunner(KnowledgeProjectorRunnerConfig{
		PollInterval:     500 * time.Millisecond,
		Timeout:          50 * time.Millisecond,
		InitialBackoff:   10 * time.Millisecond,
		MaxBackoff:       50 * time.Millisecond,
		BreakerThreshold: 100, // high — don't trip breaker in this test
		BreakerCooldown:  10 * time.Second,
		Process: func(context.Context) error {
			return nil
		},
		ListenerFactory: func(context.Context) (KnowledgeProjectorListener, error) {
			factoryCalls.Add(1)
			return nil, errors.New("factory unavailable")
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	time.Sleep(250 * time.Millisecond)
	cancel()
	require.NoError(t, <-done)

	calls := int(factoryCalls.Load())
	// With bounded backoff (10ms → 20 → 40 → 50 → 50 ...), we expect a small
	// bounded number of calls in 250ms — emphatically NOT a tight ms-scale loop.
	assert.GreaterOrEqual(t, calls, 2, "factory should still be retried")
	assert.LessOrEqual(t, calls, 20, "bounded backoff must cap retry rate (saw %d calls in 250ms)", calls)
}

func TestKnowledgeProjectorRunner_CircuitBreakerOpensAfterThreshold(t *testing.T) {
	var factoryCalls atomic.Int32

	runner := NewKnowledgeProjectorRunner(KnowledgeProjectorRunnerConfig{
		PollInterval:     500 * time.Millisecond,
		Timeout:          50 * time.Millisecond,
		InitialBackoff:   1 * time.Millisecond,
		MaxBackoff:       5 * time.Millisecond,
		BreakerThreshold: 3,
		BreakerCooldown:  500 * time.Millisecond, // longer than test window
		Process: func(context.Context) error {
			return nil
		},
		ListenerFactory: func(context.Context) (KnowledgeProjectorListener, error) {
			factoryCalls.Add(1)
			return nil, errors.New("factory unavailable")
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	// Window: long enough to hit threshold, short enough to stay inside the breaker cooldown.
	time.Sleep(150 * time.Millisecond)
	cancel()
	require.NoError(t, <-done)

	calls := int(factoryCalls.Load())
	// With threshold=3 and cooldown=500ms, after 3 failures the breaker opens
	// and additional factory calls should be rare (at most 1 per cooldown).
	assert.GreaterOrEqual(t, calls, 3, "should have reached threshold")
	assert.LessOrEqual(t, calls, 5, "circuit breaker must suppress retries during cooldown (saw %d calls)", calls)
}

func TestKnowledgeProjectorRunner_BackoffResetsOnSuccessfulListener(t *testing.T) {
	var factoryCalls atomic.Int32
	var failNextN atomic.Int32
	failNextN.Store(2) // first 2 factory calls fail, then succeed

	listener := newFakeListener()

	runner := NewKnowledgeProjectorRunner(KnowledgeProjectorRunnerConfig{
		PollInterval:     500 * time.Millisecond,
		Timeout:          50 * time.Millisecond,
		InitialBackoff:   10 * time.Millisecond,
		MaxBackoff:       100 * time.Millisecond,
		BreakerThreshold: 5,
		BreakerCooldown:  10 * time.Second,
		Process: func(context.Context) error {
			return nil
		},
		ListenerFactory: func(context.Context) (KnowledgeProjectorListener, error) {
			factoryCalls.Add(1)
			if failNextN.Load() > 0 {
				failNextN.Add(-1)
				return nil, errors.New("transient factory error")
			}
			return listener, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	// Wait long enough for 2 factory failures + 1 success + a few wait cycles.
	time.Sleep(200 * time.Millisecond)
	cancel()
	require.NoError(t, <-done)

	calls := int(factoryCalls.Load())
	// The breaker would have opened only if we hit 5 consecutive failures.
	// Since we succeed on the 3rd attempt, the breaker stays closed and
	// the listener stays attached for the rest of the run.
	assert.GreaterOrEqual(t, calls, 3, "should have eventually attached the listener")
	assert.Less(t, calls, 10, "no need for many factory calls once attached")
}

func TestKnowledgeProjectorRunner_BoundedRetryRateOnListenerWaitErrors(t *testing.T) {
	// This is the actual incident scenario: factory always succeeds (creates a
	// fresh listener), but the listener immediately returns a content-type-style
	// error from WaitForNotification. Without bounded backoff this becomes a
	// sub-millisecond log-flooding loop.
	var factoryCalls atomic.Int32

	runner := NewKnowledgeProjectorRunner(KnowledgeProjectorRunnerConfig{
		PollInterval:     500 * time.Millisecond,
		Timeout:          50 * time.Millisecond,
		InitialBackoff:   10 * time.Millisecond,
		MaxBackoff:       50 * time.Millisecond,
		BreakerThreshold: 100,
		BreakerCooldown:  10 * time.Second,
		Process: func(context.Context) error {
			return nil
		},
		ListenerFactory: func(context.Context) (KnowledgeProjectorListener, error) {
			factoryCalls.Add(1)
			l := newFakeListener()
			// Pre-load a fatal error so the next WaitForNotification returns immediately.
			l.pushError(errors.New("sovereign watch stream: invalid content-type"))
			return l, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	time.Sleep(250 * time.Millisecond)
	cancel()
	require.NoError(t, <-done)

	calls := int(factoryCalls.Load())
	assert.GreaterOrEqual(t, calls, 2, "should have retried after wait errors")
	assert.LessOrEqual(t, calls, 20, "bounded backoff must cap retry rate on wait errors (saw %d calls in 250ms)", calls)
}
