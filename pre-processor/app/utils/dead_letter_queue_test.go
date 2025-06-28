// ABOUTME: This file contains tests for dead letter queue implementation
// ABOUTME: Tests error recovery and retry mechanisms
package utils

import (
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDeadLetterQueue_BasicOperations(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dlq := NewDeadLetterQueue(logger)
	
	// Add item
	dlq.Add("test-1", "test data", errors.New("test error"), 3)
	
	metrics := dlq.Metrics()
	assert.Equal(t, 1, metrics.TotalItems)
	
	// Remove item
	dlq.Remove("test-1")
	
	metrics = dlq.Metrics()
	assert.Equal(t, 0, metrics.TotalItems)
}

func TestDeadLetterQueue_RetryLogic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dlq := NewDeadLetterQueue(logger)
	
	// Add item with immediate retry
	dlq.Add("test-1", "test data", errors.New("test error"), 3)
	
	// Manually set next retry to past time for immediate retry
	dlq.mu.Lock()
	dlq.items["test-1"].NextRetry = time.Now().Add(-time.Minute)
	dlq.mu.Unlock()
	
	retryable := dlq.GetRetryableItems()
	assert.Len(t, retryable, 1)
	
	// Mark as failed retry
	dlq.MarkRetried("test-1", false, errors.New("retry failed"))
	
	metrics := dlq.Metrics()
	assert.Equal(t, 1, metrics.TotalItems)
	
	// Mark as successful retry
	dlq.MarkRetried("test-1", true, nil)
	
	metrics = dlq.Metrics()
	assert.Equal(t, 0, metrics.TotalItems)
}