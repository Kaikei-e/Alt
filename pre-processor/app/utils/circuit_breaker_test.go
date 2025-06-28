// ABOUTME: This file contains comprehensive tests for circuit breaker pattern implementation
// ABOUTME: Tests circuit breaker states and failure protection for external APIs
package utils

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreaker_ClosedState(t *testing.T) {
	tests := map[string]struct {
		threshold   int
		timeout     time.Duration
		expectState CircuitBreakerState
	}{
		"should start in closed state": {
			threshold:   3,
			timeout:     time.Second,
			expectState: StateClosed,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// RED PHASE: Test fails because CircuitBreaker doesn't exist yet
			cb := NewCircuitBreaker(tc.threshold, tc.timeout)

			assert.Equal(t, tc.expectState, cb.State())
			assert.Equal(t, 0, cb.Failures())
		})
	}
}

func TestCircuitBreaker_OpenState(t *testing.T) {
	tests := map[string]struct {
		threshold     int
		timeout       time.Duration
		failureCount  int
		expectState   CircuitBreakerState
		expectFailure bool
	}{
		"should open after threshold failures": {
			threshold:     3,
			timeout:       time.Second,
			failureCount:  3,
			expectState:   StateOpen,
			expectFailure: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// RED PHASE: Test fails because CircuitBreaker doesn't exist yet
			cb := NewCircuitBreaker(tc.threshold, tc.timeout)

			// Simulate failures to trigger open state
			for i := 0; i < tc.failureCount; i++ {
				err := cb.Call(func() error {
					return errors.New("test failure")
				})
				if i < tc.threshold-1 {
					require.Error(t, err)
				}
			}

			assert.Equal(t, tc.expectState, cb.State())

			// Next call should fail immediately due to open circuit
			err := cb.Call(func() error {
				return nil // This shouldn't execute
			})

			if tc.expectFailure {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "circuit breaker open")
			}
		})
	}
}

func TestCircuitBreaker_HalfOpenState(t *testing.T) {
	tests := map[string]struct {
		threshold   int
		timeout     time.Duration
		expectState CircuitBreakerState
	}{
		"should transition to half-open after timeout": {
			threshold:   2,
			timeout:     50 * time.Millisecond,
			expectState: StateHalfOpen,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// RED PHASE: Test fails because CircuitBreaker doesn't exist yet
			cb := NewCircuitBreaker(tc.threshold, tc.timeout)

			// Trigger open state
			for i := 0; i < tc.threshold; i++ {
				cb.Call(func() error {
					return errors.New("test failure")
				})
			}

			assert.Equal(t, StateOpen, cb.State())

			// Wait for timeout
			time.Sleep(tc.timeout + 10*time.Millisecond)

			// Next call should put it in half-open state
			err := cb.Call(func() error {
				return nil // Success
			})

			require.NoError(t, err)
			assert.Equal(t, StateClosed, cb.State()) // Should close on success
		})
	}
}

func TestCircuitBreaker_SuccessfulRecovery(t *testing.T) {
	t.Run("should close after successful call in half-open state", func(t *testing.T) {
		// RED PHASE: Test fails because CircuitBreaker doesn't exist yet
		cb := NewCircuitBreaker(2, 50*time.Millisecond)

		// Open circuit
		cb.Call(func() error { return errors.New("fail 1") })
		cb.Call(func() error { return errors.New("fail 2") })

		assert.Equal(t, StateOpen, cb.State())

		// Wait for timeout
		time.Sleep(60 * time.Millisecond)

		// Successful call should close circuit
		err := cb.Call(func() error {
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, StateClosed, cb.State())
		assert.Equal(t, 0, cb.Failures())
	})
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	t.Run("should handle concurrent access safely", func(t *testing.T) {
		// RED PHASE: Test fails because CircuitBreaker doesn't exist yet
		cb := NewCircuitBreaker(5, 100*time.Millisecond)

		var wg sync.WaitGroup
		errorCount := 0
		successCount := 0
		var mu sync.Mutex

		// Simulate concurrent calls
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				err := cb.Call(func() error {
					if id%2 == 0 {
						return errors.New("test failure")
					}
					return nil
				})

				mu.Lock()
				if err != nil {
					errorCount++
				} else {
					successCount++
				}
				mu.Unlock()
			}(i)
		}

		wg.Wait()

		// Should have both successes and failures
		assert.Greater(t, errorCount, 0)
		assert.Greater(t, successCount, 0)
	})
}

func TestCircuitBreaker_Metrics(t *testing.T) {
	t.Run("should track metrics correctly", func(t *testing.T) {
		// RED PHASE: Test fails because CircuitBreaker doesn't exist yet
		cb := NewCircuitBreaker(3, 100*time.Millisecond)

		// Initial metrics
		metrics := cb.Metrics()
		assert.Equal(t, int64(0), metrics.TotalCalls)
		assert.Equal(t, int64(0), metrics.TotalFailures)
		assert.Equal(t, int64(0), metrics.TotalSuccesses)

		// Make some calls
		cb.Call(func() error { return nil })                   // Success
		cb.Call(func() error { return errors.New("failure") }) // Failure
		cb.Call(func() error { return nil })                   // Success

		metrics = cb.Metrics()
		assert.Equal(t, int64(3), metrics.TotalCalls)
		assert.Equal(t, int64(1), metrics.TotalFailures)
		assert.Equal(t, int64(2), metrics.TotalSuccesses)
	})
}

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	tests := []struct {
		name       string
		threshold  int
		timeout    time.Duration
		operations []struct {
			fn            func() error
			expectedState CircuitBreakerState
			expectError   bool
		}
		sleepBetween time.Duration
	}{
		{
			name:      "full state cycle",
			threshold: 2,
			timeout:   50 * time.Millisecond,
			operations: []struct {
				fn            func() error
				expectedState CircuitBreakerState
				expectError   bool
			}{
				{fn: func() error { return nil }, expectedState: StateClosed, expectError: false},
				{fn: func() error { return errors.New("fail") }, expectedState: StateClosed, expectError: true},
				{fn: func() error { return errors.New("fail") }, expectedState: StateOpen, expectError: true},
				{fn: func() error { return nil }, expectedState: StateOpen, expectError: true}, // Should fail - circuit open
			},
			sleepBetween: 60 * time.Millisecond,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// RED PHASE: Test fails because CircuitBreaker doesn't exist yet
			cb := NewCircuitBreaker(tc.threshold, tc.timeout)

			for i, op := range tc.operations {
				// Don't sleep before the last operation for this test case
				// The test expects the circuit to still be open

				err := cb.Call(op.fn)

				if op.expectError {
					assert.Error(t, err, "operation %d should have failed", i)
				} else {
					assert.NoError(t, err, "operation %d should have succeeded", i)
				}

				assert.Equal(t, op.expectedState, cb.State(), "operation %d state mismatch", i)
			}
		})
	}
}

func BenchmarkCircuitBreaker_ClosedState(b *testing.B) {
	cb := NewCircuitBreaker(5, 100*time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Call(func() error {
			return nil
		})
	}
}

func BenchmarkCircuitBreaker_OpenState(b *testing.B) {
	cb := NewCircuitBreaker(1, 100*time.Millisecond)

	// Open the circuit
	cb.Call(func() error {
		return errors.New("failure")
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Call(func() error {
			return nil
		})
	}
}
