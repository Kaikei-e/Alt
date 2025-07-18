// ABOUTME: This file tests the retry mechanism with exponential backoff and jitter
// ABOUTME: Implements comprehensive TDD tests for error handling and recovery
package retry

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Only errors in tests
	}))
}

// TDD RED PHASE: Test retrier interface and basic functionality
func TestRetrier_Do(t *testing.T) {
	tests := map[string]struct {
		operation     func() error
		expectedCalls int
		wantErr       bool
		description   string
	}{
		"success on first attempt": {
			operation:     func() error { return nil },
			expectedCalls: 1,
			wantErr:       false,
			description:   "Should succeed immediately without retries",
		},
		"success on second attempt": {
			operation: func() func() error {
				attempt := 0
				return func() error {
					attempt++
					if attempt == 1 {
						return errors.New("temporary error")
					}
					return nil
				}
			}(),
			expectedCalls: 2,
			wantErr:       false,
			description:   "Should succeed after one retry",
		},
		"failure after max attempts": {
			operation:     func() error { return errors.New("temporary error") },
			expectedCalls: 3,
			wantErr:       true,
			description:   "Should fail after all retry attempts exhausted",
		},
		"non-retryable error fails immediately": {
			operation:     func() error { return errors.New("non-retryable error") },
			expectedCalls: 1,
			wantErr:       true,
			description:   "Non-retryable errors should fail without retries",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			config := RetryConfig{
				MaxAttempts:   3,
				BaseDelay:     1 * time.Millisecond,
				MaxDelay:      10 * time.Millisecond,
				BackoffFactor: 2.0,
				JitterFactor:  0.1,
			}

			calls := 0
			wrappedOp := func() error {
				calls++
				return tc.operation()
			}

			// Simple classifier for testing
			classifier := func(err error) bool {
				return err.Error() == "temporary error"
			}

			retrier := NewRetrier(config, classifier, testLogger())

			err := retrier.Do(context.Background(), wrappedOp)

			assert.Equal(t, tc.expectedCalls, calls, tc.description)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TDD RED PHASE: Test context cancellation during retry
func TestRetrier_Do_ContextCancellation(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:   5,
		BaseDelay:     100 * time.Millisecond,
		MaxDelay:      1 * time.Second,
		BackoffFactor: 2.0,
		JitterFactor:  0.1,
	}

	calls := 0
	operation := func() error {
		calls++
		return errors.New("temporary error")
	}

	classifier := func(err error) bool { return true } // Always retryable

	retrier := NewRetrier(config, classifier, testLogger())

	// Cancel context after short delay
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := retrier.Do(ctx, operation)
	duration := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "retry cancelled")
	assert.Less(t, duration, 200*time.Millisecond, "Should cancel quickly")
	assert.GreaterOrEqual(t, calls, 1, "Should make at least one attempt")
}

// TDD RED PHASE: Test exponential backoff calculation
func TestRetrier_CalculateDelay(t *testing.T) {
	config := RetryConfig{
		BaseDelay:     100 * time.Millisecond,
		MaxDelay:      1 * time.Second,
		BackoffFactor: 2.0,
		JitterFactor:  0.1,
	}

	retrier := NewRetrier(config, nil, testLogger())

	tests := []struct {
		attempt  int
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{1, 90 * time.Millisecond, 110 * time.Millisecond},    // 100ms ± 10%
		{2, 180 * time.Millisecond, 220 * time.Millisecond},   // 200ms ± 10%
		{3, 360 * time.Millisecond, 440 * time.Millisecond},   // 400ms ± 10%
		{10, 900 * time.Millisecond, 1100 * time.Millisecond}, // Should cap at MaxDelay ± 10%
	}

	for _, tc := range tests {
		delay := retrier.calculateDelay(tc.attempt)
		assert.GreaterOrEqual(t, delay, tc.minDelay, "Delay too small for attempt %d", tc.attempt)
		assert.LessOrEqual(t, delay, tc.maxDelay, "Delay too large for attempt %d", tc.attempt)
	}
}

// TDD RED PHASE: Test retry with timeout context
func TestRetrier_Do_WithTimeout(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:   10,
		BaseDelay:     50 * time.Millisecond,
		MaxDelay:      200 * time.Millisecond,
		BackoffFactor: 2.0,
		JitterFactor:  0.1,
	}

	calls := 0
	operation := func() error {
		calls++
		return errors.New("temporary error")
	}

	classifier := func(err error) bool { return true }
	retrier := NewRetrier(config, classifier, testLogger())

	// Short timeout should prevent all retries
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := retrier.Do(ctx, operation)
	duration := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "retry cancelled")
	assert.Less(t, duration, 200*time.Millisecond, "Should timeout quickly")
	assert.Greater(t, calls, 0, "Should make at least one attempt")
	assert.Less(t, calls, 10, "Should not complete all attempts due to timeout")
}

// TDD RED PHASE: Test config validation
func TestNewRetrier(t *testing.T) {
	t.Run("should create retrier with valid config", func(t *testing.T) {
		config := RetryConfig{
			MaxAttempts:   3,
			BaseDelay:     1 * time.Second,
			MaxDelay:      30 * time.Second,
			BackoffFactor: 2.0,
			JitterFactor:  0.1,
		}

		classifier := func(error) bool { return true }
		retrier := NewRetrier(config, classifier, testLogger())

		assert.NotNil(t, retrier)
		assert.Equal(t, config.MaxAttempts, retrier.config.MaxAttempts)
		assert.Equal(t, config.BaseDelay, retrier.config.BaseDelay)
	})
}

// TDD RED PHASE: Test comprehensive performance logging
func TestRetrier_PerformanceLogging(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:   3,
		BaseDelay:     1 * time.Millisecond,
		MaxDelay:      10 * time.Millisecond,
		BackoffFactor: 2.0,
		JitterFactor:  0.1,
	}

	attempts := 0
	operation := func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	}

	classifier := func(err error) bool { return err.Error() == "temporary error" }
	retrier := NewRetrier(config, classifier, testLogger())

	start := time.Now()
	err := retrier.Do(context.Background(), operation)
	totalDuration := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, 3, attempts)
	assert.Greater(t, totalDuration, time.Duration(0), "Should measure execution time")
}
