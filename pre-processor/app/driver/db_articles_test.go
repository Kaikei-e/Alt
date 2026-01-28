package driver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"pre-processor/models"
)

func TestUpsertArticlesBatch_Validation(t *testing.T) {
	t.Run("should skip articles with empty UserID", func(t *testing.T) {
		articles := []*models.Article{
			{
				Title:   "Article without UserID",
				Content: "Some content",
				URL:     "https://example.com/1",
				FeedID:  "feed-123",
				UserID:  "", // Empty UserID - should be skipped
			},
		}

		// With nil db, the article is skipped due to empty UserID,
		// resulting in an empty batch which returns nil
		err := UpsertArticlesBatch(context.Background(), nil, articles)

		assert.NoError(t, err, "should return nil when all articles are skipped due to validation")
	})

	t.Run("should skip articles with empty FeedID", func(t *testing.T) {
		// RED PHASE: This test should fail until we add FeedID validation
		articles := []*models.Article{
			{
				Title:   "Article without FeedID",
				Content: "Some content",
				URL:     "https://example.com/1",
				FeedID:  "", // Empty FeedID - should be skipped
				UserID:  "user-123",
			},
		}

		// With nil db and article that should be skipped due to empty FeedID,
		// if the validation works correctly, the batch will be empty and
		// the function should return nil (not error) for empty valid batch
		err := UpsertArticlesBatch(context.Background(), nil, articles)

		// After FeedID validation is added, this should return nil
		// because the article is skipped and batch becomes empty
		// Currently this will fail because FeedID validation is not implemented
		assert.NoError(t, err, "should return nil when all articles are skipped due to validation")
	})

	t.Run("should return nil for empty articles slice", func(t *testing.T) {
		err := UpsertArticlesBatch(context.Background(), nil, []*models.Article{})

		assert.NoError(t, err, "empty slice should return nil")
	})

	t.Run("should return error for nil db with valid articles", func(t *testing.T) {
		articles := []*models.Article{
			{
				Title:   "Valid Article",
				Content: "Some content",
				URL:     "https://example.com/1",
				FeedID:  "feed-123",
				UserID:  "user-123",
			},
		}

		err := UpsertArticlesBatch(context.Background(), nil, articles)

		assert.Error(t, err, "nil db should return error with valid articles")
	})
}
