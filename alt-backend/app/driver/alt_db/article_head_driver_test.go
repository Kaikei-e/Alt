package alt_db

import (
	"context"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v5"
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
