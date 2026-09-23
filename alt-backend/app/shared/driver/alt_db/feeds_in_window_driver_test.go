package alt_db

import (
	"alt/domain"
	"alt/utils/constants"
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeedRepository_FetchFeedsInWindow_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := NewFeedRepository(mock)

	from := time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	pubDate := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 3, 20, 10, 5, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 3, 20, 10, 5, 0, 0, time.UTC)

	columns := []string{
		"total_count", "id", "title", "description", "website_url",
		"pub_date", "created_at", "updated_at", "article_id",
		"feed_link_id", "og_image_url",
	}

	rows := pgxmock.NewRows(columns).
		AddRow(
			2064,
			"f1e2d3c4-0001-4000-8000-000000000001",
			"Example Headline",
			"<p>Example lede.</p>",
			"https://example.com/post",
			pubDate,
			createdAt,
			updatedAt,
			sql.NullString{String: "art-123", Valid: true},
			sql.NullString{String: "link-456", Valid: true},
			sql.NullString{String: "https://example.com/image.png", Valid: true},
		)

	mock.ExpectQuery(regexp.QuoteMeta(feedsInWindowQuery)).
		WithArgs(from, to, 0, 500).
		WillReturnRows(rows)

	result, err := repo.FetchFeedsInWindow(context.Background(), domain.FeedsInWindowQuery{
		From:     from,
		To:       to,
		Page:     1,
		PageSize: 500,
	})

	require.NoError(t, err)
	assert.Equal(t, 2064, result.Total)
	assert.Equal(t, 1, result.Page)
	assert.Equal(t, 500, result.PageSize)
	assert.True(t, result.HasMore)
	require.Len(t, result.Feeds, 1)

	feed := result.Feeds[0]
	assert.Equal(t, "f1e2d3c4-0001-4000-8000-000000000001", feed.ID)
	assert.Equal(t, "Example Headline", feed.Title)
	assert.Equal(t, "<p>Example lede.</p>", feed.Description)
	assert.Equal(t, "https://example.com/post", feed.WebsiteURL)
	assert.Equal(t, pubDate, feed.PubDate)
	assert.Equal(t, createdAt, feed.CreatedAt)
	assert.Equal(t, updatedAt, feed.UpdatedAt)
	require.NotNil(t, feed.ArticleID)
	assert.Equal(t, "art-123", *feed.ArticleID)
	assert.False(t, feed.IsRead)
	require.NotNil(t, feed.FeedLinkID)
	assert.Equal(t, "link-456", *feed.FeedLinkID)
	require.NotNil(t, feed.OgImageURL)
	assert.Equal(t, "https://example.com/image.png", *feed.OgImageURL)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedRepository_FetchFeedsInWindow_Empty(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := NewFeedRepository(mock)

	from := time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)

	columns := []string{
		"total_count", "id", "title", "description", "website_url",
		"pub_date", "created_at", "updated_at", "article_id",
		"feed_link_id", "og_image_url",
	}

	rows := pgxmock.NewRows(columns)

	mock.ExpectQuery(regexp.QuoteMeta(feedsInWindowQuery)).
		WithArgs(from, to, 0, 500).
		WillReturnRows(rows)

	result, err := repo.FetchFeedsInWindow(context.Background(), domain.FeedsInWindowQuery{
		From:     from,
		To:       to,
		Page:     1,
		PageSize: 500,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.Total)
	assert.False(t, result.HasMore)
	assert.Empty(t, result.Feeds)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedRepository_FetchFeedsInWindow_QueryError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := NewFeedRepository(mock)

	from := time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(feedsInWindowQuery)).
		WithArgs(from, to, 0, 500).
		WillReturnError(errors.New("db query error"))

	result, err := repo.FetchFeedsInWindow(context.Background(), domain.FeedsInWindowQuery{
		From:     from,
		To:       to,
		Page:     1,
		PageSize: 500,
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "fetch feeds in window")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFeedRepository_FetchFeedsInWindow_Validation(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := NewFeedRepository(mock)
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()

	// Nil receiver or pool
	var nilRepo *FeedRepository
	_, err = nilRepo.FetchFeedsInWindow(context.Background(), domain.FeedsInWindowQuery{From: from, To: to, Page: 1, PageSize: 500})
	assert.Error(t, err)

	repoNoPool := &FeedRepository{}
	_, err = repoNoPool.FetchFeedsInWindow(context.Background(), domain.FeedsInWindowQuery{From: from, To: to, Page: 1, PageSize: 500})
	assert.Error(t, err)

	// Page <= 0
	_, err = repo.FetchFeedsInWindow(context.Background(), domain.FeedsInWindowQuery{From: from, To: to, Page: 0, PageSize: 500})
	assert.Error(t, err)

	// PageSize <= 0
	_, err = repo.FetchFeedsInWindow(context.Background(), domain.FeedsInWindowQuery{From: from, To: to, Page: 1, PageSize: 0})
	assert.Error(t, err)

	// PageSize > constants.MaxRecapPageSize
	_, err = repo.FetchFeedsInWindow(context.Background(), domain.FeedsInWindowQuery{From: from, To: to, Page: 1, PageSize: constants.MaxRecapPageSize + 1})
	assert.Error(t, err)
}
