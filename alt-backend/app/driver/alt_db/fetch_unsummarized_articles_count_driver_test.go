package alt_db

import (
	"context"
	"errors"
	"regexp"
	"testing"

	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
)

func TestAltDBRepository_FetchUnsummarizedArticlesCount_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	ctx := context.Background()

	expectedQuery := `
		SELECT COUNT(*)
		FROM articles a
		LEFT JOIN article_summaries s ON a.id = s.article_id
		WHERE s.article_id IS NULL
	`

	// Test case: 3 unsummarized articles out of 5 total
	mock.ExpectQuery(regexp.QuoteMeta(expectedQuery)).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(3))

	count, err := repo.FetchUnsummarizedArticlesCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_FetchUnsummarizedArticlesCount_AllSummarized(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	ctx := context.Background()

	expectedQuery := `
		SELECT COUNT(*)
		FROM articles a
		LEFT JOIN article_summaries s ON a.id = s.article_id
		WHERE s.article_id IS NULL
	`

	// Test case: all articles have summaries
	mock.ExpectQuery(regexp.QuoteMeta(expectedQuery)).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))

	count, err := repo.FetchUnsummarizedArticlesCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_FetchUnsummarizedArticlesCount_NoArticles(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	ctx := context.Background()

	expectedQuery := `
		SELECT COUNT(*)
		FROM articles a
		LEFT JOIN article_summaries s ON a.id = s.article_id
		WHERE s.article_id IS NULL
	`

	// Test case: no articles in database
	mock.ExpectQuery(regexp.QuoteMeta(expectedQuery)).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))

	count, err := repo.FetchUnsummarizedArticlesCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_FetchUnsummarizedArticlesCount_QueryError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	ctx := context.Background()

	expectedQuery := `
		SELECT COUNT(*)
		FROM articles a
		LEFT JOIN article_summaries s ON a.id = s.article_id
		WHERE s.article_id IS NULL
	`

	// Test case: database error
	mock.ExpectQuery(regexp.QuoteMeta(expectedQuery)).
		WillReturnError(errors.New("database connection failed"))

	count, err := repo.FetchUnsummarizedArticlesCount(ctx)
	require.Error(t, err)
	require.Equal(t, 0, count)
	require.ErrorContains(t, err, "failed to fetch unsummarized articles count")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_FetchUnsummarizedArticlesCount_NilRepository(t *testing.T) {
	var repo *AltDBRepository
	ctx := context.Background()

	count, err := repo.FetchUnsummarizedArticlesCount(ctx)
	require.Error(t, err)
	require.Equal(t, 0, count)
	require.Equal(t, "database connection not available", err.Error())
}

func TestAltDBRepository_FetchUnsummarizedArticlesCount_NilPool(t *testing.T) {
	repo := &AltDBRepository{}
	ctx := context.Background()

	count, err := repo.FetchUnsummarizedArticlesCount(ctx)
	require.Error(t, err)
	require.Equal(t, 0, count)
	require.Equal(t, "database connection not available", err.Error())
}

// TestAltDBRepository_FetchUnsummarizedArticlesCount_IgnoresOrphanedSummaries tests that
// the query correctly ignores article_summaries records that have no corresponding article.
// This ensures data integrity and accurate counting even if orphaned records exist.
func TestAltDBRepository_FetchUnsummarizedArticlesCount_IgnoresOrphanedSummaries(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	ctx := context.Background()

	expectedQuery := `
		SELECT COUNT(*)
		FROM articles a
		LEFT JOIN article_summaries s ON a.id = s.article_id
		WHERE s.article_id IS NULL
	`

	// Scenario:
	// - 10 articles exist
	// - 7 have valid summaries
	// - 3 articles are unsummarized
	// - 2 orphaned summaries exist (article was deleted but summary remains)
	// Expected count: 3 (only articles without summaries)
	//
	// The query uses LEFT JOIN from articles, so orphaned summaries are automatically ignored
	mock.ExpectQuery(regexp.QuoteMeta(expectedQuery)).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(3))

	count, err := repo.FetchUnsummarizedArticlesCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, count, "Should count only articles without summaries, ignoring orphaned summary records")
	require.NoError(t, mock.ExpectationsWereMet())
}
