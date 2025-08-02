package resilience

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSimpleCircuitBreaker_InitialState(t *testing.T) {
	cb := NewSimpleCircuitBreaker(DefaultCircuitBreakerConfig())
	
	assert.Equal(t, StateClosed, cb.GetState(), "Initial state should be Closed")
	assert.Equal(t, 0, cb.GetFailureCount(), "Initial failure count should be 0")
	assert.True(t, cb.CanExecute(), "Should be able to execute initially")
}

func TestSimpleCircuitBreaker_SuccessfulExecution(t *testing.T) {
	cb := NewSimpleCircuitBreaker(DefaultCircuitBreakerConfig())
	ctx := context.Background()

	// Successful operation
	err := cb.Execute(ctx, func() error {
		return nil
	})

	assert.NoError(t, err, "Successful operation should not return error")
	assert.Equal(t, StateClosed, cb.GetState(), "State should remain Closed after success")
	assert.Equal(t, 0, cb.GetFailureCount(), "Failure count should remain 0 after success")
}

func TestSimpleCircuitBreaker_FailureHandling(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 3,
		ResetTimeout:     100 * time.Millisecond,
		MaxConcurrentRequests: 1,
	}
	cb := NewSimpleCircuitBreaker(config)
	ctx := context.Background()

	// First failure
	err := cb.Execute(ctx, func() error {
		return errors.New("test failure")
	})

	assert.Error(t, err, "Failed operation should return error")
	assert.Equal(t, StateClosed, cb.GetState(), "State should remain Closed after single failure")
	assert.Equal(t, 1, cb.GetFailureCount(), "Failure count should be 1")
}

func TestSimpleCircuitBreaker_OpenOnMultipleFailures(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 3,
		ResetTimeout:     100 * time.Millisecond,
		MaxConcurrentRequests: 1,
	}
	cb := NewSimpleCircuitBreaker(config)
	ctx := context.Background()

	// Generate failures to trigger Open state
	for i := 0; i < 3; i++ {
		err := cb.Execute(ctx, func() error {
			return errors.New("test failure")
		})
		assert.Error(t, err, "Failed operation should return error")
	}

	assert.Equal(t, StateOpen, cb.GetState(), "State should be Open after threshold failures")
	assert.False(t, cb.CanExecute(), "Should not be able to execute when Open")
}

func TestSimpleCircuitBreaker_RejectRequestsWhenOpen(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 1,
		ResetTimeout:     100 * time.Millisecond,
		MaxConcurrentRequests: 1,
	}
	cb := NewSimpleCircuitBreaker(config)
	ctx := context.Background()

	// Trigger Open state
	cb.Execute(ctx, func() error {
		return errors.New("test failure")
	})

	assert.Equal(t, StateOpen, cb.GetState(), "State should be Open")

	// Attempt execution when Open
	err := cb.Execute(ctx, func() error {
		return nil
	})

	assert.Error(t, err, "Should reject requests when Open")
	assert.Contains(t, err.Error(), "circuit breaker is open", "Error should indicate circuit breaker is open")
}

func TestSimpleCircuitBreaker_HalfOpenTransition(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 1,
		ResetTimeout:     50 * time.Millisecond,
		MaxConcurrentRequests: 1,
	}
	cb := NewSimpleCircuitBreaker(config)
	ctx := context.Background()

	// Trigger Open state
	cb.Execute(ctx, func() error {
		return errors.New("test failure")
	})

	assert.Equal(t, StateOpen, cb.GetState(), "State should be Open")

	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)

	assert.True(t, cb.CanExecute(), "Should be able to execute after reset timeout")
	
	// Execute should trigger Half-Open state
	err := cb.Execute(ctx, func() error {
		return nil
	})

	assert.NoError(t, err, "Successful operation in Half-Open should work")
	assert.Equal(t, StateClosed, cb.GetState(), "State should transition to Closed after success in Half-Open")
}

func TestSimpleCircuitBreaker_HalfOpenFailure(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 1,
		ResetTimeout:     50 * time.Millisecond,
		MaxConcurrentRequests: 1,
	}
	cb := NewSimpleCircuitBreaker(config)
	ctx := context.Background()

	// Trigger Open state
	cb.Execute(ctx, func() error {
		return errors.New("test failure")
	})

	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)

	// Execute with failure in Half-Open state
	err := cb.Execute(ctx, func() error {
		return errors.New("test failure in half-open")
	})

	assert.Error(t, err, "Failed operation in Half-Open should return error")
	assert.Equal(t, StateOpen, cb.GetState(), "State should transition back to Open after failure in Half-Open")
}

func TestSimpleCircuitBreaker_ConcurrentRequestLimiting(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 5,
		ResetTimeout:     100 * time.Millisecond,
		MaxConcurrentRequests: 1,
	}
	cb := NewSimpleCircuitBreaker(config)
	ctx := context.Background()

	// Start a long-running operation
	done := make(chan bool)
	go func() {
		cb.Execute(ctx, func() error {
			time.Sleep(100 * time.Millisecond) // Long operation
			return nil
		})
		done <- true
	}()

	// Wait a bit to ensure first operation is running
	time.Sleep(10 * time.Millisecond)

	// Try to execute another operation (should be rejected due to concurrent limit)
	err := cb.Execute(ctx, func() error {
		return nil
	})

	assert.Error(t, err, "Should reject concurrent requests beyond limit")
	assert.Contains(t, err.Error(), "too many concurrent requests", "Error should indicate concurrent request limit")

	// Wait for first operation to complete
	<-done
}

func TestSimpleCircuitBreaker_Reset(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 1,
		ResetTimeout:     100 * time.Millisecond,
		MaxConcurrentRequests: 1,
	}
	cb := NewSimpleCircuitBreaker(config)
	ctx := context.Background()

	// Trigger Open state
	cb.Execute(ctx, func() error {
		return errors.New("test failure")
	})

	assert.Equal(t, StateOpen, cb.GetState(), "State should be Open")
	assert.Equal(t, 1, cb.GetFailureCount(), "Failure count should be 1")

	// Reset circuit breaker
	cb.Reset()

	assert.Equal(t, StateClosed, cb.GetState(), "State should be Closed after reset")
	assert.Equal(t, 0, cb.GetFailureCount(), "Failure count should be 0 after reset")
	assert.True(t, cb.CanExecute(), "Should be able to execute after reset")
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig()

	assert.Greater(t, config.FailureThreshold, 0, "Failure threshold should be positive")
	assert.Greater(t, config.ResetTimeout, time.Duration(0), "Reset timeout should be positive")
	assert.Greater(t, config.MaxConcurrentRequests, 0, "Max concurrent requests should be positive")
}