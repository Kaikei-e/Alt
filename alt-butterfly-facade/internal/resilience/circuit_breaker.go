// Package resilience provides resilience patterns for the BFF service.
package resilience

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrCircuitOpen is returned when the circuit breaker is open and not allowing requests.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// StateClosed means the circuit is operating normally.
	StateClosed CircuitState = iota
	// StateOpen means the circuit has tripped and is rejecting requests.
	StateOpen
	// StateHalfOpen means the circuit is testing if the backend has recovered.
	StateHalfOpen
)

// String returns the string representation of the circuit state.
func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// StateTransition is one circuit state change, described well enough for an
// operator to act on it without reading any other service's logs: which
// dependency class degraded, which endpoint's outcome moved it, and how that
// compares to the configured budget.
//
// It is built at the point the state field is written, under the breaker's
// lock, so it is produced exactly once per transition and never inferred by a
// reader racing the writer. It is delivered afterwards, with the lock
// released, so a slow observer cannot put itself on every request's path.
type StateTransition struct {
	// Seq is a per-breaker counter assigned under the lock. Observers run
	// unlocked and so may be called out of order under concurrency; Seq is
	// how a reader (or a test) orders reports and spots a dropped one.
	Seq              uint64
	Class            DependencyClass
	Endpoint         string
	From             CircuitState
	To               CircuitState
	ConsecFailures   int
	ConsecSuccesses  int
	FailureThreshold int
	SuccessThreshold int
	OpenTimeout      time.Duration
	// RejectedSinceLastReport is how many requests this breaker refused since
	// the previous transition was reported. Rejections are counted rather than
	// reported individually: an open breaker can refuse thousands of requests
	// a second, and one report each turns an outage into a log flood. The
	// tally rides the next transition instead, which bounds reporting to the
	// state changes themselves — during a sustained outage that is the
	// OPEN -> HALF_OPEN probe (at most once per OpenTimeout) and the failed
	// trial behind it, so the summary is also effectively periodic.
	RejectedSinceLastReport int64
}

// TransitionObserver receives every state change. It is called on the
// goroutine whose call caused the transition, with the breaker's lock
// released, so it may read the breaker back and may block without stalling
// other requests' admission decisions.
type TransitionObserver func(StateTransition)

// CircuitBreakerConfig holds the configuration for a circuit breaker.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive failures before opening the circuit.
	FailureThreshold int
	// SuccessThreshold is the number of consecutive successes in half-open state before closing.
	SuccessThreshold int
	// OpenTimeout is how long the circuit stays open before transitioning to half-open.
	OpenTimeout time.Duration
	// Class labels this breaker's dependency class in transition reports.
	Class DependencyClass
	// OnTransition, when set, receives every state change. Left nil the
	// breaker is silent — which is what the incident this reporting exists for
	// looked like, so production wiring must always set it.
	OnTransition TransitionObserver
}

// DefaultCircuitBreakerConfig returns a configuration with sensible defaults.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		OpenTimeout:      30 * time.Second,
	}
}

// CircuitBreakerStats holds statistics about the circuit breaker.
type CircuitBreakerStats struct {
	State            CircuitState
	TotalSuccesses   int64
	TotalFailures    int64
	TotalRejections  int64
	ConsecFailures   int
	ConsecSuccesses  int
	HalfOpenInFlight int
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	mu sync.RWMutex

	config CircuitBreakerConfig

	state           CircuitState
	consecFailures  int
	consecSuccesses int
	lastFailure     time.Time

	// halfOpenInFlight tracks the number of trial calls currently permitted
	// (allowed but not yet resolved via RecordSuccess/RecordFailure/Release)
	// while the circuit is half-open. Only one trial call is permitted at a
	// time so concurrent requests don't all flood a not-yet-recovered backend.
	halfOpenInFlight int
	// halfOpenSince is when the outstanding trial permit was handed out, so a
	// permit whose holder never resolved it can be reclaimed instead of
	// rejecting the whole dependency class until the process restarts.
	halfOpenSince time.Time
	// halfOpenSeq increments on every trial permit handed out, so a Release
	// arriving after the permit was already resolved or superseded is a no-op.
	halfOpenSeq uint64

	// transitionSeq numbers state changes so an observer running unlocked can
	// still tell the order they happened in, and notice a missing one.
	transitionSeq uint64
	// rejectedSinceReport is the rejection tally carried by the next
	// transition report; totalRejections is the cumulative count exposed by
	// Stats for /v1/bff/stats. Both are written under mu.
	rejectedSinceReport int64
	totalRejections     int64

	// Stats
	totalSuccesses int64
	totalFailures  int64
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	state := cb.state
	lastFailure := cb.lastFailure
	cb.mu.RUnlock()

	// Check if we should transition from open to half-open
	if state == StateOpen && time.Since(lastFailure) > cb.config.OpenTimeout {
		return StateHalfOpen
	}

	return state
}

// Permit is the admission granted by Acquire. While the circuit is half-open
// it carries the single trial slot; Release hands that slot back so callers
// that finish without recording an outcome (an early return, a cache hit, a
// panic unwinding) don't strand it. Release is a no-op for permits granted
// while the circuit was closed and a no-op once RecordSuccess/RecordFailure
// resolved the trial, so `defer permit.Release()` is always safe to place
// immediately after acquisition.
type Permit struct {
	cb  *CircuitBreaker
	seq uint64
}

// Release hands an unresolved half-open trial slot back to the circuit breaker.
func (p Permit) Release() {
	if p.cb == nil {
		return
	}

	p.cb.mu.Lock()
	defer p.cb.mu.Unlock()

	// A newer trial (sequence mismatch) or an already-recorded outcome
	// (nothing in flight) means this permit is no longer outstanding.
	if p.cb.halfOpenSeq != p.seq || p.cb.halfOpenInFlight == 0 {
		return
	}
	p.cb.halfOpenInFlight--
}

// Acquire checks if a request should be allowed through and returns the permit
// covering it. Callers must `defer permit.Release()` immediately after a
// successful acquisition so the half-open trial slot is handed back on every
// exit path — including the ones that never reach the dependency and so never
// record an outcome.
func (cb *CircuitBreaker) Acquire(endpoint string) (Permit, bool) {
	cb.mu.Lock()
	permit, allowed, transition := cb.acquireLocked(endpoint)
	cb.mu.Unlock()

	cb.report(transition)
	return permit, allowed
}

// acquireLocked makes the admission decision and captures the state change it
// caused, if any. Callers must hold cb.mu and must report the transition only
// after releasing it.
func (cb *CircuitBreaker) acquireLocked(endpoint string) (Permit, bool, *StateTransition) {
	now := time.Now()

	switch cb.state {
	case StateClosed:
		return Permit{}, true, nil

	case StateOpen:
		// Check if we should transition to half-open
		if now.Sub(cb.lastFailure) > cb.config.OpenTimeout {
			cb.consecSuccesses = 0
			cb.halfOpenInFlight = 0
			transition := cb.transitionLocked(StateHalfOpen, endpoint)
			return cb.grantTrialLocked(now), true, transition
		}
		return cb.rejectLocked()

	case StateHalfOpen:
		// Only one trial call is permitted at a time while the circuit
		// verifies the backend has recovered; concurrent callers are rejected
		// until that trial resolves via RecordSuccess/RecordFailure/Release.
		// A trial still outstanding after OpenTimeout is treated as abandoned
		// and reclaimed — a permit nobody hands back must not reject the whole
		// dependency class until the process restarts.
		if cb.halfOpenInFlight >= 1 && now.Sub(cb.halfOpenSince) <= cb.config.OpenTimeout {
			return cb.rejectLocked()
		}
		cb.halfOpenInFlight = 0
		return cb.grantTrialLocked(now), true, nil

	default:
		return cb.rejectLocked()
	}
}

// rejectLocked refuses a request and tallies it. Callers must hold cb.mu.
func (cb *CircuitBreaker) rejectLocked() (Permit, bool, *StateTransition) {
	cb.rejectedSinceReport++
	cb.totalRejections++
	return Permit{}, false, nil
}

// transitionLocked moves the circuit to `to` and captures the report for the
// change. Every write to cb.state goes through here, which is what makes a
// transition reported exactly once: it is recorded by the goroutine that
// performed it, at the instant it performed it, instead of being deduced
// afterwards by a reader that may have missed an intervening state.
// Callers must hold cb.mu and must deliver the report after releasing it.
func (cb *CircuitBreaker) transitionLocked(to CircuitState, endpoint string) *StateTransition {
	from := cb.state
	cb.state = to

	cb.transitionSeq++
	rejected := cb.rejectedSinceReport
	cb.rejectedSinceReport = 0

	if cb.config.OnTransition == nil {
		return nil
	}
	return &StateTransition{
		Seq:                     cb.transitionSeq,
		Class:                   cb.config.Class,
		Endpoint:                endpoint,
		From:                    from,
		To:                      to,
		ConsecFailures:          cb.consecFailures,
		ConsecSuccesses:         cb.consecSuccesses,
		FailureThreshold:        cb.config.FailureThreshold,
		SuccessThreshold:        cb.config.SuccessThreshold,
		OpenTimeout:             cb.config.OpenTimeout,
		RejectedSinceLastReport: rejected,
	}
}

// report hands a captured transition to the observer. cb.config is fixed at
// construction, so reading it without the lock is safe — and reporting here,
// after the caller released cb.mu, keeps a slow observer off the admission
// path of every other goroutine.
func (cb *CircuitBreaker) report(transition *StateTransition) {
	if transition == nil {
		return
	}
	cb.config.OnTransition(*transition)
}

// Allow checks if a request should be allowed through, discarding the permit.
// Returns true if the request is allowed, false if it should be rejected.
// A half-open trial taken this way is released only by RecordSuccess /
// RecordFailure or by Acquire's reclaim; callers with exit paths that record
// no outcome must use Acquire and release the permit themselves.
func (cb *CircuitBreaker) Allow(endpoint string) bool {
	_, allowed := cb.Acquire(endpoint)
	return allowed
}

// grantTrialLocked hands out the single half-open trial slot.
// Callers must hold cb.mu.
func (cb *CircuitBreaker) grantTrialLocked(now time.Time) Permit {
	cb.halfOpenInFlight++
	cb.halfOpenSince = now
	cb.halfOpenSeq++
	return Permit{cb: cb, seq: cb.halfOpenSeq}
}

// RecordSuccess records a successful operation against the endpoint that
// produced it; the endpoint is what a resulting transition is attributed to.
func (cb *CircuitBreaker) RecordSuccess(endpoint string) {
	atomic.AddInt64(&cb.totalSuccesses, 1)

	cb.mu.Lock()

	cb.consecFailures = 0
	cb.consecSuccesses++

	var transition *StateTransition
	if cb.state == StateHalfOpen {
		if cb.halfOpenInFlight > 0 {
			cb.halfOpenInFlight--
		}
		if cb.consecSuccesses >= cb.config.SuccessThreshold {
			transition = cb.transitionLocked(StateClosed, endpoint)
			cb.consecSuccesses = 0
		}
	}

	cb.mu.Unlock()
	cb.report(transition)
}

// RecordFailure records a failed operation against the endpoint that produced
// it; the endpoint is what a resulting transition is attributed to.
func (cb *CircuitBreaker) RecordFailure(endpoint string) {
	atomic.AddInt64(&cb.totalFailures, 1)

	cb.mu.Lock()

	cb.consecSuccesses = 0
	cb.consecFailures++
	cb.lastFailure = time.Now()

	var transition *StateTransition
	if cb.state == StateClosed {
		if cb.consecFailures >= cb.config.FailureThreshold {
			transition = cb.transitionLocked(StateOpen, endpoint)
		}
	} else if cb.state == StateHalfOpen {
		// Any failure in half-open state trips the circuit again
		transition = cb.transitionLocked(StateOpen, endpoint)
		cb.halfOpenInFlight = 0
	}

	cb.mu.Unlock()
	cb.report(transition)
}

// Stats returns the current statistics.
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		State:            cb.state,
		TotalSuccesses:   atomic.LoadInt64(&cb.totalSuccesses),
		TotalFailures:    atomic.LoadInt64(&cb.totalFailures),
		TotalRejections:  cb.totalRejections,
		ConsecFailures:   cb.consecFailures,
		ConsecSuccesses:  cb.consecSuccesses,
		HalfOpenInFlight: cb.halfOpenInFlight,
	}
}

// resetEndpoint attributes the transition Reset causes: it is an operator
// escape hatch rather than an outcome, so no request endpoint triggered it.
const resetEndpoint = "(reset)"

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()

	var transition *StateTransition
	if cb.state != StateClosed {
		transition = cb.transitionLocked(StateClosed, resetEndpoint)
	}
	cb.consecFailures = 0
	cb.consecSuccesses = 0
	cb.halfOpenInFlight = 0
	// Stale outstanding permits must not decrement a future trial slot.
	cb.halfOpenSeq++
	cb.halfOpenSince = time.Time{}

	cb.mu.Unlock()
	cb.report(transition)
}

// Execute runs the given function if the circuit breaker allows it.
// It automatically records success or failure based on the returned error.
func Execute[T any](cb *CircuitBreaker, endpoint string, fn func() (T, error)) (T, error) {
	var zero T

	permit, allowed := cb.Acquire(endpoint)
	if !allowed {
		return zero, ErrCircuitOpen
	}
	// Hands the half-open trial slot back if fn panics before an outcome is
	// recorded; a no-op once RecordSuccess/RecordFailure ran.
	defer permit.Release()

	result, err := fn()
	if err != nil {
		cb.RecordFailure(endpoint)
		return result, err
	}

	cb.RecordSuccess(endpoint)
	return result, nil
}
