package alt_db

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestFetchTagArticleCounts_ScanErrorReturnsError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}
	userID := uuid.New()
	since := time.Now().Add(-7 * 24 * time.Hour)

	rows := pgxmock.NewRows([]string{"tag_name", "article_count"}).
		AddRow("golang", 5).
		AddRow("rust", "not-an-int")

	mock.ExpectQuery(regexp.QuoteMeta(buildTagArticleCountsQuery())).
		WithArgs(since, userID).
		WillReturnRows(rows)

	counts, err := repo.FetchTagArticleCounts(context.Background(), userID, since)
	require.Error(t, err)
	assert.Nil(t, counts)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchTagArticleCounts_RowsErrorReturnsError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}
	userID := uuid.New()
	since := time.Now().Add(-7 * 24 * time.Hour)

	rows := pgxmock.NewRows([]string{"tag_name", "article_count"}).
		AddRow("golang", 5).
		RowError(0, errors.New("stream reset"))

	mock.ExpectQuery(regexp.QuoteMeta(buildTagArticleCountsQuery())).
		WithArgs(since, userID).
		WillReturnRows(rows)

	counts, err := repo.FetchTagArticleCounts(context.Background(), userID, since)
	require.Error(t, err)
	assert.Nil(t, counts)
	require.NoError(t, mock.ExpectationsWereMet())
}
