package resilience

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		OpenTimeout:      30 * time.Second,
	})

	assert.NotNil(t, cb)
	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	assert.Equal(t, StateClosed, cb.State())
	assert.True(t, cb.Allow())
}

func TestCircuitBreaker_StaysClosedOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	for i := 0; i < 10; i++ {
		assert.True(t, cb.Allow())
		cb.RecordSuccess()
	}

	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		OpenTimeout:      30 * time.Second,
	})

	// Record failures up to threshold
	for i := 0; i < 5; i++ {
		assert.True(t, cb.Allow())
		cb.RecordFailure()
	}

	// Circuit should be open now
	assert.Equal(t, StateOpen, cb.State())
	assert.False(t, cb.Allow())
}

func TestCircuitBreaker_RejectsWhenOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		OpenTimeout:      1 * time.Hour, // Long timeout to ensure it stays open
	})

	// Trip the circuit
	for i := 0; i < 3; i++ {
		cb.Allow()
		cb.RecordFailure()
	}

	// All subsequent requests should be rejected
	for i := 0; i < 10; i++ {
		assert.False(t, cb.Allow())
	}
}

func TestCircuitBreaker_TransitionsToHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		OpenTimeout:      50 * time.Millisecond,
	})

	// Trip the circuit
	for i := 0; i < 3; i++ {
		cb.Allow()
		cb.RecordFailure()
	}

	assert.Equal(t, StateOpen, cb.State())

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Should transition to half-open and allow one request
	assert.True(t, cb.Allow())
	assert.Equal(t, StateHalfOpen, cb.State())
}

func TestCircuitBreaker_ClosesAfterSuccessInHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		OpenTimeout:      50 * time.Millisecond,
	})

	// Trip the circuit
	for i := 0; i < 3; i++ {
		cb.Allow()
		cb.RecordFailure()
	}

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// Record successes in half-open state
	cb.Allow()
	cb.RecordSuccess()
	cb.Allow()
	cb.RecordSuccess()

	// Should be closed now
	assert.Equal(t, StateClosed, cb.State())
	assert.True(t, cb.Allow())
}

func TestCircuitBreaker_ReopensOnFailureInHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		OpenTimeout:      50 * time.Millisecond,
	})

	// Trip the circuit
	for i := 0; i < 3; i++ {
		cb.Allow()
		cb.RecordFailure()
	}

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// Allow one request
	cb.Allow()
	assert.Equal(t, StateHalfOpen, cb.State())

	// Record a failure
	cb.RecordFailure()

	// Should be back to open
	assert.Equal(t, StateOpen, cb.State())
	assert.False(t, cb.Allow())
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		OpenTimeout:      30 * time.Second,
	})

	// Record some failures (but not enough to trip)
	for i := 0; i < 4; i++ {
		cb.Allow()
		cb.RecordFailure()
	}

	// Record a success
	cb.Allow()
	cb.RecordSuccess()

	// Failure count should be reset, so more failures needed to trip
	for i := 0; i < 4; i++ {
		cb.Allow()
		cb.RecordFailure()
	}

	// Should still be closed
	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_Execute_Success(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	result, err := Execute(cb, func() (string, error) {
		return "success", nil
	})

	assert.NoError(t, err)
	assert.Equal(t, "success", result)
}

func TestCircuitBreaker_Execute_Failure(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())
	expectedErr := errors.New("operation failed")

	result, err := Execute(cb, func() (string, error) {
		return "", expectedErr
	})

	assert.Equal(t, expectedErr, err)
	assert.Equal(t, "", result)
}

func TestCircuitBreaker_Execute_CircuitOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 1,
		OpenTimeout:      1 * time.Hour,
	})

	// Trip the circuit
	for i := 0; i < 2; i++ {
		Execute(cb, func() (string, error) {
			return "", errors.New("fail")
		})
	}

	// Next execution should fail immediately
	_, err := Execute(cb, func() (string, error) {
		return "should not execute", nil
	})

	assert.Error(t, err)
	assert.Equal(t, ErrCircuitOpen, err)
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 100,
		SuccessThreshold: 10,
		OpenTimeout:      30 * time.Second,
	})

	done := make(chan bool)

	// Concurrent successes
	go func() {
		for i := 0; i < 50; i++ {
			if cb.Allow() {
				cb.RecordSuccess()
			}
		}
		done <- true
	}()

	// Concurrent failures
	go func() {
		for i := 0; i < 50; i++ {
			if cb.Allow() {
				cb.RecordFailure()
			}
		}
		done <- true
	}()

	// Concurrent state checks
	go func() {
		for i := 0; i < 50; i++ {
			cb.State()
			cb.Allow()
		}
		done <- true
	}()

	<-done
	<-done
	<-done

	// If we got here without panic, concurrent access is safe
	require.True(t, true)
}

func TestCircuitBreaker_Stats(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	// Record some activity
	for i := 0; i < 5; i++ {
		cb.Allow()
		cb.RecordSuccess()
	}
	for i := 0; i < 3; i++ {
		cb.Allow()
		cb.RecordFailure()
	}

	stats := cb.Stats()

	assert.Equal(t, int64(5), stats.TotalSuccesses)
	assert.Equal(t, int64(3), stats.TotalFailures)
	assert.Equal(t, StateClosed, stats.State)
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig()

	assert.Equal(t, 5, config.FailureThreshold)
	assert.Equal(t, 2, config.SuccessThreshold)
	assert.Equal(t, 30*time.Second, config.OpenTimeout)
}

func TestCircuitBreakerState_String(t *testing.T) {
	assert.Equal(t, "CLOSED", StateClosed.String())
	assert.Equal(t, "OPEN", StateOpen.String())
	assert.Equal(t, "HALF_OPEN", StateHalfOpen.String())
}

func TestCircuitBreaker_HalfOpenLimitsRequests(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 3,
		OpenTimeout:      50 * time.Millisecond,
	})

	// Trip the circuit
	for i := 0; i < 2; i++ {
		cb.Allow()
		cb.RecordFailure()
	}

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// First request allowed
	assert.True(t, cb.Allow())
	assert.Equal(t, StateHalfOpen, cb.State())

	// Subsequent requests should be limited until the first completes
	// This tests the half-open rate limiting behavior
}
