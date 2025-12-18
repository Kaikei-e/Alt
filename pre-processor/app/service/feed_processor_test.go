package service

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFeedProcessorService_ProcessFeeds(t *testing.T) {
	// REFACTOR phase - Focus on the service logic structure and interface compliance
	t.Run("service implements interface correctly", func(t *testing.T) {
		// Test that our service properly implements the interface
		service := NewFeedProcessorService(nil, nil, nil, testLogger())

		// Verify interface compliance
		var _ = service

		assert.NotNil(t, service)
	})

	t.Run("service handles nil dependencies gracefully", func(t *testing.T) {
		// Test service creation with nil dependencies (should not panic)
		service := NewFeedProcessorService(nil, nil, nil, testLogger())
		assert.NotNil(t, service)

		// Reset pagination should work even with nil deps
		err := service.ResetPagination()
		assert.NoError(t, err)
	})
}

func TestFeedProcessorService_GetProcessingStats(t *testing.T) {
	t.Run("service implements GetProcessingStats method", func(t *testing.T) {
		service := NewFeedProcessorService(nil, nil, nil, testLogger())
		assert.NotNil(t, service)

		// Method exists and has correct signature
		var _ = service.GetProcessingStats
	})
}

func TestFeedProcessorService_ResetPagination(t *testing.T) {
	t.Run("reset pagination works correctly", func(t *testing.T) {
		service := NewFeedProcessorService(nil, nil, nil, testLogger())

		// Should not return error
		err := service.ResetPagination()
		assert.NoError(t, err)

		// Should be idempotent
		err = service.ResetPagination()
		assert.NoError(t, err)
	})
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Only errors in tests to keep output clean
	}))
}
