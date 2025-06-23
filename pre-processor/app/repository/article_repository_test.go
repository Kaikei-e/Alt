package repository

import (
	"context"
	"log/slog"
	"os"
	"pre-processor/logger"
	"pre-processor/models"
	"testing"

	"github.com/stretchr/testify/assert"
)

func testLoggerRepo() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Only errors in tests
	}))
}

func TestArticleRepository_InterfaceCompliance(t *testing.T) {
	t.Run("should implement ArticleRepository interface", func(t *testing.T) {
		// GREEN PHASE: Test that repository implements interface
		repo := NewArticleRepository(nil, testLoggerRepo())

		// Verify interface compliance at compile time
		var _ ArticleRepository = repo
		assert.NotNil(t, repo)
	})
}

func TestArticleRepository_Create(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		// GREEN PHASE: Test minimal implementation
		// Initialize global logger for driver dependencies
		logger.Init()

		repo := NewArticleRepository(nil, testLoggerRepo())

		err := repo.Create(context.Background(), &models.Article{})

		// Should return error due to nil database
		assert.Error(t, err)
	})
}

func TestArticleRepository_CheckExists(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		// GREEN PHASE: Test minimal implementation
		logger.Init()

		repo := NewArticleRepository(nil, testLoggerRepo())

		exists, err := repo.CheckExists(context.Background(), []string{"http://example.com"})

		// Should return error due to nil database
		assert.Error(t, err)
		assert.False(t, exists)
	})
}

func TestArticleRepository_FindForSummarization(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		// GREEN PHASE: Test minimal implementation
		logger.Init()

		repo := NewArticleRepository(nil, testLoggerRepo())

		articles, cursor, err := repo.FindForSummarization(context.Background(), nil, 10)

		// Should return error due to nil database
		assert.Error(t, err)
		assert.Nil(t, articles)
		assert.Nil(t, cursor)
	})
}

func TestArticleRepository_HasUnsummarizedArticles(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		// GREEN PHASE: Test minimal implementation
		logger.Init()

		repo := NewArticleRepository(nil, testLoggerRepo())

		hasUnsummarized, err := repo.HasUnsummarizedArticles(context.Background())

		// Should return error due to nil database
		assert.Error(t, err)
		assert.False(t, hasUnsummarized)
	})
}
