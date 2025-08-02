package resilience

import (
	"context"
	"errors"
	"sync"
	"time"
)

// CircuitBreakerState represents the state of the circuit breaker
type CircuitBreakerState string

const (
	StateClosed   CircuitBreakerState = "closed"
	StateOpen     CircuitBreakerState = "open"
	StateHalfOpen CircuitBreakerState = "half_open"
)

// CircuitBreakerConfig holds the configuration for the circuit breaker
type CircuitBreakerConfig struct {
	FailureThreshold      int           `json:"failure_threshold"`
	ResetTimeout          time.Duration `json:"reset_timeout"`
	MaxConcurrentRequests int           `json:"max_concurrent_requests"`
}

// DefaultCircuitBreakerConfig returns default configuration
func DefaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		FailureThreshold:      5,
		ResetTimeout:          60 * time.Second,
		MaxConcurrentRequests: 10,
	}
}

// SimpleCircuitBreaker implements a basic circuit breaker pattern
type SimpleCircuitBreaker struct {
	config           *CircuitBreakerConfig
	state            CircuitBreakerState
	failureCount     int
	lastFailureTime  time.Time
	concurrentReqs   int
	mutex            sync.RWMutex
}

// NewSimpleCircuitBreaker creates a new circuit breaker instance
func NewSimpleCircuitBreaker(config *CircuitBreakerConfig) *SimpleCircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig()
	}

	return &SimpleCircuitBreaker{
		config: config,
		state:  StateClosed,
	}
}

// Execute runs the operation with circuit breaker protection
func (cb *SimpleCircuitBreaker) Execute(ctx context.Context, operation func() error) error {
	// Check and potentially transition from Open to Half-Open
	cb.checkAndTransitionToHalfOpen()

	if !cb.CanExecute() {
		return errors.New("circuit breaker is open")
	}

	// Check concurrent request limit
	cb.mutex.Lock()
	if cb.concurrentReqs >= cb.config.MaxConcurrentRequests {
		cb.mutex.Unlock()
		return errors.New("too many concurrent requests")
	}
	cb.concurrentReqs++
	cb.mutex.Unlock()

	defer func() {
		cb.mutex.Lock()
		cb.concurrentReqs--
		cb.mutex.Unlock()
	}()

	// Execute operation
	err := operation()

	if err != nil {
		cb.onFailure()
		return err
	}

	cb.onSuccess()
	return nil
}

// checkAndTransitionToHalfOpen transitions from Open to Half-Open if timeout has passed
func (cb *SimpleCircuitBreaker) checkAndTransitionToHalfOpen() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if cb.state == StateOpen && time.Since(cb.lastFailureTime) >= cb.config.ResetTimeout {
		cb.state = StateHalfOpen
	}
}

// CanExecute checks if the circuit breaker allows execution
func (cb *SimpleCircuitBreaker) CanExecute() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		// Check if reset timeout has passed
		if time.Since(cb.lastFailureTime) >= cb.config.ResetTimeout {
			// Transition to Half-Open will happen on next Execute call
			return true
		}
		return false
	case StateHalfOpen:
		return cb.concurrentReqs == 0 // Allow one request at a time in half-open
	default:
		return false
	}
}

// GetState returns the current state
func (cb *SimpleCircuitBreaker) GetState() CircuitBreakerState {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	// Auto-transition from Open to Half-Open if timeout has passed
	if cb.state == StateOpen && time.Since(cb.lastFailureTime) >= cb.config.ResetTimeout {
		// Don't change state here, just report the potential state
		return cb.state
	}

	return cb.state
}

// GetFailureCount returns the current failure count
func (cb *SimpleCircuitBreaker) GetFailureCount() int {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.failureCount
}

// Reset resets the circuit breaker to initial state
func (cb *SimpleCircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.state = StateClosed
	cb.failureCount = 0
	cb.lastFailureTime = time.Time{}
}

// onSuccess handles successful operations
func (cb *SimpleCircuitBreaker) onSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	switch cb.state {
	case StateClosed:
		cb.failureCount = 0
	case StateHalfOpen:
		// Successful execution in Half-Open transitions back to Closed
		cb.state = StateClosed
		cb.failureCount = 0
	}
}

// onFailure handles failed operations
func (cb *SimpleCircuitBreaker) onFailure() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		if cb.failureCount >= cb.config.FailureThreshold {
			cb.state = StateOpen
		}
	case StateHalfOpen:
		// Failure in Half-Open immediately transitions back to Open
		cb.state = StateOpen
	}
}