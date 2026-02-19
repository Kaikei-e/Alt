package alt_db

import (
	"context"
	"testing"

	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetFeedURLsByArticleIDs_ReturnsArticleURLs(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	articleIDs := []string{"article-1", "article-2"}

	rows := pgxmock.NewRows([]string{"feed_id", "article_id", "url", "feed_title", "article_title"}).
		AddRow("feed-1", "article-1", "https://example.com/blog/post-123", "Example Feed", "Post 123").
		AddRow("feed-2", "article-2", "https://other.com/news/456", "Other Feed", "News 456")

	mock.ExpectQuery("SELECT").
		WithArgs(articleIDs).
		WillReturnRows(rows)

	results, err := repo.GetFeedURLsByArticleIDs(context.Background(), articleIDs)
	require.NoError(t, err)
	require.Len(t, results, 2)

	// Verify article URLs are returned (not feed homepage URLs)
	assert.Equal(t, "https://example.com/blog/post-123", results[0].URL)
	assert.Equal(t, "https://other.com/news/456", results[1].URL)
	assert.Equal(t, "article-1", results[0].ArticleID)
	assert.Equal(t, "article-2", results[1].ArticleID)
	assert.Equal(t, "feed-1", results[0].FeedID)
	assert.Equal(t, "feed-2", results[1].FeedID)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFeedURLsByArticleIDs_ArticleWithoutFeed(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	articleIDs := []string{"article-orphan"}

	// LEFT JOIN: feed fields are empty strings (COALESCE handles NULLs)
	rows := pgxmock.NewRows([]string{"feed_id", "article_id", "url", "feed_title", "article_title"}).
		AddRow("", "article-orphan", "https://example.com/orphan-post", "", "Orphan Post")

	mock.ExpectQuery("SELECT").
		WithArgs(articleIDs).
		WillReturnRows(rows)

	results, err := repo.GetFeedURLsByArticleIDs(context.Background(), articleIDs)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "https://example.com/orphan-post", results[0].URL)
	assert.Equal(t, "article-orphan", results[0].ArticleID)
	assert.Equal(t, "", results[0].FeedID)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFeedURLsByArticleIDs_EmptyInput(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	results, err := repo.GetFeedURLsByArticleIDs(context.Background(), []string{})
	require.NoError(t, err)
	assert.Nil(t, results)

	require.NoError(t, mock.ExpectationsWereMet())
}
