package alt_db

import (
	"context"
	"testing"

	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListUntaggedArticles_NullFeedID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	// Count query
	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int32(2)))

	// Article with NULL feed_id should return nil, not empty string
	feedID1 := "feed-uuid-1"
	rows := pgxmock.NewRows([]string{"id", "title", "content", "user_id", "feed_id"}).
		AddRow("art-1", "Title 1", "Content 1", "user-1", &feedID1).
		AddRow("art-2", "Title 2", "Content 2", "user-2", nil)

	mock.ExpectQuery("SELECT a.id").
		WithArgs(10, 0).
		WillReturnRows(rows)

	articles, count, err := repo.ListUntaggedArticles(context.Background(), 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int32(2), count)
	require.Len(t, articles, 2)

	// Article with feed_id should have it set
	assert.NotNil(t, articles[0].FeedID)
	assert.Equal(t, "feed-uuid-1", *articles[0].FeedID)

	// Article with NULL feed_id should have nil FeedID
	assert.Nil(t, articles[1].FeedID)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListUntaggedArticles_AllWithFeedIDs(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int32(1)))

	feedID1 := "feed-uuid-1"
	rows := pgxmock.NewRows([]string{"id", "title", "content", "user_id", "feed_id"}).
		AddRow("art-1", "Title 1", "Content 1", "user-1", &feedID1)

	mock.ExpectQuery("SELECT a.id").
		WithArgs(10, 0).
		WillReturnRows(rows)

	articles, count, err := repo.ListUntaggedArticles(context.Background(), 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int32(1), count)
	require.Len(t, articles, 1)
	assert.NotNil(t, articles[0].FeedID)
	assert.Equal(t, "feed-uuid-1", *articles[0].FeedID)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListUntaggedArticles_Empty(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int32(0)))

	mock.ExpectQuery("SELECT a.id").
		WithArgs(10, 0).
		WillReturnRows(pgxmock.NewRows([]string{"id", "title", "content", "user_id", "feed_id"}))

	articles, count, err := repo.ListUntaggedArticles(context.Background(), 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int32(0), count)
	assert.Empty(t, articles)

	require.NoError(t, mock.ExpectationsWereMet())
}
