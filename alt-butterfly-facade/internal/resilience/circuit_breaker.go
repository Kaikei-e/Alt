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

// CircuitBreakerConfig holds the configuration for a circuit breaker.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive failures before opening the circuit.
	FailureThreshold int
	// SuccessThreshold is the number of consecutive successes in half-open state before closing.
	SuccessThreshold int
	// OpenTimeout is how long the circuit stays open before transitioning to half-open.
	OpenTimeout time.Duration
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
func (cb *CircuitBreaker) Acquire() (Permit, bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case StateClosed:
		return Permit{}, true

	case StateOpen:
		// Check if we should transition to half-open
		if now.Sub(cb.lastFailure) > cb.config.OpenTimeout {
			cb.state = StateHalfOpen
			cb.consecSuccesses = 0
			cb.halfOpenInFlight = 0
			return cb.grantTrialLocked(now), true
		}
		return Permit{}, false

	case StateHalfOpen:
		// Only one trial call is permitted at a time while the circuit
		// verifies the backend has recovered; concurrent callers are rejected
		// until that trial resolves via RecordSuccess/RecordFailure/Release.
		// A trial still outstanding after OpenTimeout is treated as abandoned
		// and reclaimed — a permit nobody hands back must not reject the whole
		// dependency class until the process restarts.
		if cb.halfOpenInFlight >= 1 && now.Sub(cb.halfOpenSince) <= cb.config.OpenTimeout {
			return Permit{}, false
		}
		cb.halfOpenInFlight = 0
		return cb.grantTrialLocked(now), true

	default:
		return Permit{}, false
	}
}

// Allow checks if a request should be allowed through, discarding the permit.
// Returns true if the request is allowed, false if it should be rejected.
// A half-open trial taken this way is released only by RecordSuccess /
// RecordFailure or by Acquire's reclaim; callers with exit paths that record
// no outcome must use Acquire and release the permit themselves.
func (cb *CircuitBreaker) Allow() bool {
	_, allowed := cb.Acquire()
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

// RecordSuccess records a successful operation.
func (cb *CircuitBreaker) RecordSuccess() {
	atomic.AddInt64(&cb.totalSuccesses, 1)

	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecFailures = 0
	cb.consecSuccesses++

	if cb.state == StateHalfOpen {
		if cb.halfOpenInFlight > 0 {
			cb.halfOpenInFlight--
		}
		if cb.consecSuccesses >= cb.config.SuccessThreshold {
			cb.state = StateClosed
			cb.consecSuccesses = 0
		}
	}
}

// RecordFailure records a failed operation.
func (cb *CircuitBreaker) RecordFailure() {
	atomic.AddInt64(&cb.totalFailures, 1)

	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecSuccesses = 0
	cb.consecFailures++
	cb.lastFailure = time.Now()

	if cb.state == StateClosed {
		if cb.consecFailures >= cb.config.FailureThreshold {
			cb.state = StateOpen
		}
	} else if cb.state == StateHalfOpen {
		// Any failure in half-open state trips the circuit again
		cb.state = StateOpen
		cb.halfOpenInFlight = 0
	}
}

// Stats returns the current statistics.
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		State:            cb.state,
		TotalSuccesses:   atomic.LoadInt64(&cb.totalSuccesses),
		TotalFailures:    atomic.LoadInt64(&cb.totalFailures),
		ConsecFailures:   cb.consecFailures,
		ConsecSuccesses:  cb.consecSuccesses,
		HalfOpenInFlight: cb.halfOpenInFlight,
	}
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.consecFailures = 0
	cb.consecSuccesses = 0
	cb.halfOpenInFlight = 0
	// Stale outstanding permits must not decrement a future trial slot.
	cb.halfOpenSeq++
	cb.halfOpenSince = time.Time{}
}

// Execute runs the given function if the circuit breaker allows it.
// It automatically records success or failure based on the returned error.
func Execute[T any](cb *CircuitBreaker, fn func() (T, error)) (T, error) {
	var zero T

	permit, allowed := cb.Acquire()
	if !allowed {
		return zero, ErrCircuitOpen
	}
	// Hands the half-open trial slot back if fn panics before an outcome is
	// recorded; a no-op once RecordSuccess/RecordFailure ran.
	defer permit.Release()

	result, err := fn()
	if err != nil {
		cb.RecordFailure()
		return result, err
	}

	cb.RecordSuccess()
	return result, nil
}
