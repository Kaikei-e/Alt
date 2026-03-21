package alt_db

import (
	"context"
	"errors"
	"regexp"
	"testing"

	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
)

func TestAltDBRepository_FetchTotalArticlesCount_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	ctx := authContext()

	expectedQuery := `SELECT COUNT(*) FROM articles WHERE user_id = $1 AND deleted_at IS NULL`

	mock.ExpectQuery(regexp.QuoteMeta(expectedQuery)).
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(12))

	count, err := repo.FetchTotalArticlesCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 12, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_FetchTotalArticlesCount_RequiresAuth(t *testing.T) {
	repo := &AltDBRepository{}
	count, err := repo.FetchTotalArticlesCount(context.Background())
	require.Error(t, err)
	require.Equal(t, 0, count)
}

func TestAltDBRepository_FetchTotalArticlesCount_QueryError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	ctx := authContext()

	expectedQuery := `SELECT COUNT(*) FROM articles WHERE user_id = $1 AND deleted_at IS NULL`

	mock.ExpectQuery(regexp.QuoteMeta(expectedQuery)).
		WithArgs(pgxmock.AnyArg()).
		WillReturnError(errors.New("db failed"))

	count, err := repo.FetchTotalArticlesCount(ctx)
	require.Error(t, err)
	require.Equal(t, 0, count)
	require.ErrorContains(t, err, "failed to fetch total articles count")
	require.NoError(t, mock.ExpectationsWereMet())
}
