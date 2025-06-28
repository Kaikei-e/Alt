// ABOUTME: This file implements retry policy with exponential backoff and jitter
// ABOUTME: Provides configurable retry mechanisms for operation failures
package errors

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// RetryPolicy defines the retry behavior for failed operations
type RetryPolicy struct {
	MaxAttempts int           `json:"max_attempts"`
	BaseDelay   time.Duration `json:"base_delay"`
	MaxDelay    time.Duration `json:"max_delay"`
	Multiplier  float64       `json:"multiplier"`
	Jitter      bool          `json:"jitter"`
}

// RetryExecutor executes operations with retry logic
type RetryExecutor struct {
	policy *RetryPolicy
}

// NewRetryPolicy creates a new retry policy with default values
func NewRetryPolicy(maxAttempts int, baseDelay time.Duration) *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts: maxAttempts,
		BaseDelay:   baseDelay,
		MaxDelay:    30 * time.Second, // Default max delay
		Multiplier:  2.0,              // Default exponential multiplier
		Jitter:      true,             // Default jitter enabled
	}
}

// NewRetryExecutor creates a new retry executor with the given policy
func NewRetryExecutor(policy *RetryPolicy) *RetryExecutor {
	return &RetryExecutor{
		policy: policy,
	}
}

// CalculateDelay calculates the delay for a given retry attempt
func (rp *RetryPolicy) CalculateDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	// Calculate exponential backoff
	delay := rp.BaseDelay
	for i := 1; i < attempt; i++ {
		delay = time.Duration(float64(delay) * rp.Multiplier)
		if delay > rp.MaxDelay {
			delay = rp.MaxDelay
			break
		}
	}

	// Apply jitter if enabled
	if rp.Jitter {
		// Add random jitter between 50% and 100% of calculated delay
		jitterRange := float64(delay) * 0.5
		jitter := rand.Float64() * jitterRange
		delay = time.Duration(float64(delay)*0.5 + jitter)
	}

	return delay
}

// Execute executes the given operation with retry logic
func (re *RetryExecutor) Execute(ctx context.Context, operation func() error) error {
	var lastErr error

	for attempt := 1; attempt <= re.policy.MaxAttempts; attempt++ {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return fmt.Errorf("operation cancelled: %w", ctx.Err())
		default:
		}

		// Execute the operation
		err := operation()
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if error is retryable
		if !ShouldRetry(err) {
			return err // Don't retry non-retryable errors
		}

		// Don't wait after the last attempt
		if attempt == re.policy.MaxAttempts {
			break
		}

		// Calculate delay and wait
		delay := re.policy.CalculateDelay(attempt)

		select {
		case <-ctx.Done():
			return fmt.Errorf("operation cancelled during retry delay: %w", ctx.Err())
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return lastErr
}

// ShouldRetry determines if an error should be retried
func ShouldRetry(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's an OperationError and if it's marked as retryable
	if opErr, ok := err.(*OperationError); ok {
		return opErr.Retryable
	}

	// Don't retry regular errors by default
	return false
}

// WithMaxDelay creates a copy of the policy with a different max delay
func (rp *RetryPolicy) WithMaxDelay(maxDelay time.Duration) *RetryPolicy {
	newPolicy := *rp
	newPolicy.MaxDelay = maxDelay
	return &newPolicy
}

// WithJitter creates a copy of the policy with jitter setting
func (rp *RetryPolicy) WithJitter(jitter bool) *RetryPolicy {
	newPolicy := *rp
	newPolicy.Jitter = jitter
	return &newPolicy
}

// WithMultiplier creates a copy of the policy with a different multiplier
func (rp *RetryPolicy) WithMultiplier(multiplier float64) *RetryPolicy {
	newPolicy := *rp
	newPolicy.Multiplier = multiplier
	return &newPolicy
}
