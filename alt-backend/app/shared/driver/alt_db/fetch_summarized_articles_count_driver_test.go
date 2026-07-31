package alt_db

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

func TestAltDBRepository_FetchSummarizedArticlesCount_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &DashboardRepository{pool: mock}
	ctx := context.Background()

	expectedQuery := `SELECT COUNT(*) FROM article_summaries WHERE user_id = $1`

	mock.ExpectQuery(regexp.QuoteMeta(expectedQuery)).
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(9))

	count, err := repo.FetchSummarizedArticlesCountForUser(ctx, statsTestUserID)
	require.NoError(t, err)
	require.Equal(t, 9, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAltDBRepository_FetchSummarizedArticlesCount_RequiresAuth pins the refusal that replaced the
// missing-user-context one.
//
// The pool is real and the owner is the zero UUID, so the only thing that can
// stop the query is the guard. That distinction matters: user_id is the sole
// tenant predicate on the article_summaries count, so an unguarded zero owner would run,
// return 0, and be indistinguishable from an empty account.
func TestAltDBRepository_FetchSummarizedArticlesCount_RequiresAuth(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &DashboardRepository{pool: mock}

	count, err := repo.FetchSummarizedArticlesCountForUser(context.Background(), uuid.Nil)
	require.Error(t, err)
	require.ErrorContains(t, err, "authentication required")
	require.Equal(t, 0, count)
	// No query was issued at all.
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_FetchSummarizedArticlesCount_QueryError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &DashboardRepository{pool: mock}
	ctx := context.Background()

	expectedQuery := `SELECT COUNT(*) FROM article_summaries WHERE user_id = $1`

	mock.ExpectQuery(regexp.QuoteMeta(expectedQuery)).
		WithArgs(pgxmock.AnyArg()).
		WillReturnError(errors.New("db failed"))

	count, err := repo.FetchSummarizedArticlesCountForUser(ctx, statsTestUserID)
	require.Error(t, err)
	require.Equal(t, 0, count)
	require.ErrorContains(t, err, "failed to fetch summarized articles count")
	require.NoError(t, mock.ExpectationsWereMet())
}
