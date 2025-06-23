package service

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func testLoggerSummarizer() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Only errors in tests
	}))
}

func TestArticleSummarizerService_InterfaceCompliance(t *testing.T) {
	t.Run("should implement ArticleSummarizerService interface", func(t *testing.T) {
		// GREEN PHASE: Test that service implements interface
		service := NewArticleSummarizerService(nil, nil, nil, testLoggerSummarizer())

		// Verify interface compliance at compile time
		var _ ArticleSummarizerService = service
		assert.NotNil(t, service)
	})
}

func TestArticleSummarizerService_SummarizeArticles(t *testing.T) {
	t.Run("should return empty result with minimal implementation", func(t *testing.T) {
		// GREEN PHASE: Test minimal implementation
		service := NewArticleSummarizerService(nil, nil, nil, testLoggerSummarizer())

		result, err := service.SummarizeArticles(context.Background(), 10)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ProcessedCount)
		assert.Equal(t, 0, result.SuccessCount)
		assert.Equal(t, 0, result.ErrorCount)
		assert.False(t, result.HasMore)
		assert.Empty(t, result.Errors)
	})
}

func TestArticleSummarizerService_HasUnsummarizedArticles(t *testing.T) {
	t.Run("should return false with minimal implementation", func(t *testing.T) {
		// GREEN PHASE: Test minimal implementation
		service := NewArticleSummarizerService(nil, nil, nil, testLoggerSummarizer())

		hasArticles, err := service.HasUnsummarizedArticles(context.Background())

		assert.NoError(t, err)
		assert.False(t, hasArticles)
	})
}

func TestArticleSummarizerService_ResetPagination(t *testing.T) {
	t.Run("should reset pagination without error", func(t *testing.T) {
		// GREEN PHASE: Test minimal implementation
		service := NewArticleSummarizerService(nil, nil, nil, testLoggerSummarizer())

		err := service.ResetPagination()

		assert.NoError(t, err)
	})
}
