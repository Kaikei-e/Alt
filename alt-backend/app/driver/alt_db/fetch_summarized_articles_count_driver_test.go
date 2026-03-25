package alt_db

import (
	"context"
	"errors"
	"regexp"
	"testing"

	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
)

func TestAltDBRepository_FetchSummarizedArticlesCount_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &DashboardRepository{pool: mock}
	ctx := authContext()

	expectedQuery := `SELECT COUNT(*) FROM article_summaries WHERE user_id = $1`

	mock.ExpectQuery(regexp.QuoteMeta(expectedQuery)).
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(9))

	count, err := repo.FetchSummarizedArticlesCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 9, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_FetchSummarizedArticlesCount_RequiresAuth(t *testing.T) {
	repo := &DashboardRepository{}
	count, err := repo.FetchSummarizedArticlesCount(context.Background())
	require.Error(t, err)
	require.Equal(t, 0, count)
}

func TestAltDBRepository_FetchSummarizedArticlesCount_QueryError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &DashboardRepository{pool: mock}
	ctx := authContext()

	expectedQuery := `SELECT COUNT(*) FROM article_summaries WHERE user_id = $1`

	mock.ExpectQuery(regexp.QuoteMeta(expectedQuery)).
		WithArgs(pgxmock.AnyArg()).
		WillReturnError(errors.New("db failed"))

	count, err := repo.FetchSummarizedArticlesCount(ctx)
	require.Error(t, err)
	require.Equal(t, 0, count)
	require.ErrorContains(t, err, "failed to fetch summarized articles count")
	require.NoError(t, mock.ExpectationsWereMet())
}
