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
	State           CircuitState
	TotalSuccesses  int64
	TotalFailures   int64
	ConsecFailures  int
	ConsecSuccesses int
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	mu sync.RWMutex

	config CircuitBreakerConfig

	state           CircuitState
	consecFailures  int
	consecSuccesses int
	lastFailure     time.Time

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

// Allow checks if a request should be allowed through.
// Returns true if the request is allowed, false if it should be rejected.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true

	case StateOpen:
		// Check if we should transition to half-open
		if time.Since(cb.lastFailure) > cb.config.OpenTimeout {
			cb.state = StateHalfOpen
			cb.consecSuccesses = 0
			return true
		}
		return false

	case StateHalfOpen:
		// Allow requests in half-open state for testing
		return true

	default:
		return false
	}
}

// RecordSuccess records a successful operation.
func (cb *CircuitBreaker) RecordSuccess() {
	atomic.AddInt64(&cb.totalSuccesses, 1)

	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecFailures = 0
	cb.consecSuccesses++

	if cb.state == StateHalfOpen {
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
	}
}

// Stats returns the current statistics.
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		State:           cb.state,
		TotalSuccesses:  atomic.LoadInt64(&cb.totalSuccesses),
		TotalFailures:   atomic.LoadInt64(&cb.totalFailures),
		ConsecFailures:  cb.consecFailures,
		ConsecSuccesses: cb.consecSuccesses,
	}
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.consecFailures = 0
	cb.consecSuccesses = 0
}

// Execute runs the given function if the circuit breaker allows it.
// It automatically records success or failure based on the returned error.
func Execute[T any](cb *CircuitBreaker, fn func() (T, error)) (T, error) {
	var zero T

	if !cb.Allow() {
		return zero, ErrCircuitOpen
	}

	result, err := fn()
	if err != nil {
		cb.RecordFailure()
		return result, err
	}

	cb.RecordSuccess()
	return result, nil
}
