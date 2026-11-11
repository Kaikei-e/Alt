package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagRepository_FetchTagCooccurrences_RanksByOchiaiWithDeterministicTiebreak(t *testing.T) {
	logger.InitLogger()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := NewTagRepository(mock)
	tagNames := []string{"rust", "tokio", "hub"}

	rows := pgxmock.NewRows([]string{"tag_a", "tag_b", "shared_count"}).
		AddRow("rust", "tokio", 18).
		AddRow("hub", "rust", 42)

	expectedOrder := regexp.QuoteMeta(
		"ORDER BY pc.shared_count::float8 / SQRT(tc1.article_count::float8 * tc2.article_count::float8) DESC,",
	) + `\s+` + regexp.QuoteMeta("pc.shared_count DESC,") +
		`\s+` + regexp.QuoteMeta("pc.tag_a ASC,") +
		`\s+` + regexp.QuoteMeta("pc.tag_b ASC") +
		`\s+` + regexp.QuoteMeta("LIMIT 2000")

	expectedPattern := `(?s)` +
		regexp.QuoteMeta("WHERE tag_name = ANY($1)") + `.*` +
		regexp.QuoteMeta("COUNT(DISTINCT at.article_id) AS article_count") + `.*` +
		regexp.QuoteMeta("GROUP BY tt.tag_name") + `.*` +
		regexp.QuoteMeta("tt1.tag_name < tt2.tag_name") + `.*` +
		regexp.QuoteMeta("HAVING COUNT(DISTINCT at1.article_id) >= 2") + `.*` +
		expectedOrder

	mock.ExpectQuery(expectedPattern).
		WithArgs(tagNames).
		WillReturnRows(rows)

	items, err := repo.FetchTagCooccurrences(context.Background(), tagNames)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "rust", items[0].TagNameA)
	assert.Equal(t, "tokio", items[0].TagNameB)
	assert.Equal(t, 18, items[0].SharedCount)
	assert.Equal(t, "hub", items[1].TagNameA)
	assert.Equal(t, "rust", items[1].TagNameB)
	assert.Equal(t, 42, items[1].SharedCount)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTagRepository_FetchTagCooccurrences_NoRawSharedCountPrimaryOrder(t *testing.T) {
	logger.InitLogger()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := NewTagRepository(mock)

	mock.ExpectQuery(`(?s)ORDER BY shared_count DESC\s+LIMIT 2000`).
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"tag_a", "tag_b", "shared_count"}))

	_, _ = repo.FetchTagCooccurrences(context.Background(), []string{"rust", "tokio"})

	err = mock.ExpectationsWereMet()
	assert.Error(t, err, "driver must rank 2,000-pair cutoff by Ochiai association, not raw shared_count DESC")
}

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
