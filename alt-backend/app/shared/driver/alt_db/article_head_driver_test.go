package alt_db

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CleanupExpiredArticleHeads becomes load-bearing for the 7-day OG image
// retention policy, so pin its behaviour: it deletes by created_at and reports
// the number of purged rows.
func TestArticleRepository_CleanupExpiredArticleHeads(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}

	mock.ExpectExec(`(?s)DELETE FROM article_heads.*created_at <`).
		WithArgs(pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("DELETE", 4))

	deleted, err := repo.CleanupExpiredArticleHeads(context.Background(), 7*24*time.Hour)
	require.NoError(t, err)
	require.EqualValues(t, 4, deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_FetchOgImageURLsByArticleIDs_ScanErrorReturnsError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}
	articleIDs := []string{"art-1", "art-2"}

	rows := pgxmock.NewRows([]string{"article_id", "coalesce"}).
		AddRow("art-1", "https://example.com/1.png").
		AddRow("art-2", nil)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT article_id, COALESCE(og_image_url, '') FROM article_heads WHERE article_id = ANY($1)`)).
		WithArgs(articleIDs).
		WillReturnRows(rows)

	urls, err := repo.FetchOgImageURLsByArticleIDs(context.Background(), articleIDs)
	require.Error(t, err)
	assert.Nil(t, urls)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_FetchOgImageURLsByArticleIDs_RowsErrorReturnsError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}
	articleIDs := []string{"art-1"}

	rows := pgxmock.NewRows([]string{"article_id", "coalesce"}).
		AddRow("art-1", "https://example.com/1.png").
		RowError(0, errors.New("cursor closed"))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT article_id, COALESCE(og_image_url, '') FROM article_heads WHERE article_id = ANY($1)`)).
		WithArgs(articleIDs).
		WillReturnRows(rows)

	urls, err := repo.FetchOgImageURLsByArticleIDs(context.Background(), articleIDs)
	require.Error(t, err)
	assert.Nil(t, urls)
	require.NoError(t, mock.ExpectationsWereMet())
}
