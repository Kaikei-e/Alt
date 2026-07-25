package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeClock is a Clock whose Sleep does not really block -- it just
// advances a virtual "now" by the requested duration (or, if ctx is
// already done, returns ctx.Err() without advancing). This lets WaitReady
// tests exercise many poll iterations and even minutes of virtual timeout
// instantly and deterministically.
type fakeClock struct {
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(0, 0)}
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	c.now = c.now.Add(d)
	return nil
}

// sequencePoller returns a Poller that yields each element of seq in turn,
// repeating the last element forever once exhausted. It also records how
// many times it was called.
func sequencePoller(seq [][]ServiceStatus) (Poller, *int) {
	calls := 0
	return func(ctx context.Context) ([]ServiceStatus, error) {
		defer func() { calls++ }()
		idx := calls
		if idx >= len(seq) {
			idx = len(seq) - 1
		}
		return seq[idx], nil
	}, &calls
}

func TestWaitReady_NoHealthcheck_ReadyAssoonAsRunning(t *testing.T) {
	poller, calls := sequencePoller([][]ServiceStatus{
		{{Name: "alt-backend", State: "running"}},
	})
	w := &Waiter{Poll: poller, Clock: newFakeClock()}

	result, err := w.WaitReady(context.Background(), []Target{{Service: "alt-backend", Stack: "core"}}, Options{Timeout: time.Minute})
	if err != nil {
		t.Fatalf("WaitReady returned error: %v", err)
	}
	if !result.Ready {
		t.Fatalf("expected Ready=true, got states=%+v", result.States)
	}
	if *calls != 1 {
		t.Errorf("expected exactly 1 poll when already running, got %d", *calls)
	}
	if len(result.States) != 1 || !result.States[0].Ready || result.States[0].Reason != "running" {
		t.Errorf("unexpected state: %+v", result.States)
	}
}

func TestWaitReady_HealthcheckBecomesHealthyBeforeTimeout(t *testing.T) {
	poller, calls := sequencePoller([][]ServiceStatus{
		{{Name: "rerank-local", State: "running", Health: "starting"}},
		{{Name: "rerank-local", State: "running", Health: "starting"}},
		{{Name: "rerank-local", State: "running", Health: "healthy"}},
	})

	var progressCalls int
	w := &Waiter{Poll: poller, Clock: newFakeClock()}
	opts := Options{
		Timeout:      time.Minute,
		PollInterval: 2 * time.Second,
		OnProgress:   func(states []State) { progressCalls++ },
	}

	result, err := w.WaitReady(context.Background(), []Target{{Service: "rerank-local", Stack: "ai"}}, opts)
	if err != nil {
		t.Fatalf("WaitReady returned error: %v", err)
	}
	if !result.Ready {
		t.Fatalf("expected Ready=true eventually, got states=%+v", result.States)
	}
	if *calls != 3 {
		t.Errorf("expected 3 polls, got %d", *calls)
	}
	if progressCalls != 3 {
		t.Errorf("expected OnProgress called once per poll (3), got %d", progressCalls)
	}
}

func TestWaitReady_TimesOutWhenStuckStarting(t *testing.T) {
	poller, _ := sequencePoller([][]ServiceStatus{
		{{Name: "alt-backend", State: "running", Health: "starting"}},
	})
	w := &Waiter{Poll: poller, Clock: newFakeClock()}

	result, err := w.WaitReady(context.Background(), []Target{{Service: "alt-backend", Stack: "core"}}, Options{
		Timeout:      5 * time.Second,
		PollInterval: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("WaitReady returned error: %v", err)
	}
	if result.Ready {
		t.Fatalf("expected Ready=false on timeout")
	}
	if !result.TimedOut {
		t.Errorf("expected TimedOut=true")
	}
	if len(result.States) != 1 || result.States[0].Reason != "health: starting" {
		t.Errorf("unexpected states: %+v", result.States)
	}
}

func TestWaitReady_OneShotExitZeroIsReadyRegardlessOfHealthcheck(t *testing.T) {
	poller, calls := sequencePoller([][]ServiceStatus{
		{{Name: "migrator", State: "exited", ExitCode: 0}},
	})
	w := &Waiter{Poll: poller, Clock: newFakeClock()}

	result, err := w.WaitReady(context.Background(), []Target{{Service: "migrator", Stack: "db"}}, Options{Timeout: time.Minute})
	if err != nil {
		t.Fatalf("WaitReady returned error: %v", err)
	}
	if !result.Ready {
		t.Fatalf("expected one-shot exited(0) to be Ready, got %+v", result.States)
	}
	if *calls != 1 {
		t.Errorf("expected exactly 1 poll, got %d", *calls)
	}
	if result.States[0].Reason != "exited(0)" {
		t.Errorf("expected reason exited(0), got %q", result.States[0].Reason)
	}
}

func TestWaitReady_OneShotExitNonZeroNeverReady(t *testing.T) {
	poller, _ := sequencePoller([][]ServiceStatus{
		{{Name: "migrator", State: "exited", ExitCode: 1}},
	})
	w := &Waiter{Poll: poller, Clock: newFakeClock()}

	result, err := w.WaitReady(context.Background(), []Target{{Service: "migrator", Stack: "db"}}, Options{
		Timeout:      4 * time.Second,
		PollInterval: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("WaitReady returned error: %v", err)
	}
	if result.Ready || !result.TimedOut {
		t.Fatalf("expected timeout with non-zero exit, got %+v", result)
	}
	if result.States[0].Reason != "exited(1)" {
		t.Errorf("expected reason exited(1), got %q", result.States[0].Reason)
	}
}

func TestWaitReady_MissingServiceReportsMissing(t *testing.T) {
	poller, _ := sequencePoller([][]ServiceStatus{
		{},
	})
	w := &Waiter{Poll: poller, Clock: newFakeClock()}

	result, err := w.WaitReady(context.Background(), []Target{{Service: "search-indexer", Stack: "workers"}}, Options{
		Timeout:      2 * time.Second,
		PollInterval: 1 * time.Second,
	})
	if err != nil {
		t.Fatalf("WaitReady returned error: %v", err)
	}
	if result.Ready {
		t.Fatalf("expected not ready for a missing service")
	}
	if result.States[0].Reason != "missing" {
		t.Errorf("expected reason missing, got %q", result.States[0].Reason)
	}
}

func TestWaitReady_MultipleTargets_AllMustBeReady(t *testing.T) {
	poller, calls := sequencePoller([][]ServiceStatus{
		{
			{Name: "a", State: "running"},
			{Name: "b", State: "running", Health: "starting"},
		},
		{
			{Name: "a", State: "running"},
			{Name: "b", State: "running", Health: "healthy"},
		},
	})
	w := &Waiter{Poll: poller, Clock: newFakeClock()}

	result, err := w.WaitReady(context.Background(), []Target{
		{Service: "a", Stack: "core"},
		{Service: "b", Stack: "ai"},
	}, Options{Timeout: time.Minute, PollInterval: time.Second})
	if err != nil {
		t.Fatalf("WaitReady returned error: %v", err)
	}
	if !result.Ready {
		t.Fatalf("expected all-ready, got %+v", result.States)
	}
	if *calls != 2 {
		t.Errorf("expected 2 polls, got %d", *calls)
	}
}

func TestWaitReady_PollErrorPropagatesImmediately(t *testing.T) {
	wantErr := errors.New("docker compose ps failed")
	poller := func(ctx context.Context) ([]ServiceStatus, error) {
		return nil, wantErr
	}
	w := &Waiter{Poll: poller, Clock: newFakeClock()}

	_, err := w.WaitReady(context.Background(), []Target{{Service: "a"}}, Options{Timeout: time.Minute})
	if err == nil {
		t.Fatal("expected error from failing poller, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected wrapped %v, got %v", wantErr, err)
	}
}

func TestWaitReady_ContextCancelledDuringSleepReturnsPromptly(t *testing.T) {
	poller, calls := sequencePoller([][]ServiceStatus{
		{{Name: "a", State: "running", Health: "starting"}},
	})
	ctx, cancel := context.WithCancel(context.Background())

	w := &Waiter{Poll: poller, Clock: newFakeClock()}
	opts := Options{
		Timeout:      time.Hour,
		PollInterval: time.Second,
		OnProgress: func(states []State) {
			// Cancel right after the first poll, so the Waiter should
			// observe cancellation in the following Sleep and return
			// promptly instead of looping until the (long) timeout.
			cancel()
		},
	}

	result, err := w.WaitReady(ctx, []Target{{Service: "a", Stack: "core"}}, opts)
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if result.Ready {
		t.Errorf("expected Ready=false on cancellation")
	}
	if *calls != 1 {
		t.Errorf("expected exactly 1 poll before cancellation was observed, got %d", *calls)
	}
}

func TestWaitReady_ZeroTimeoutMeansNoInternalDeadline(t *testing.T) {
	poller, calls := sequencePoller([][]ServiceStatus{
		{{Name: "a", State: "running", Health: "starting"}},
		{{Name: "a", State: "running", Health: "starting"}},
		{{Name: "a", State: "running", Health: "healthy"}},
	})
	w := &Waiter{Poll: poller, Clock: newFakeClock()}

	result, err := w.WaitReady(context.Background(), []Target{{Service: "a"}}, Options{PollInterval: time.Millisecond})
	if err != nil {
		t.Fatalf("WaitReady returned error: %v", err)
	}
	if !result.Ready {
		t.Fatalf("expected eventual readiness with Timeout=0, got %+v", result.States)
	}
	if *calls != 3 {
		t.Errorf("expected 3 polls, got %d", *calls)
	}
}

func TestEvaluateOne_Reasons(t *testing.T) {
	cases := []struct {
		name   string
		status ServiceStatus
		ready  bool
		reason string
	}{
		{"running-no-healthcheck", ServiceStatus{Name: "svc", State: "running"}, true, "running"},
		{"running-healthy", ServiceStatus{Name: "svc", State: "running", Health: "healthy"}, true, "healthy"},
		{"running-starting", ServiceStatus{Name: "svc", State: "running", Health: "starting"}, false, "health: starting"},
		{"running-unhealthy", ServiceStatus{Name: "svc", State: "running", Health: "unhealthy"}, false, "health: unhealthy"},
		{"restarting", ServiceStatus{Name: "svc", State: "restarting"}, false, "restarting"},
		{"created", ServiceStatus{Name: "svc", State: "created"}, false, "created"},
		{"exited-zero", ServiceStatus{Name: "svc", State: "exited", ExitCode: 0}, true, "exited(0)"},
		{"exited-nonzero", ServiceStatus{Name: "svc", State: "exited", ExitCode: 137}, false, "exited(137)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			statuses := map[string]ServiceStatus{tc.status.Name: tc.status}
			got := evaluateOne(Target{Service: tc.status.Name, Stack: "s"}, statuses)
			if got.Ready != tc.ready {
				t.Errorf("Ready = %v, want %v", got.Ready, tc.ready)
			}
			if got.Reason != tc.reason {
				t.Errorf("Reason = %q, want %q", got.Reason, tc.reason)
			}
		})
	}
}

func TestEvaluate_SortsByServiceName(t *testing.T) {
	statuses := []ServiceStatus{
		{Name: "zeta", State: "running"},
		{Name: "alpha", State: "running"},
	}
	states := evaluate([]Target{{Service: "zeta"}, {Service: "alpha"}}, statuses)
	if len(states) != 2 || states[0].Service != "alpha" || states[1].Service != "zeta" {
		t.Errorf("expected sorted [alpha zeta], got %+v", states)
	}
}
