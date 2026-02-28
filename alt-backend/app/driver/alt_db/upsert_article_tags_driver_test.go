package alt_db

import (
	"context"
	"testing"

	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchUpsertArticleTags_AllEmptyFeedIDs(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	// All items have empty FeedID — should return 0 without touching DB
	items := []BatchUpsertTagItem{
		{ArticleID: "art-1", FeedID: "", Tags: []TagUpsertItem{{Name: "go", Confidence: 0.9}}},
		{ArticleID: "art-2", FeedID: "", Tags: []TagUpsertItem{{Name: "rust", Confidence: 0.8}}},
	}

	count, err := repo.BatchUpsertArticleTags(context.Background(), items)
	require.NoError(t, err)
	assert.Equal(t, int32(0), count)

	// No DB expectations — the function should short-circuit
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchUpsertArticleTags_EmptyInput(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	count, err := repo.BatchUpsertArticleTags(context.Background(), []BatchUpsertTagItem{})
	require.NoError(t, err)
	assert.Equal(t, int32(0), count)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertArticleTags_EmptyFeedID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	// Empty FeedID should return 0 without touching DB
	count, err := repo.UpsertArticleTags(context.Background(), "art-1", "", []TagUpsertItem{{Name: "go", Confidence: 0.9}})
	require.NoError(t, err)
	assert.Equal(t, int32(0), count)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertArticleTags_EmptyTags(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	count, err := repo.UpsertArticleTags(context.Background(), "art-1", "feed-1", []TagUpsertItem{})
	require.NoError(t, err)
	assert.Equal(t, int32(0), count)

	require.NoError(t, mock.ExpectationsWereMet())
}
