package job

import (
	"context"
	"errors"
	"sync"
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
