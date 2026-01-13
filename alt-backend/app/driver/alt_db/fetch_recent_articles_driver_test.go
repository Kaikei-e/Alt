package alt_db

import (
	"alt/utils/logger"
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// Initialize logger for tests
	var buf bytes.Buffer
	testLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Logger = testLogger
}

func TestAltDBRepository_FetchRecentArticles_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	ctx := context.Background()
	since := time.Now().Add(-24 * time.Hour)
	limit := 10

	articleID := uuid.New()
	feedID := uuid.New()
	publishedAt := time.Now().Add(-2 * time.Hour)
	tags := []string{"tech", "news"}

	// Mock article query with tags
	articleRows := pgxmock.NewRows([]string{
		"id", "feed_id", "title", "url", "content", "published_at", "created_at", "tags",
	}).AddRow(
		articleID, feedID, "Test Article", "https://example.com/article", "Article content", publishedAt, publishedAt, tags,
	)

	mock.ExpectQuery("SELECT").
		WithArgs(pgxmock.AnyArg(), limit).
		WillReturnRows(articleRows)

	articles, err := repo.FetchRecentArticles(ctx, since, limit)
	require.NoError(t, err)
	assert.Len(t, articles, 1)
	assert.Equal(t, articleID, articles[0].ID)
	assert.Equal(t, "Test Article", articles[0].Title)
	assert.Equal(t, "https://example.com/article", articles[0].URL)
	assert.Equal(t, feedID, articles[0].FeedID)
	assert.Contains(t, articles[0].Tags, "tech")
	assert.Contains(t, articles[0].Tags, "news")

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_FetchRecentArticles_MultipleArticles(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	ctx := context.Background()
	since := time.Now().Add(-24 * time.Hour)
	limit := 10

	article1ID := uuid.New()
	article2ID := uuid.New()
	feedID := uuid.New()
	time1 := time.Now().Add(-1 * time.Hour)
	time2 := time.Now().Add(-2 * time.Hour)

	articleRows := pgxmock.NewRows([]string{
		"id", "feed_id", "title", "url", "content", "published_at", "created_at", "tags",
	}).
		AddRow(article1ID, feedID, "Article 1", "https://example.com/1", "Content 1", time1, time1, []string{"tech"}).
		AddRow(article2ID, feedID, "Article 2", "https://example.com/2", "Content 2", time2, time2, []string{})

	mock.ExpectQuery("SELECT").
		WithArgs(pgxmock.AnyArg(), limit).
		WillReturnRows(articleRows)

	articles, err := repo.FetchRecentArticles(ctx, since, limit)
	require.NoError(t, err)
	assert.Len(t, articles, 2)
	assert.Equal(t, "Article 1", articles[0].Title)
	assert.Equal(t, "Article 2", articles[1].Title)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_FetchRecentArticles_EmptyResult(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	ctx := context.Background()
	since := time.Now().Add(-24 * time.Hour)
	limit := 10

	articleRows := pgxmock.NewRows([]string{
		"id", "feed_id", "title", "url", "content", "published_at", "created_at", "tags",
	})

	mock.ExpectQuery("SELECT").
		WithArgs(pgxmock.AnyArg(), limit).
		WillReturnRows(articleRows)

	articles, err := repo.FetchRecentArticles(ctx, since, limit)
	require.NoError(t, err)
	assert.Empty(t, articles)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_FetchRecentArticles_NilRepository(t *testing.T) {
	ctx := context.Background()
	since := time.Now().Add(-24 * time.Hour)
	limit := 10

	var repo *AltDBRepository
	_, err := repo.FetchRecentArticles(ctx, since, limit)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection not available")
}

func TestAltDBRepository_FetchRecentArticles_NilPool(t *testing.T) {
	ctx := context.Background()
	since := time.Now().Add(-24 * time.Hour)
	limit := 10

	repo := &AltDBRepository{}
	_, err := repo.FetchRecentArticles(ctx, since, limit)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection not available")
}

func TestAltDBRepository_FetchRecentArticles_LimitBounds(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	ctx := context.Background()
	since := time.Now().Add(-24 * time.Hour)

	t.Run("negative limit fetches all articles (no limit)", func(t *testing.T) {
		articleRows := pgxmock.NewRows([]string{
			"id", "feed_id", "title", "url", "content", "published_at", "created_at", "tags",
		})

		// When limit <= 0, no LIMIT clause is applied (only time constraint)
		mock.ExpectQuery("SELECT").
			WithArgs(pgxmock.AnyArg()).
			WillReturnRows(articleRows)

		_, err := repo.FetchRecentArticles(ctx, since, -1)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("zero limit fetches all articles (no limit)", func(t *testing.T) {
		articleRows := pgxmock.NewRows([]string{
			"id", "feed_id", "title", "url", "content", "published_at", "created_at", "tags",
		})

		// When limit <= 0, no LIMIT clause is applied (only time constraint)
		mock.ExpectQuery("SELECT").
			WithArgs(pgxmock.AnyArg()).
			WillReturnRows(articleRows)

		_, err := repo.FetchRecentArticles(ctx, since, 0)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("positive limit applies LIMIT clause", func(t *testing.T) {
		articleRows := pgxmock.NewRows([]string{
			"id", "feed_id", "title", "url", "content", "published_at", "created_at", "tags",
		})

		mock.ExpectQuery("SELECT").
			WithArgs(pgxmock.AnyArg(), 50).
			WillReturnRows(articleRows)

		_, err := repo.FetchRecentArticles(ctx, since, 50)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestAltDBRepository_FetchRecentArticles_QueryError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	ctx := context.Background()
	since := time.Now().Add(-24 * time.Hour)
	limit := 10

	mock.ExpectQuery("SELECT").
		WithArgs(pgxmock.AnyArg(), limit).
		WillReturnError(pgx.ErrNoRows)

	_, err = repo.FetchRecentArticles(ctx, since, limit)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error fetching recent articles")

	require.NoError(t, mock.ExpectationsWereMet())
}
