package job

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/di"
	"alt/domain"
	"alt/orchestrator/port/push_dispatch_port"
	"alt/orchestrator/usecase/push_dispatch_usecase"
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

type stubDeliveryPort struct {
	batch         []domain.PushDelivery
	claimErr      error
	backlogAge    time.Duration
	pending       int64
	subscriptions int64
	backlogErr    error
	backlogCalls  atomic.Int32
}

func (s *stubDeliveryPort) ClaimBatch(_ context.Context, _ int) ([]domain.PushDelivery, error) {
	return s.batch, s.claimErr
}

func (s *stubDeliveryPort) MarkSent(_ context.Context, _ string, _ int) error { return nil }
func (s *stubDeliveryPort) MarkDead(_ context.Context, _ string, _ int, _ string) error {
	return nil
}
func (s *stubDeliveryPort) Release(_ context.Context, _ string, _ time.Duration, _ int, _ string) error {
	return nil
}

func (s *stubDeliveryPort) BacklogAge(_ context.Context) (time.Duration, int64, int64, error) {
	s.backlogCalls.Add(1)
	if s.backlogErr != nil {
		return 0, 0, 0, s.backlogErr
	}
	return s.backlogAge, s.pending, s.subscriptions, nil
}

type stubSubscriptionPort struct{}

func (stubSubscriptionPort) DeleteByEndpoint(_ context.Context, _, _ string) error { return nil }

type stubSenderPort struct {
	outcome push_dispatch_port.SendOutcome
}

func (s stubSenderPort) Send(_ context.Context, _ push_dispatch_port.SendRequest) (push_dispatch_port.SendOutcome, error) {
	return s.outcome, nil
}

func TestRegisterNotifierJobs_PanicsWhenDispatchUnwired(t *testing.T) {
	scheduler := NewJobScheduler()
	assert.Panics(t, func() {
		RegisterNotifierJobs(scheduler, nil)
	})
	assert.Panics(t, func() {
		RegisterNotifierJobs(scheduler, &di.NotifierComponents{Dispatch: nil})
	})
}

func TestRegisterNotifierJobs_ExecutesPassAndPublishesBacklogAge(t *testing.T) {
	deliveries := &stubDeliveryPort{
		backlogAge:    45 * time.Second,
		pending:       3,
		subscriptions: 2,
	}
	uc := push_dispatch_usecase.New(deliveries, stubSubscriptionPort{}, stubSenderPort{})
	scheduler := NewJobScheduler()
	RegisterNotifierJobs(scheduler, &di.NotifierComponents{Dispatch: uc})

	require.Len(t, scheduler.jobs, 1)
	assert.Equal(t, "push-dispatcher", scheduler.jobs[0].Name)
	assert.Equal(t, dispatchInterval, scheduler.jobs[0].Interval)
	assert.Equal(t, dispatchTimeout, scheduler.jobs[0].Timeout)

	err := scheduler.jobs[0].Fn(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(1), deliveries.backlogCalls.Load())
}

func TestPublishBacklogAge_LogsWarnOnErrorWithoutPanicking(t *testing.T) {
	initDispatchMetrics()

	var logBuf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logBuf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	deliveries := &stubDeliveryPort{
		backlogErr: errors.New("datahub unreachable"),
	}
	uc := push_dispatch_usecase.New(deliveries, stubSubscriptionPort{}, stubSenderPort{})

	publishBacklogAge(context.Background(), uc)
	assert.Contains(t, logBuf.String(), `"msg":"push_backlog_age_unavailable"`)
	assert.Contains(t, logBuf.String(), "datahub unreachable")
}

func TestRecordDispatchPass_LogsPassAndClaimFailures(t *testing.T) {
	initDispatchMetrics()

	t.Run("claim failure logs error", func(t *testing.T) {
		var logBuf bytes.Buffer
		prev := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&logBuf, nil)))
		t.Cleanup(func() { slog.SetDefault(prev) })

		recordDispatchPass(context.Background(), push_dispatch_usecase.Stats{}, errors.New("claim rpc failed"))
		assert.Contains(t, logBuf.String(), `"msg":"push_dispatch_claim_failed"`)
		assert.Contains(t, logBuf.String(), "claim rpc failed")
	})

	t.Run("non-empty batch logs pass summary and records attempts", func(t *testing.T) {
		var logBuf bytes.Buffer
		prev := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&logBuf, nil)))
		t.Cleanup(func() { slog.SetDefault(prev) })

		stats := push_dispatch_usecase.Stats{
			Claimed:  2,
			Sent:     1,
			Dead:     1,
			Attempts: []push_dispatch_usecase.Attempt{{StatusCode: 201, Result: "sent"}, {StatusCode: 401, Result: "dead"}},
		}
		recordDispatchPass(context.Background(), stats, nil)
		logged := logBuf.String()
		assert.Contains(t, logged, `"msg":"push_dispatch_pass"`)
		assert.Contains(t, logged, `"claimed":2`)
		assert.Contains(t, logged, `"sent":1`)
		assert.Contains(t, logged, `"dead":1`)
	})
}
