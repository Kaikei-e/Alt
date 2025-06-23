package service

import (
	"context"
	"log/slog"
	"os"
	"pre-processor/logger"
	"testing"

	"github.com/stretchr/testify/assert"
)

func testLoggerFetcher() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Only errors in tests
	}))
}

func TestArticleFetcherService_InterfaceCompliance(t *testing.T) {
	t.Run("should implement ArticleFetcherService interface", func(t *testing.T) {
		// GREEN PHASE: Test that service implements interface
		service := NewArticleFetcherService(testLoggerFetcher())

		// Verify interface compliance at compile time
		var _ ArticleFetcherService = service
		assert.NotNil(t, service)
	})
}

func TestArticleFetcherService_FetchArticle(t *testing.T) {
	t.Run("should handle invalid URL gracefully", func(t *testing.T) {
		// GREEN PHASE: Test minimal implementation with invalid URL
		// Initialize global logger for article-fetcher dependency
		logger.Init()

		service := NewArticleFetcherService(testLoggerFetcher())

		article, err := service.FetchArticle(context.Background(), "invalid-url")

		assert.Error(t, err)
		assert.Nil(t, article)
	})
}

func TestArticleFetcherService_ValidateURL(t *testing.T) {
	t.Run("should validate URL format", func(t *testing.T) {
		// GREEN PHASE: Test minimal implementation
		service := NewArticleFetcherService(testLoggerFetcher())

		// Valid URL should pass
		err := service.ValidateURL("https://example.com")
		assert.NoError(t, err)

		// Invalid URL should fail (use a clearly malformed URL)
		err = service.ValidateURL("://invalid")
		assert.Error(t, err)
	})
}
