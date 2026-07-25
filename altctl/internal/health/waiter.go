// Package health evaluates Docker Compose service readiness and polls
// until every target service reaches it (or a timeout/cancellation stops
// waiting).
//
// "Ready" (altctl reliability plan, "trustworthy success") means:
//   - a service with a healthcheck is Ready when State is running AND
//     Health is healthy;
//   - a service with no healthcheck is Ready when State is running;
//   - a one-shot container (a migrator/init job that is expected to exit)
//     is Ready when State is exited AND ExitCode is 0 -- regardless of any
//     healthcheck.
//
// The Waiter polls an injectable Poller on an injectable Clock so it can be
// driven deterministically in tests without a live Docker daemon or real
// sleeps; production callers (cmd/up.go, cmd/restart.go) wire Poller to
// compose.Client.PS and leave Clock nil (defaulting to SystemClock).
package health

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Target is one service whose readiness the Waiter should track, plus the
// stack it belongs to (carried through purely for diagnostics/reporting).
type Target struct {
	Service string
	Stack   string
}

// ServiceStatus is the raw per-service status polled from `docker compose
// ps`. It intentionally mirrors compose.ServiceStatus's relevant fields
// rather than importing internal/compose, so this package has no
// dependency on the compose executor and stays reusable (e.g. by a future
// rebuild command) and independently testable.
type ServiceStatus struct {
	Name     string
	State    string
	Health   string
	ExitCode int
}

// Poller fetches the current status of every service compose knows about
// for the file set the caller is waiting on (normally backed by
// compose.Client.PS against the same -f files passed to `up`/`restart`).
type Poller func(ctx context.Context) ([]ServiceStatus, error)

// Clock abstracts time so the poll loop can be driven deterministically in
// tests without real sleeps.
type Clock interface {
	Now() time.Time
	// Sleep blocks for d or until ctx is done, whichever comes first. It
	// returns ctx.Err() when interrupted by cancellation.
	Sleep(ctx context.Context, d time.Duration) error
}

// SystemClock is the production Clock, backed by real wall-clock time.
type SystemClock struct{}

// Now returns the current wall-clock time.
func (SystemClock) Now() time.Time { return time.Now() }

// Sleep blocks for d or until ctx is done.
func (SystemClock) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// State is one target's readiness as of the most recent poll.
type State struct {
	Service string
	Stack   string
	Ready   bool
	// Reason is a short human-readable explanation of the current
	// (non-)ready state: "running", "healthy", "health: starting",
	// "health: unhealthy", "exited(0)", "exited(1)", "missing", or the raw
	// compose State ("restarting", "created", ...) for anything else.
	Reason string
}

// Options configures a WaitReady call.
type Options struct {
	// Timeout bounds the whole wait, measured against Clock.Now(). Zero
	// means no internal deadline -- WaitReady then only stops when every
	// target becomes Ready or ctx is done.
	Timeout time.Duration
	// PollInterval is how often to re-poll. Defaults to 2s.
	PollInterval time.Duration
	// OnProgress, if set, is invoked synchronously after every poll
	// (including the first) with the full per-target state, so callers can
	// render live progress (e.g. "12/17 Ready -- waiting: ..."). It must
	// not block for long -- it runs inline in the poll loop.
	OnProgress func(states []State)
}

// Result is the outcome of a WaitReady call.
type Result struct {
	Ready    bool
	TimedOut bool
	States   []State // sorted by Service; populated even on timeout, always empty on a Poll error
}

// Waiter polls for target readiness. The zero value is not usable directly
// for production use (Poll must be set); NewWaiter is the normal
// constructor.
type Waiter struct {
	Poll  Poller
	Clock Clock
}

// NewWaiter builds a Waiter backed by poll and the production SystemClock.
func NewWaiter(poll Poller) *Waiter {
	return &Waiter{Poll: poll, Clock: SystemClock{}}
}

// WaitReady polls Poll every Options.PollInterval until every target in
// targets is Ready, Options.Timeout elapses (if set), or ctx is cancelled.
//
// A Poll error is returned immediately, wrapped -- it means the underlying
// `docker compose ps` itself failed, which is not a "still starting"
// condition and must not be retried silently.
//
// A ctx cancellation (e.g. Ctrl-C) is detected promptly: it is checked
// before every Sleep, so the loop never blocks for a full PollInterval past
// cancellation. The returned error is ctx.Err() (wrapped) in that case, and
// Result carries the last-observed States for diagnostics.
func (w *Waiter) WaitReady(ctx context.Context, targets []Target, opts Options) (Result, error) {
	pollInterval := opts.PollInterval
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	clock := w.Clock
	if clock == nil {
		clock = SystemClock{}
	}

	start := clock.Now()

	for {
		if err := ctx.Err(); err != nil {
			return Result{}, fmt.Errorf("waiting for services to become ready: %w", err)
		}

		statuses, err := w.Poll(ctx)
		if err != nil {
			return Result{}, fmt.Errorf("polling service status: %w", err)
		}

		states := evaluate(targets, statuses)
		if opts.OnProgress != nil {
			opts.OnProgress(states)
		}

		if allReady(states) {
			return Result{Ready: true, States: states}, nil
		}

		if opts.Timeout > 0 && clock.Now().Sub(start) >= opts.Timeout {
			return Result{Ready: false, TimedOut: true, States: states}, nil
		}

		if err := clock.Sleep(ctx, pollInterval); err != nil {
			return Result{Ready: false, States: states}, fmt.Errorf("waiting for services to become ready: %w", err)
		}
	}
}

func allReady(states []State) bool {
	for _, s := range states {
		if !s.Ready {
			return false
		}
	}
	return true
}

// evaluate classifies every target against the latest poll of statuses,
// returning States sorted by Service for deterministic rendering/testing.
func evaluate(targets []Target, statuses []ServiceStatus) []State {
	byName := make(map[string]ServiceStatus, len(statuses))
	for _, s := range statuses {
		byName[s.Name] = s
	}

	states := make([]State, 0, len(targets))
	for _, t := range targets {
		states = append(states, evaluateOne(t, byName))
	}
	sort.Slice(states, func(i, j int) bool { return states[i].Service < states[j].Service })
	return states
}

// evaluateOne applies the Ready rules (see package doc) to a single target
// against the latest poll.
func evaluateOne(target Target, statuses map[string]ServiceStatus) State {
	st, ok := statuses[target.Service]
	if !ok {
		return State{Service: target.Service, Stack: target.Stack, Ready: false, Reason: "missing"}
	}

	state := strings.ToLower(strings.TrimSpace(st.State))

	// One-shot containers (migrators/init jobs) are Ready once they exit
	// cleanly, regardless of whether they declare a healthcheck. We only
	// look at State/ExitCode here -- restart policies are deliberately not
	// parsed (see altctl reliability plan).
	if strings.Contains(state, "exit") {
		if st.ExitCode == 0 {
			return State{Service: target.Service, Stack: target.Stack, Ready: true, Reason: "exited(0)"}
		}
		return State{Service: target.Service, Stack: target.Stack, Ready: false, Reason: fmt.Sprintf("exited(%d)", st.ExitCode)}
	}

	if !strings.HasPrefix(state, "running") && !strings.HasPrefix(state, "up") {
		reason := state
		if reason == "" {
			reason = "unknown"
		}
		return State{Service: target.Service, Stack: target.Stack, Ready: false, Reason: reason}
	}

	health := strings.ToLower(strings.TrimSpace(st.Health))
	switch health {
	case "":
		// No healthcheck declared: running is enough.
		return State{Service: target.Service, Stack: target.Stack, Ready: true, Reason: "running"}
	case "healthy":
		return State{Service: target.Service, Stack: target.Stack, Ready: true, Reason: "healthy"}
	default:
		return State{Service: target.Service, Stack: target.Stack, Ready: false, Reason: "health: " + health}
	}
}
