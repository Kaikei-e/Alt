package repository

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"pre-processor/domain"

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
		var _ = repo

		assert.NotNil(t, repo)
	})
}

func TestArticleRepository_Create(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		// GREEN PHASE: Test minimal implementation

		repo := NewArticleRepository(nil, testLoggerRepo())

		err := repo.Create(context.Background(), &domain.Article{})

		// Should return error due to nil database
		assert.Error(t, err)
	})
}

func TestArticleRepository_CheckExists(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		// GREEN PHASE: Test minimal implementation

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

		repo := NewArticleRepository(nil, testLoggerRepo())

		hasUnsummarized, err := repo.HasUnsummarizedArticles(context.Background())

		// Should return error due to nil database
		assert.Error(t, err)
		assert.False(t, hasUnsummarized)
	})
}

func TestArticleRepository_UpsertArticles(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		repo := NewArticleRepository(nil, testLoggerRepo())

		err := repo.UpsertArticles(context.Background(), []*domain.Article{
			{
				Title:   "Test",
				Content: "Content",
				URL:     "https://example.com",
				FeedURL: "https://feed.example.com/rss",
				UserID:  "user-123",
			},
		})

		// Should return error due to nil database (when trying to resolve FeedID)
		assert.Error(t, err)
	})

	t.Run("should return nil for empty articles slice", func(t *testing.T) {
		repo := NewArticleRepository(nil, testLoggerRepo())

		err := repo.UpsertArticles(context.Background(), []*domain.Article{})

		assert.NoError(t, err)
	})

	t.Run("should skip articles with empty FeedURL and log warning", func(t *testing.T) {
		// RED PHASE: This test should fail until we add FeedURL validation in UpsertArticles
		repo := NewArticleRepository(nil, testLoggerRepo())

		// Article with empty FeedURL should be skipped
		articles := []*domain.Article{
			{
				Title:   "Article without FeedURL",
				Content: "Content",
				URL:     "https://example.com/1",
				FeedURL: "", // Empty FeedURL - should be skipped
				UserID:  "user-123",
			},
		}

		// When all articles are skipped, it should return nil without error
		err := repo.UpsertArticles(context.Background(), articles)

		// After implementing the fix, this should pass
		assert.NoError(t, err, "should return nil when all articles are skipped due to empty FeedURL")
	})
}
