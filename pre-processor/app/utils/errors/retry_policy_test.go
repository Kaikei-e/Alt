// ABOUTME: This file tests retry policy and exponential backoff mechanisms
package errors

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryPolicy_New(t *testing.T) {
	tests := map[string]struct {
		maxAttempts int
		baseDelay   time.Duration
		expected    *RetryPolicy
	}{
		"standard policy": {
			maxAttempts: 3,
			baseDelay:   100 * time.Millisecond,
			expected: &RetryPolicy{
				MaxAttempts: 3,
				BaseDelay:   100 * time.Millisecond,
				MaxDelay:    30 * time.Second,
				Multiplier:  2.0,
				Jitter:      true,
			},
		},
		"zero attempts": {
			maxAttempts: 0,
			baseDelay:   time.Second,
			expected: &RetryPolicy{
				MaxAttempts: 0,
				BaseDelay:   time.Second,
				MaxDelay:    30 * time.Second,
				Multiplier:  2.0,
				Jitter:      true,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			policy := NewRetryPolicy(tc.maxAttempts, tc.baseDelay)

			assert.Equal(t, tc.expected.MaxAttempts, policy.MaxAttempts)
			assert.Equal(t, tc.expected.BaseDelay, policy.BaseDelay)
			assert.Equal(t, tc.expected.MaxDelay, policy.MaxDelay)
			assert.Equal(t, tc.expected.Multiplier, policy.Multiplier)
			assert.Equal(t, tc.expected.Jitter, policy.Jitter)
		})
	}
}

func TestRetryPolicy_CalculateDelay(t *testing.T) {
	policy := &RetryPolicy{
		MaxAttempts: 5,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    1 * time.Second,
		Multiplier:  2.0,
		Jitter:      false, // Disable jitter for predictable testing
	}

	tests := map[string]struct {
		attempt  int
		expected time.Duration
	}{
		"first retry": {
			attempt:  1,
			expected: 100 * time.Millisecond,
		},
		"second retry": {
			attempt:  2,
			expected: 200 * time.Millisecond,
		},
		"third retry": {
			attempt:  3,
			expected: 400 * time.Millisecond,
		},
		"fourth retry": {
			attempt:  4,
			expected: 800 * time.Millisecond,
		},
		"fifth retry (capped)": {
			attempt:  5,
			expected: 1 * time.Second, // Capped at MaxDelay
		},
		"excessive retry (capped)": {
			attempt:  10,
			expected: 1 * time.Second, // Capped at MaxDelay
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			delay := policy.CalculateDelay(tc.attempt)
			assert.Equal(t, tc.expected, delay)
		})
	}
}

func TestRetryPolicy_CalculateDelayWithJitter(t *testing.T) {
	policy := &RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    10 * time.Second,
		Multiplier:  2.0,
		Jitter:      true,
	}

	// Test that jitter produces delays within expected range
	expectedBase := 200 * time.Millisecond // Second attempt
	delay := policy.CalculateDelay(2)

	// With jitter, delay should be between 50% and 100% of base delay
	minDelay := time.Duration(float64(expectedBase) * 0.5)
	maxDelay := expectedBase

	assert.GreaterOrEqual(t, delay, minDelay)
	assert.LessOrEqual(t, delay, maxDelay)
}

func TestRetryExecutor_Execute_Success(t *testing.T) {
	policy := NewRetryPolicy(3, 10*time.Millisecond)
	executor := NewRetryExecutor(policy)

	callCount := 0
	operation := func() error {
		callCount++
		return nil // Success on first try
	}

	ctx := context.Background()
	err := executor.Execute(ctx, operation)

	assert.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

func TestRetryExecutor_Execute_RetryableError(t *testing.T) {
	policy := NewRetryPolicy(3, 1*time.Millisecond)
	executor := NewRetryExecutor(policy)

	callCount := 0
	operation := func() error {
		callCount++
		if callCount < 3 {
			// Return retryable error for first two attempts
			return NewOperationError("test", errors.New("network timeout"), true)
		}
		return nil // Success on third try
	}

	ctx := context.Background()
	err := executor.Execute(ctx, operation)

	assert.NoError(t, err)
	assert.Equal(t, 3, callCount)
}

func TestRetryExecutor_Execute_NonRetryableError(t *testing.T) {
	policy := NewRetryPolicy(3, 10*time.Millisecond)
	executor := NewRetryExecutor(policy)

	callCount := 0
	operation := func() error {
		callCount++
		// Return non-retryable error
		return NewOperationError("test", errors.New("validation failed"), false)
	}

	ctx := context.Background()
	err := executor.Execute(ctx, operation)

	assert.Error(t, err)
	assert.Equal(t, 1, callCount) // Should not retry

	var opErr *OperationError
	require.True(t, errors.As(err, &opErr))
	assert.False(t, opErr.Retryable)
}

func TestRetryExecutor_Execute_MaxAttemptsExceeded(t *testing.T) {
	policy := NewRetryPolicy(2, 1*time.Millisecond)
	executor := NewRetryExecutor(policy)

	callCount := 0
	operation := func() error {
		callCount++
		// Always return retryable error
		return NewOperationError("test", errors.New("network timeout"), true)
	}

	ctx := context.Background()
	err := executor.Execute(ctx, operation)

	assert.Error(t, err)
	assert.Equal(t, 2, callCount) // Should try MaxAttempts times

	var opErr *OperationError
	require.True(t, errors.As(err, &opErr))
	assert.True(t, opErr.Retryable)
}

func TestRetryExecutor_Execute_ContextCancellation(t *testing.T) {
	policy := NewRetryPolicy(5, 100*time.Millisecond)
	executor := NewRetryExecutor(policy)

	callCount := 0
	operation := func() error {
		callCount++
		// Always return retryable error
		return NewOperationError("test", errors.New("network timeout"), true)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context after first failure
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := executor.Execute(ctx, operation)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
	assert.Equal(t, 1, callCount) // Should stop retrying when context is canceled
}

func TestRetryExecutor_Execute_RegularError(t *testing.T) {
	policy := NewRetryPolicy(3, 1*time.Millisecond)
	executor := NewRetryExecutor(policy)

	callCount := 0
	operation := func() error {
		callCount++
		// Return regular error (not OperationError)
		return errors.New("regular error")
	}

	ctx := context.Background()
	err := executor.Execute(ctx, operation)

	assert.Error(t, err)
	assert.Equal(t, 1, callCount) // Should not retry regular errors
	assert.Equal(t, "regular error", err.Error())
}

func TestShouldRetry(t *testing.T) {
	tests := map[string]struct {
		err      error
		expected bool
	}{
		"retryable operation error": {
			err:      NewOperationError("test", errors.New("timeout"), true),
			expected: true,
		},
		"non-retryable operation error": {
			err:      NewOperationError("test", errors.New("validation"), false),
			expected: false,
		},
		"regular error": {
			err:      errors.New("regular error"),
			expected: false,
		},
		"nil error": {
			err:      nil,
			expected: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := ShouldRetry(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
