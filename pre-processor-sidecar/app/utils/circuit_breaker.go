// TDD Phase 3 - REFACTOR: Circuit Breaker Pattern Implementation
package utils

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// CircuitBreakerState represents the current state of the circuit breaker
type CircuitBreakerState int

const (
	StateClosed CircuitBreakerState = iota // 正常状態：リクエストを通す
	StateOpen                              // 障害状態：リクエストを遮断
	StateHalfOpen                          // 回復テスト状態：限定的にリクエストを通す
)

func (s CircuitBreakerState) String() string {
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

// CircuitBreakerConfig holds configuration for the circuit breaker
type CircuitBreakerConfig struct {
	FailureThreshold int           // 失敗閾値：この回数失敗するとOPENになる
	SuccessThreshold int           // 成功閾値：HALF_OPENでこの回数成功するとCLOSEDになる
	Timeout          time.Duration // タイムアウト：OPENからHALF_OPENになるまでの時間
	MaxRequests      int           // 最大リクエスト数：HALF_OPENで同時に実行できるリクエスト数
}

// DefaultCircuitBreakerConfig returns a default circuit breaker configuration
func DefaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		FailureThreshold: 5,              // 5回連続失敗でOPEN
		SuccessThreshold: 3,              // HALF_OPENで3回成功すればCLOSED
		Timeout:          30 * time.Second, // 30秒でHALF_OPENに移行
		MaxRequests:      2,              // HALF_OPENで最大2つのリクエストを許可
	}
}

// CircuitBreakerStats holds statistics for monitoring
type CircuitBreakerStats struct {
	State                CircuitBreakerState
	FailureCount         int
	SuccessCount         int
	LastFailureTime      time.Time
	LastSuccessTime      time.Time
	TotalRequests        int64
	TotalSuccesses       int64
	TotalFailures        int64
	TotalRejections      int64
	StateTransitionCount int64
}

// CircuitBreaker implements the circuit breaker pattern for API resilience
type CircuitBreaker struct {
	config *CircuitBreakerConfig
	logger *slog.Logger

	mu               sync.RWMutex
	state            CircuitBreakerState
	failureCount     int
	successCount     int
	lastFailureTime  time.Time
	lastSuccessTime  time.Time
	nextRetry        time.Time
	halfOpenRequests int

	// Statistics
	totalRequests        int64
	totalSuccesses       int64
	totalFailures        int64
	totalRejections      int64
	stateTransitionCount int64
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config *CircuitBreakerConfig, logger *slog.Logger) *CircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &CircuitBreaker{
		config: config,
		logger: logger,
		state:  StateClosed,
	}
}

// ErrCircuitBreakerOpen is returned when the circuit breaker is open
var ErrCircuitBreakerOpen = errors.New("circuit breaker is open")

// Execute executes the given function if the circuit breaker allows it
func (cb *CircuitBreaker) Execute(ctx context.Context, operation func(ctx context.Context) error) error {
	// Check if request is allowed
	if !cb.allowRequest() {
		cb.mu.Lock()
		cb.totalRejections++
		cb.mu.Unlock()
		
		cb.logger.Debug("Circuit breaker rejected request",
			"state", cb.state.String(),
			"failure_count", cb.failureCount)
		return ErrCircuitBreakerOpen
	}

	cb.mu.Lock()
	cb.totalRequests++
	cb.mu.Unlock()

	// Execute the operation
	err := operation(ctx)

	if err != nil {
		cb.onFailure(err)
	} else {
		cb.onSuccess()
	}

	return err
}

// allowRequest checks if the circuit breaker should allow the request
func (cb *CircuitBreaker) allowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		// Check if timeout has passed to transition to half-open
		if time.Now().After(cb.nextRetry) {
			cb.logger.Info("Circuit breaker transitioning to half-open state")
			cb.setState(StateHalfOpen)
			cb.halfOpenRequests++
			return true
		}
		return false
	case StateHalfOpen:
		// Allow limited number of requests in half-open state
		if cb.halfOpenRequests < cb.config.MaxRequests {
			cb.halfOpenRequests++
			return true
		}
		return false
	default:
		return false
	}
}

// onSuccess handles successful operation completion
func (cb *CircuitBreaker) onSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalSuccesses++
	cb.lastSuccessTime = time.Now()

	switch cb.state {
	case StateClosed:
		// Reset failure count on success
		cb.failureCount = 0
	case StateHalfOpen:
		cb.successCount++
		cb.halfOpenRequests--
		
		// If we have enough successes, close the circuit
		if cb.successCount >= cb.config.SuccessThreshold {
			cb.logger.Info("Circuit breaker closing - sufficient successes",
				"success_count", cb.successCount,
				"threshold", cb.config.SuccessThreshold)
			cb.setState(StateClosed)
		}
	}
}

// onFailure handles failed operation completion
func (cb *CircuitBreaker) onFailure(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalFailures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		cb.failureCount++
		// Open circuit if failure threshold is reached
		if cb.failureCount >= cb.config.FailureThreshold {
			cb.logger.Warn("Circuit breaker opening due to failures",
				"failure_count", cb.failureCount,
				"threshold", cb.config.FailureThreshold,
				"error", err)
			cb.setState(StateOpen)
		}
	case StateHalfOpen:
		// Any failure in half-open state immediately opens the circuit
		cb.halfOpenRequests--
		cb.logger.Warn("Circuit breaker re-opening from half-open state",
			"error", err)
		cb.setState(StateOpen)
	}
}

// setState changes the circuit breaker state with logging
func (cb *CircuitBreaker) setState(newState CircuitBreakerState) {
	oldState := cb.state
	cb.state = newState
	cb.stateTransitionCount++

	switch newState {
	case StateClosed:
		cb.failureCount = 0
		cb.successCount = 0
		cb.halfOpenRequests = 0
	case StateOpen:
		cb.nextRetry = time.Now().Add(cb.config.Timeout)
		cb.successCount = 0
		cb.halfOpenRequests = 0
	case StateHalfOpen:
		cb.halfOpenRequests = 0
		cb.successCount = 0
	}

	cb.logger.Info("Circuit breaker state transition",
		"from", oldState.String(),
		"to", newState.String(),
		"next_retry", cb.nextRetry.Format(time.RFC3339))
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetStats returns current statistics for monitoring
func (cb *CircuitBreaker) GetStats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		State:                cb.state,
		FailureCount:         cb.failureCount,
		SuccessCount:         cb.successCount,
		LastFailureTime:      cb.lastFailureTime,
		LastSuccessTime:      cb.lastSuccessTime,
		TotalRequests:        cb.totalRequests,
		TotalSuccesses:       cb.totalSuccesses,
		TotalFailures:        cb.totalFailures,
		TotalRejections:      cb.totalRejections,
		StateTransitionCount: cb.stateTransitionCount,
	}
}

// Reset resets the circuit breaker to its initial state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.logger.Info("Resetting circuit breaker")
	cb.setState(StateClosed)
}