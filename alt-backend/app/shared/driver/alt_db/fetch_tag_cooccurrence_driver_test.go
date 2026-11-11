package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagRepository_FetchTagCooccurrences_EmptyInputSkipsQuery(t *testing.T) {
	logger.InitLogger()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := NewTagRepository(mock)

	items, err := repo.FetchTagCooccurrences(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, items)

	items, err = repo.FetchTagCooccurrences(context.Background(), []string{})
	require.NoError(t, err)
	assert.Nil(t, items)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTagRepository_FetchTagCooccurrences_ScanAndRowsErrors(t *testing.T) {
	logger.InitLogger()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := NewTagRepository(mock)
	tagNames := []string{"rust", "tokio"}

	mock.ExpectQuery("SELECT").
		WithArgs(tagNames).
		WillReturnRows(pgxmock.NewRows([]string{"tag_a", "tag_b", "shared_count"}).
			AddRow("rust", "tokio", "not-an-int"))

	items, err := repo.FetchTagCooccurrences(context.Background(), tagNames)
	require.Error(t, err)
	assert.Nil(t, items)

	mock.ExpectQuery("SELECT").
		WithArgs(tagNames).
		WillReturnRows(pgxmock.NewRows([]string{"tag_a", "tag_b", "shared_count"}).
			AddRow("rust", "tokio", 5).
			RowError(0, errors.New("stream interrupted")))

	items, err = repo.FetchTagCooccurrences(context.Background(), tagNames)
	require.Error(t, err)
	assert.Nil(t, items)

	require.NoError(t, mock.ExpectationsWereMet())
}
