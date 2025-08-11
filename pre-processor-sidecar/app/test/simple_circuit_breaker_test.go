// TDD Phase 3 - REFACTOR: Simple Circuit Breaker Test
package test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"pre-processor-sidecar/utils"
)

// TestCircuitBreakerIntegration_BasicFlow tests basic circuit breaker flow
func TestCircuitBreakerIntegration_BasicFlow(t *testing.T) {
	config := &utils.CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          1000 * time.Millisecond, // 1 second timeout for testing
		MaxRequests:      1,
	}
	
	cb := utils.NewCircuitBreaker(config, slog.Default())
	ctx := context.Background()

	// Test successful operation
	err := cb.Execute(ctx, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}

	// Test failure leading to circuit open
	for i := 0; i < 3; i++ {
		cb.Execute(ctx, func(ctx context.Context) error {
			return errors.New("simulated failure")
		})
	}

	// Circuit should be open now
	if cb.GetState() != utils.StateOpen {
		t.Errorf("Expected circuit to be OPEN, got %s", cb.GetState())
	}

	// Test immediate rejection (before timeout)
	err = cb.Execute(ctx, func(ctx context.Context) error {
		t.Error("This function should not be called when circuit is open")
		return nil
	})

	if err != utils.ErrCircuitBreakerOpen {
		t.Errorf("Expected ErrCircuitBreakerOpen, got %v", err)
	}

	t.Logf("Circuit breaker integration test completed successfully")
}

// TestCircuitBreakerIntegration_Statistics tests statistics collection
func TestCircuitBreakerIntegration_Statistics(t *testing.T) {
	config := utils.DefaultCircuitBreakerConfig()
	cb := utils.NewCircuitBreaker(config, slog.Default())
	ctx := context.Background()

	// Execute mixed operations
	successOps := 5
	failureOps := 3

	for i := 0; i < successOps; i++ {
		cb.Execute(ctx, func(ctx context.Context) error {
			return nil
		})
	}

	for i := 0; i < failureOps; i++ {
		cb.Execute(ctx, func(ctx context.Context) error {
			return errors.New("test failure")
		})
	}

	stats := cb.GetStats()

	if stats.TotalRequests != int64(successOps+failureOps) {
		t.Errorf("Expected %d total requests, got %d", successOps+failureOps, stats.TotalRequests)
	}
	if stats.TotalSuccesses != int64(successOps) {
		t.Errorf("Expected %d total successes, got %d", successOps, stats.TotalSuccesses)
	}
	if stats.TotalFailures != int64(failureOps) {
		t.Errorf("Expected %d total failures, got %d", failureOps, stats.TotalFailures)
	}

	t.Logf("Statistics test completed - Requests: %d, Successes: %d, Failures: %d", 
		stats.TotalRequests, stats.TotalSuccesses, stats.TotalFailures)
}