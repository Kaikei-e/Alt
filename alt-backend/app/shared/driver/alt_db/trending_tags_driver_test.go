package alt_db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestFetchTagArticleCounts_NilPool(t *testing.T) {
	repo := &TagRepository{pool: nil}
	_, err := repo.FetchTagArticleCounts(context.Background(), uuid.New(), time.Now().Add(-7*24*time.Hour))
	assert.Error(t, err, "should return error with nil pool")
}

func TestFetchTagArticleCounts_QueryContainsExpectedClauses(t *testing.T) {
	query := buildTagArticleCountsQuery()
	assert.Contains(t, query, "feed_tags")
	assert.Contains(t, query, "article_tags")
	assert.Contains(t, query, "articles")
	assert.Contains(t, query, "a.created_at >= $1")
	assert.Contains(t, query, "a.user_id = $2")
	assert.Contains(t, query, "a.deleted_at IS NULL")
	assert.Contains(t, query, "GROUP BY ft.tag_name")
	assert.NotContains(t, query, "CASE", "SQL should not contain business logic (CASE expressions)")
	assert.NotContains(t, query, "surge", "SQL should not contain trending logic")
}
