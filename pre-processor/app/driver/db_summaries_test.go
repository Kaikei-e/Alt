package driver

import (
	"context"
	"testing"

	"pre-processor/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateArticleSummary_NilDatabase(t *testing.T) {
	ctx := context.Background()
	articleSummary := &models.ArticleSummary{
		ArticleID:       uuid.New().String(),
		ArticleTitle:    "Test",
		SummaryJapanese: "テスト",
	}

	err := CreateArticleSummary(ctx, nil, articleSummary)
	require.Error(t, err)
	assert.Equal(t, "database connection is nil", err.Error())
}

func TestGetArticleSummaryByArticleID_NilDatabase(t *testing.T) {
	ctx := context.Background()
	articleID := uuid.New().String()

	summary, err := GetArticleSummaryByArticleID(ctx, nil, articleID)
	require.Error(t, err)
	assert.Nil(t, summary)
	assert.Equal(t, "database connection is nil", err.Error())
}

// Note: Integration tests with real database connections should be added separately
// to test the following scenarios:
//
// 1. CreateArticleSummary with existing article - should succeed
// 2. CreateArticleSummary with non-existent article - should fail with "does not exist" error
// 3. CreateArticleSummary with existing summary - should update (UPSERT)
// 4. GetArticleSummaryByArticleID with existing summary - should return summary
// 5. GetArticleSummaryByArticleID with non-existent summary - should return error
//
// These tests require a test database with proper schema and test data.
