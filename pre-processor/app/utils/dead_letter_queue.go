// ABOUTME: This file implements a simple dead letter queue for failed operations
// ABOUTME: Provides error recovery mechanisms and retry functionality
package utils

import (
	"context"
	"log/slog"
	"sync"
	"time"

	operrors "pre-processor/utils/errors"
)

// DeadLetterItem represents an item in the dead letter queue
type DeadLetterItem struct {
	ID          string
	Data        interface{}
	Error       error
	Timestamp   time.Time
	RetryCount  int
	MaxRetries  int
	NextRetry   time.Time
}

// DeadLetterQueue manages failed operations for retry
type DeadLetterQueue struct {
	items   map[string]*DeadLetterItem
	logger  *slog.Logger
	mu      sync.RWMutex
}

// DeadLetterMetrics holds metrics for the dead letter queue
type DeadLetterMetrics struct {
	TotalItems    int
	RetryingItems int
	FailedItems   int
	ProcessedItems int64
}

// NewDeadLetterQueue creates a new dead letter queue
func NewDeadLetterQueue(logger *slog.Logger) *DeadLetterQueue {
	if logger == nil {
		logger = slog.Default()
	}
	
	return &DeadLetterQueue{
		items:  make(map[string]*DeadLetterItem),
		logger: logger,
	}
}

// Add adds an item to the dead letter queue
func (dlq *DeadLetterQueue) Add(id string, data interface{}, err error, maxRetries int) {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()
	
	item := &DeadLetterItem{
		ID:         id,
		Data:       data,
		Error:      err,
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: maxRetries,
		NextRetry:  time.Now().Add(time.Minute), // Initial 1-minute delay
	}
	
	dlq.items[id] = item
	
	dlq.logger.Warn("item added to dead letter queue",
		"id", id,
		"error", err,
		"max_retries", maxRetries,
	)
}

// AddOperationError adds an OperationError to the dead letter queue
func (dlq *DeadLetterQueue) AddOperationError(id string, data interface{}, opErr *operrors.OperationError, maxRetries int) {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()
	
	item := &DeadLetterItem{
		ID:         id,
		Data:       data,
		Error:      opErr,
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: maxRetries,
		NextRetry:  time.Now().Add(time.Minute), // Initial 1-minute delay
	}
	
	dlq.items[id] = item
	
	dlq.logger.Warn("operation error added to dead letter queue",
		"id", id,
		"operation", opErr.Operation,
		"request_id", opErr.RequestID,
		"trace_id", opErr.TraceID,
		"retryable", opErr.Retryable,
		"max_retries", maxRetries,
	)
}

// GetRetryableItems returns items ready for retry
func (dlq *DeadLetterQueue) GetRetryableItems() []*DeadLetterItem {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()
	
	now := time.Now()
	var retryable []*DeadLetterItem
	
	for _, item := range dlq.items {
		if item.RetryCount < item.MaxRetries && now.After(item.NextRetry) {
			retryable = append(retryable, item)
		}
	}
	
	return retryable
}

// MarkRetried marks an item as retried (either success or failure)
func (dlq *DeadLetterQueue) MarkRetried(id string, success bool, err error) {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()
	
	item, exists := dlq.items[id]
	if !exists {
		return
	}
	
	item.RetryCount++
	
	if success {
		delete(dlq.items, id)
		dlq.logger.Info("item successfully retried and removed from dead letter queue",
			"id", id,
			"retry_count", item.RetryCount,
		)
		return
	}
	
	// Update error and calculate next retry time
	item.Error = err
	
	if item.RetryCount >= item.MaxRetries {
		dlq.logger.Error("item exceeded max retries",
			"id", id,
			"retry_count", item.RetryCount,
			"max_retries", item.MaxRetries,
			"error", err,
		)
		// Keep in queue but don't retry
		return
	}
	
	// Exponential backoff: 1min, 2min, 4min, 8min, etc.
	backoff := time.Duration(1<<item.RetryCount) * time.Minute
	if backoff > 30*time.Minute {
		backoff = 30 * time.Minute // Cap at 30 minutes
	}
	
	item.NextRetry = time.Now().Add(backoff)
	
	dlq.logger.Warn("item retry failed, scheduled for next attempt",
		"id", id,
		"retry_count", item.RetryCount,
		"next_retry", item.NextRetry,
		"error", err,
	)
}

// MarkRetriedWithOperationError marks an item as retried with OperationError context
func (dlq *DeadLetterQueue) MarkRetriedWithOperationError(id string, success bool, opErr *operrors.OperationError) {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()
	
	item, exists := dlq.items[id]
	if !exists {
		return
	}
	
	item.RetryCount++
	
	if success {
		delete(dlq.items, id)
		dlq.logger.Info("operation successfully retried and removed from dead letter queue",
			"id", id,
			"operation", opErr.Operation,
			"retry_count", item.RetryCount,
			"request_id", opErr.RequestID,
			"trace_id", opErr.TraceID,
		)
		return
	}
	
	// Update error and calculate next retry time
	item.Error = opErr
	
	if item.RetryCount >= item.MaxRetries {
		dlq.logger.Error("operation exceeded max retries",
			"id", id,
			"operation", opErr.Operation,
			"retry_count", item.RetryCount,
			"max_retries", item.MaxRetries,
			"request_id", opErr.RequestID,
			"trace_id", opErr.TraceID,
			"error", opErr.Underlying,
		)
		// Keep in queue but don't retry
		return
	}
	
	// Exponential backoff: 1min, 2min, 4min, 8min, etc.
	backoff := time.Duration(1<<item.RetryCount) * time.Minute
	if backoff > 30*time.Minute {
		backoff = 30 * time.Minute // Cap at 30 minutes
	}
	
	item.NextRetry = time.Now().Add(backoff)
	
	dlq.logger.Warn("operation retry failed, scheduled for next attempt",
		"id", id,
		"operation", opErr.Operation,
		"retry_count", item.RetryCount,
		"next_retry", item.NextRetry,
		"request_id", opErr.RequestID,
		"trace_id", opErr.TraceID,
		"error", opErr.Underlying,
	)
}

// Remove removes an item from the dead letter queue
func (dlq *DeadLetterQueue) Remove(id string) {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()
	
	delete(dlq.items, id)
	dlq.logger.Info("item removed from dead letter queue", "id", id)
}

// Metrics returns current metrics for the dead letter queue
func (dlq *DeadLetterQueue) Metrics() DeadLetterMetrics {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()
	
	metrics := DeadLetterMetrics{
		TotalItems: len(dlq.items),
	}
	
	now := time.Now()
	for _, item := range dlq.items {
		if item.RetryCount >= item.MaxRetries {
			metrics.FailedItems++
		} else if now.After(item.NextRetry) {
			metrics.RetryingItems++
		}
	}
	
	return metrics
}

// ProcessRetries processes all retryable items using the provided function
func (dlq *DeadLetterQueue) ProcessRetries(ctx context.Context, processor func(context.Context, *DeadLetterItem) error) {
	retryable := dlq.GetRetryableItems()
	
	dlq.logger.Info("processing retryable items",
		"count", len(retryable),
	)
	
	for _, item := range retryable {
		select {
		case <-ctx.Done():
			dlq.logger.Info("retry processing cancelled")
			return
		default:
		}
		
		err := processor(ctx, item)
		dlq.MarkRetried(item.ID, err == nil, err)
	}
}

// StartRetryWorker starts a background worker that processes retries periodically
func (dlq *DeadLetterQueue) StartRetryWorker(ctx context.Context, interval time.Duration, processor func(context.Context, *DeadLetterItem) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	dlq.logger.Info("starting dead letter queue retry worker",
		"interval", interval,
	)
	
	for {
		select {
		case <-ctx.Done():
			dlq.logger.Info("dead letter queue retry worker stopped")
			return
		case <-ticker.C:
			dlq.ProcessRetries(ctx, processor)
		}
	}
}

// ProcessRetriesWithExecutor processes retryable items using a retry executor
func (dlq *DeadLetterQueue) ProcessRetriesWithExecutor(ctx context.Context, executor *operrors.RetryExecutor, processor func(context.Context, *DeadLetterItem) error) {
	retryable := dlq.GetRetryableItems()
	
	dlq.logger.Info("processing retryable items with retry executor",
		"count", len(retryable),
	)
	
	for _, item := range retryable {
		select {
		case <-ctx.Done():
			dlq.logger.Info("retry processing cancelled")
			return
		default:
		}
		
		// Use the retry executor to handle the operation
		operation := func() error {
			return processor(ctx, item)
		}
		
		err := executor.Execute(ctx, operation)
		
		// Handle the result based on error type
		if err == nil {
			dlq.MarkRetried(item.ID, true, nil)
		} else if opErr, ok := err.(*operrors.OperationError); ok {
			// Use the enhanced method for OperationError
			dlq.MarkRetriedWithOperationError(item.ID, false, opErr)
		} else {
			// Fall back to regular method for other errors
			dlq.MarkRetried(item.ID, false, err)
		}
	}
}