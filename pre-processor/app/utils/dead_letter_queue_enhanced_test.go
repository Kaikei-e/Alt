// ABOUTME: This file tests enhanced dead letter queue with OperationError integration
package utils

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	operrors "pre-processor/utils/errors"
)

func TestDeadLetterQueue_WithOperationError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dlq := NewDeadLetterQueue(logger)

	// Create OperationError with context
	ctx := context.Background()
	ctx = operrors.WithRequestID(ctx, "req-12345")
	ctx = operrors.WithTraceID(ctx, "trace-67890")

	baseErr := errors.New("network timeout")
	opErr := operrors.NewOperationError("feed_processing", baseErr, true).WithContext(ctx)

	// Add OperationError to DLQ
	dlq.AddOperationError("test-1", "test data", opErr, 3)

	// Verify item was added with correct context
	dlq.mu.RLock()
	item, exists := dlq.items["test-1"]
	dlq.mu.RUnlock()

	require.True(t, exists)
	assert.Equal(t, "test-1", item.ID)
	assert.Equal(t, "test data", item.Data)

	// Verify OperationError is preserved
	assert.IsType(t, &operrors.OperationError{}, item.Error)
	itemOpErr := item.Error.(*operrors.OperationError)
	assert.Equal(t, "feed_processing", itemOpErr.Operation)
	assert.Equal(t, "req-12345", itemOpErr.RequestID)
	assert.Equal(t, "trace-67890", itemOpErr.TraceID)
	assert.True(t, itemOpErr.Retryable)
}

func TestDeadLetterQueue_RetryWithOperationError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dlq := NewDeadLetterQueue(logger)

	// Create initial OperationError
	ctx := context.Background()
	ctx = operrors.WithRequestID(ctx, "req-12345")

	baseErr := errors.New("network timeout")
	opErr := operrors.NewOperationError("feed_processing", baseErr, true).WithContext(ctx)

	// Add to DLQ
	dlq.AddOperationError("test-1", "test data", opErr, 3)

	// Set next retry to past for immediate retry
	dlq.mu.Lock()
	dlq.items["test-1"].NextRetry = time.Now().Add(-time.Minute)
	dlq.mu.Unlock()

	// Get retryable items
	retryable := dlq.GetRetryableItems()
	require.Len(t, retryable, 1)

	// Create new OperationError for retry failure with updated context
	newCtx := operrors.WithTraceID(ctx, "trace-retry-1")
	retryErr := operrors.NewOperationError("feed_processing", errors.New("still failing"), true).WithContext(newCtx)

	// Mark retry as failed with new OperationError
	dlq.MarkRetriedWithOperationError("test-1", false, retryErr)

	// Verify error context was updated
	dlq.mu.RLock()
	item := dlq.items["test-1"]
	dlq.mu.RUnlock()

	require.NotNil(t, item)
	assert.Equal(t, 1, item.RetryCount)

	itemOpErr := item.Error.(*operrors.OperationError)
	assert.Equal(t, "trace-retry-1", itemOpErr.TraceID)
	assert.Equal(t, "still failing", itemOpErr.Underlying.Error())
}

func TestDeadLetterQueue_ProcessWithRetryExecutor(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dlq := NewDeadLetterQueue(logger)

	// Create retry policy and executor
	policy := operrors.NewRetryPolicy(2, 1*time.Millisecond)
	executor := operrors.NewRetryExecutor(policy)

	// Add items to DLQ
	ctx := context.Background()
	ctx = operrors.WithRequestID(ctx, "req-batch-1")

	opErr1 := operrors.NewOperationError("feed_processing", errors.New("timeout"), true).WithContext(ctx)
	opErr2 := operrors.NewOperationError("validation", errors.New("invalid format"), false).WithContext(ctx)

	dlq.AddOperationError("retryable-1", "data1", opErr1, 3)
	dlq.AddOperationError("non-retryable-1", "data2", opErr2, 3)

	// Set both items for immediate retry
	dlq.mu.Lock()
	dlq.items["retryable-1"].NextRetry = time.Now().Add(-time.Minute)
	dlq.items["non-retryable-1"].NextRetry = time.Now().Add(-time.Minute)
	dlq.mu.Unlock()

	callCount := 0
	processor := func(ctx context.Context, item *DeadLetterItem) error {
		callCount++

		// Simulate retry logic based on error type
		if itemOpErr, ok := item.Error.(*operrors.OperationError); ok {
			if !itemOpErr.Retryable {
				// Non-retryable errors should not be retried by the processor
				return itemOpErr
			}

			// Simulate retryable operations succeeding on the first attempt
			if item.ID == "retryable-1" {
				return nil // Success
			}

			// Otherwise return the same error to trigger retry
			return itemOpErr
		}

		return item.Error
	}

	// Process retries
	dlq.ProcessRetriesWithExecutor(ctx, executor, processor)

	// Verify results
	dlq.mu.RLock()
	_, retryableExists := dlq.items["retryable-1"]
	nonRetryableItem, nonRetryableExists := dlq.items["non-retryable-1"]
	dlq.mu.RUnlock()

	// Retryable item should be removed after successful retry
	assert.False(t, retryableExists)

	// Non-retryable item should remain in queue without additional retries
	assert.True(t, nonRetryableExists)
	assert.Equal(t, 1, nonRetryableItem.RetryCount) // Only one attempt

	assert.GreaterOrEqual(t, callCount, 2) // At least one call for each item
}

func TestDeadLetterQueue_MetricsWithOperationErrors(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dlq := NewDeadLetterQueue(logger)

	// Add various types of errors
	retryableErr := operrors.NewOperationError("processing", errors.New("timeout"), true)
	nonRetryableErr := operrors.NewOperationError("validation", errors.New("invalid"), false)
	regularErr := errors.New("regular error")

	dlq.AddOperationError("retryable-1", "data1", retryableErr, 3)
	dlq.AddOperationError("non-retryable-1", "data2", nonRetryableErr, 3)
	dlq.Add("regular-1", "data3", regularErr, 3)

	// Set one item to max retries
	dlq.mu.Lock()
	dlq.items["retryable-1"].RetryCount = 3                               // Exceeded max retries
	dlq.items["non-retryable-1"].NextRetry = time.Now().Add(-time.Minute) // Ready for retry
	dlq.mu.Unlock()

	metrics := dlq.Metrics()

	assert.Equal(t, 3, metrics.TotalItems)
	assert.Equal(t, 1, metrics.FailedItems)   // retryable-1 exceeded max retries
	assert.Equal(t, 1, metrics.RetryingItems) // non-retryable-1 ready for retry
}
