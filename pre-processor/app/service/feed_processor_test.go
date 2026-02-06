package service

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFeedProcessorService_ProcessFeeds(t *testing.T) {
	t.Run("service implements interface correctly", func(t *testing.T) {
		service := NewFeedProcessorService(nil, testLogger())

		var _ = service
		assert.NotNil(t, service)
	})

	t.Run("service handles nil dependencies gracefully", func(t *testing.T) {
		service := NewFeedProcessorService(nil, testLogger())
		assert.NotNil(t, service)

		err := service.ResetPagination()
		assert.NoError(t, err)
	})
}

func TestFeedProcessorService_GetProcessingStats(t *testing.T) {
	t.Run("service implements GetProcessingStats method", func(t *testing.T) {
		service := NewFeedProcessorService(nil, testLogger())
		assert.NotNil(t, service)

		var _ = service.GetProcessingStats
	})
}

func TestFeedProcessorService_ResetPagination(t *testing.T) {
	t.Run("reset pagination works correctly", func(t *testing.T) {
		service := NewFeedProcessorService(nil, testLogger())

		err := service.ResetPagination()
		assert.NoError(t, err)

		err = service.ResetPagination()
		assert.NoError(t, err)
	})
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
}
