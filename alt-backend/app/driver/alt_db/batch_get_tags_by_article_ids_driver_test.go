package alt_db

import (
	"context"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchGetTagsByArticleIDs_EmptyInputShortCircuits(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}

	rows, err := repo.BatchGetTagsByArticleIDs(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, rows)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchGetTagsByArticleIDs_ReturnsRows(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}

	updatedAt := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT").
		WithArgs([]string{"a1", "a2"}).
		WillReturnRows(pgxmock.NewRows([]string{"article_id", "tag_name", "confidence", "updated_at"}).
			AddRow("a1", "go", float32(0.9), updatedAt).
			AddRow("a1", "rust", float32(0.7), updatedAt).
			AddRow("a2", "python", float32(0.6), updatedAt))

	rows, err := repo.BatchGetTagsByArticleIDs(context.Background(), []string{"a1", "a2"})
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, "a1", rows[0].ArticleID)
	assert.Equal(t, "go", rows[0].TagName)
	assert.InDelta(t, 0.9, rows[0].Confidence, 1e-6)
	assert.Equal(t, updatedAt, rows[0].UpdatedAt)
	assert.Equal(t, "a2", rows[2].ArticleID)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchGetTagsByArticleIDs_NilPoolErrors(t *testing.T) {
	repo := &TagRepository{pool: nil}
	_, err := repo.BatchGetTagsByArticleIDs(context.Background(), []string{"a1"})
	require.Error(t, err)
}
