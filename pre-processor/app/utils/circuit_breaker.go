// ABOUTME: This file implements circuit breaker pattern for external API protection
// ABOUTME: Prevents cascade failures by temporarily blocking calls to failing services
package utils

import (
	"errors"
	"sync"
	"time"
)

// CircuitBreakerState represents the current state of the circuit breaker
type CircuitBreakerState int

const (
	// StateClosed means the circuit is closed and requests are allowed
	StateClosed CircuitBreakerState = iota
	// StateOpen means the circuit is open and requests are blocked
	StateOpen
	// StateHalfOpen means the circuit is testing if the service has recovered
	StateHalfOpen
)

// CircuitBreakerMetrics holds metrics for the circuit breaker
type CircuitBreakerMetrics struct {
	TotalCalls     int64
	TotalFailures  int64
	TotalSuccesses int64
	State          CircuitBreakerState
	LastFailure    time.Time
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	failures       int64
	lastFailure    time.Time
	threshold      int
	timeout        time.Duration
	state          CircuitBreakerState
	totalCalls     int64
	totalFailures  int64
	totalSuccesses int64
	mu             sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker with specified threshold and timeout
func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold: threshold,
		timeout:   timeout,
		state:     StateClosed,
	}
}

// Call executes the provided function with circuit breaker protection
func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mu.Lock()
	cb.totalCalls++

	// Check if circuit should move to half-open
	if cb.state == StateOpen && time.Since(cb.lastFailure) >= cb.timeout {
		cb.state = StateHalfOpen
	}

	// Block requests if circuit is open
	if cb.state == StateOpen {
		cb.mu.Unlock()
		return errors.New("circuit breaker open")
	}

	cb.mu.Unlock()

	// Execute the function
	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failures++
		cb.totalFailures++
		cb.lastFailure = time.Now()

		// Check if we should open the circuit after this failure
		if cb.failures >= int64(cb.threshold) {
			cb.state = StateOpen
		}

		// If we're in half-open state and failed, go back to open
		if cb.state == StateHalfOpen {
			cb.state = StateOpen
		}

		return err
	}

	// Success - reset failures and close circuit if half-open
	cb.totalSuccesses++
	if cb.state == StateHalfOpen {
		cb.failures = 0
		cb.state = StateClosed
	} else if cb.state == StateClosed {
		cb.failures = 0
	}

	return nil
}

// State returns the current state of the circuit breaker
func (cb *CircuitBreaker) State() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Failures returns the current failure count
func (cb *CircuitBreaker) Failures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return int(cb.failures)
}

// Metrics returns comprehensive metrics for the circuit breaker
func (cb *CircuitBreaker) Metrics() CircuitBreakerMetrics {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerMetrics{
		TotalCalls:     cb.totalCalls,
		TotalFailures:  cb.totalFailures,
		TotalSuccesses: cb.totalSuccesses,
		State:          cb.state,
		LastFailure:    cb.lastFailure,
	}
}

// Reset resets the circuit breaker to closed state with zero failures
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0
	cb.state = StateClosed
	cb.lastFailure = time.Time{}
}
