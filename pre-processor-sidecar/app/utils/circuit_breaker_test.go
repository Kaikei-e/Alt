// TDD Phase 3 - REFACTOR: Circuit Breaker Pattern Tests
package utils

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

// TestCircuitBreaker_InitialState tests that circuit breaker starts in closed state
func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker(nil, slog.Default())

	if cb.GetState() != StateClosed {
		t.Errorf("Expected initial state to be CLOSED, got %s", cb.GetState())
	}

	stats := cb.GetStats()
	if stats.TotalRequests != 0 {
		t.Errorf("Expected initial total requests to be 0, got %d", stats.TotalRequests)
	}
}

// TestCircuitBreaker_SuccessfulRequests tests successful request handling
func TestCircuitBreaker_SuccessfulRequests(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          1 * time.Second,
		MaxRequests:      2,
	}

	cb := NewCircuitBreaker(config, slog.Default())
	ctx := context.Background()

	// Execute successful operations
	for i := 0; i < 5; i++ {
		err := cb.Execute(ctx, func(ctx context.Context) error {
			return nil // Success
		})

		if err != nil {
			t.Errorf("Unexpected error for successful operation %d: %v", i, err)
		}
	}

	// Circuit should remain closed
	if cb.GetState() != StateClosed {
		t.Errorf("Expected state to remain CLOSED after successes, got %s", cb.GetState())
	}

	stats := cb.GetStats()
	if stats.TotalRequests != 5 {
		t.Errorf("Expected 5 total requests, got %d", stats.TotalRequests)
	}
	if stats.TotalSuccesses != 5 {
		t.Errorf("Expected 5 total successes, got %d", stats.TotalSuccesses)
	}
}

// TestCircuitBreaker_FailureThreshold tests circuit opening on failures
func TestCircuitBreaker_FailureThreshold(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          1 * time.Second,
		MaxRequests:      2,
	}

	cb := NewCircuitBreaker(config, slog.Default())
	ctx := context.Background()
	testError := errors.New("test failure")

	// Execute operations that fail (but not enough to open circuit)
	for i := 0; i < 2; i++ {
		err := cb.Execute(ctx, func(ctx context.Context) error {
			return testError
		})

		if err != testError {
			t.Errorf("Expected test error for operation %d, got %v", i, err)
		}

		if cb.GetState() != StateClosed {
			t.Errorf("Expected state to be CLOSED after %d failures, got %s", i+1, cb.GetState())
		}
	}

	// This failure should open the circuit
	err := cb.Execute(ctx, func(ctx context.Context) error {
		return testError
	})

	if err != testError {
		t.Errorf("Expected test error for threshold failure, got %v", err)
	}

	if cb.GetState() != StateOpen {
		t.Errorf("Expected state to be OPEN after threshold failures, got %s", cb.GetState())
	}
}

// TestCircuitBreaker_OpenStateRejectsRequests tests that open circuit rejects requests
func TestCircuitBreaker_OpenStateRejectsRequests(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          1 * time.Second,
		MaxRequests:      2,
	}

	cb := NewCircuitBreaker(config, slog.Default())
	ctx := context.Background()
	testError := errors.New("test failure")

	// Force circuit to open by exceeding failure threshold
	for i := 0; i < config.FailureThreshold; i++ {
		cb.Execute(ctx, func(ctx context.Context) error {
			return testError
		})
	}

	// Verify circuit is open
	if cb.GetState() != StateOpen {
		t.Fatalf("Expected circuit to be OPEN, got %s", cb.GetState())
	}

	// Subsequent request should be rejected immediately
	err := cb.Execute(ctx, func(ctx context.Context) error {
		t.Error("Operation should not have been executed when circuit is open")
		return nil
	})

	if err != ErrCircuitBreakerOpen {
		t.Errorf("Expected ErrCircuitBreakerOpen, got %v", err)
	}

	stats := cb.GetStats()
	if stats.TotalRejections != 1 {
		t.Errorf("Expected 1 rejection, got %d", stats.TotalRejections)
	}
}

// TestCircuitBreaker_HalfOpenTransition tests transition to half-open state
func TestCircuitBreaker_HalfOpenTransition(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond, // Short timeout for testing
		MaxRequests:      1,
	}

	cb := NewCircuitBreaker(config, slog.Default())
	ctx := context.Background()
	testError := errors.New("test failure")

	// Force circuit to open
	for i := 0; i < config.FailureThreshold; i++ {
		cb.Execute(ctx, func(ctx context.Context) error {
			return testError
		})
	}

	// Wait for timeout to pass
	time.Sleep(config.Timeout + 10*time.Millisecond)

	// Next request should be allowed (transitioning to half-open)
	executed := false
	err := cb.Execute(ctx, func(ctx context.Context) error {
		executed = true
		return nil // Success
	})

	if err != nil {
		t.Errorf("Expected successful execution in half-open state, got %v", err)
	}
	if !executed {
		t.Error("Operation should have been executed in half-open state")
	}
}

// TestCircuitBreaker_HalfOpenRecovery tests recovery from half-open to closed
func TestCircuitBreaker_HalfOpenRecovery(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
		MaxRequests:      3,
	}

	cb := NewCircuitBreaker(config, slog.Default())
	ctx := context.Background()

	// Force circuit to open
	for i := 0; i < config.FailureThreshold; i++ {
		cb.Execute(ctx, func(ctx context.Context) error {
			return errors.New("test failure")
		})
	}

	// Wait for timeout
	time.Sleep(config.Timeout + 10*time.Millisecond)

	// Execute successful requests to meet success threshold
	for i := 0; i < config.SuccessThreshold; i++ {
		err := cb.Execute(ctx, func(ctx context.Context) error {
			return nil // Success
		})
		if err != nil {
			t.Errorf("Unexpected error during recovery %d: %v", i, err)
		}
	}

	// Circuit should now be closed
	if cb.GetState() != StateClosed {
		t.Errorf("Expected circuit to be CLOSED after recovery, got %s", cb.GetState())
	}
}

// TestCircuitBreaker_Statistics tests statistics collection
func TestCircuitBreaker_Statistics(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	cb := NewCircuitBreaker(config, slog.Default())
	ctx := context.Background()

	// Execute mix of successful and failed operations
	successCount := 3
	failureCount := 2

	for i := 0; i < successCount; i++ {
		cb.Execute(ctx, func(ctx context.Context) error {
			return nil
		})
	}

	for i := 0; i < failureCount; i++ {
		cb.Execute(ctx, func(ctx context.Context) error {
			return errors.New("test failure")
		})
	}

	stats := cb.GetStats()

	if stats.TotalRequests != int64(successCount+failureCount) {
		t.Errorf("Expected %d total requests, got %d", successCount+failureCount, stats.TotalRequests)
	}
	if stats.TotalSuccesses != int64(successCount) {
		t.Errorf("Expected %d total successes, got %d", successCount, stats.TotalSuccesses)
	}
	if stats.TotalFailures != int64(failureCount) {
		t.Errorf("Expected %d total failures, got %d", failureCount, stats.TotalFailures)
	}
	if stats.State != StateClosed {
		t.Errorf("Expected state CLOSED, got %s", stats.State)
	}
}

// TestCircuitBreaker_Reset tests circuit breaker reset functionality
func TestCircuitBreaker_Reset(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          1 * time.Second,
		MaxRequests:      2,
	}

	cb := NewCircuitBreaker(config, slog.Default())
	ctx := context.Background()

	// Force circuit to open
	for i := 0; i < config.FailureThreshold; i++ {
		cb.Execute(ctx, func(ctx context.Context) error {
			return errors.New("test failure")
		})
	}

	// Verify circuit is open
	if cb.GetState() != StateOpen {
		t.Fatalf("Expected circuit to be OPEN before reset, got %s", cb.GetState())
	}

	// Reset the circuit breaker
	cb.Reset()

	// Verify circuit is closed
	if cb.GetState() != StateClosed {
		t.Errorf("Expected circuit to be CLOSED after reset, got %s", cb.GetState())
	}

	// Verify operation works after reset
	err := cb.Execute(ctx, func(ctx context.Context) error {
		return nil
	})

	if err != nil {
		t.Errorf("Expected successful operation after reset, got %v", err)
	}
}
