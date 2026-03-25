package alt_db

import (
	"context"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListUntaggedArticles_ExcludesNullFeedID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &InternalRepository{pool: mock}

	// Count query should exclude NULL feed_id articles
	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int32(1)))

	// Only articles with non-NULL feed_id should be returned
	feedID1 := "feed-uuid-1"
	now := time.Now().Truncate(time.Microsecond)
	rows := pgxmock.NewRows([]string{"id", "title", "content", "user_id", "feed_id", "created_at"}).
		AddRow("art-1", "Title 1", "Content 1", "user-1", &feedID1, now)

	mock.ExpectQuery("SELECT a.id").
		WithArgs(10).
		WillReturnRows(rows)

	articles, nextCreatedAt, nextID, count, err := repo.ListUntaggedArticles(context.Background(), nil, "", 10)
	require.NoError(t, err)
	// Count should NOT include NULL feed_id articles
	assert.Equal(t, int32(1), count)
	// Only the article with valid feed_id should be returned
	require.Len(t, articles, 1)
	assert.NotNil(t, articles[0].FeedID)
	assert.Equal(t, "feed-uuid-1", *articles[0].FeedID)
	assert.Equal(t, now, articles[0].CreatedAt)
	// Next cursor should point to the last article
	assert.NotNil(t, nextCreatedAt)
	assert.Equal(t, "art-1", nextID)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListUntaggedArticles_KeysetPagination_FirstPage(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &InternalRepository{pool: mock}

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int32(3)))

	feedID := "feed-1"
	t1 := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	rows := pgxmock.NewRows([]string{"id", "title", "content", "user_id", "feed_id", "created_at"}).
		AddRow("art-1", "Title 1", "Content 1", "user-1", &feedID, t1).
		AddRow("art-2", "Title 2", "Content 2", "user-1", &feedID, t2)

	// First page: no cursor, only limit arg
	mock.ExpectQuery("SELECT a.id").
		WithArgs(2).
		WillReturnRows(rows)

	articles, nextCreatedAt, nextID, count, err := repo.ListUntaggedArticles(context.Background(), nil, "", 2)
	require.NoError(t, err)
	assert.Equal(t, int32(3), count)
	require.Len(t, articles, 2)
	assert.Equal(t, "art-1", articles[0].ID)
	assert.Equal(t, "art-2", articles[1].ID)
	// Next cursor should point to the last article
	assert.NotNil(t, nextCreatedAt)
	assert.Equal(t, t2, *nextCreatedAt)
	assert.Equal(t, "art-2", nextID)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListUntaggedArticles_KeysetPagination_NextPage(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &InternalRepository{pool: mock}

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int32(3)))

	feedID := "feed-1"
	t3 := time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)
	rows := pgxmock.NewRows([]string{"id", "title", "content", "user_id", "feed_id", "created_at"}).
		AddRow("art-3", "Title 3", "Content 3", "user-1", &feedID, t3)

	// Next page: cursor args (lastCreatedAt, lastID, limit)
	cursorTime := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT a.id").
		WithArgs(cursorTime, "art-2", 2).
		WillReturnRows(rows)

	articles, nextCreatedAt, nextID, count, err := repo.ListUntaggedArticles(context.Background(), &cursorTime, "art-2", 2)
	require.NoError(t, err)
	assert.Equal(t, int32(3), count)
	require.Len(t, articles, 1)
	assert.Equal(t, "art-3", articles[0].ID)
	assert.NotNil(t, nextCreatedAt)
	assert.Equal(t, t3, *nextCreatedAt)
	assert.Equal(t, "art-3", nextID)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListUntaggedArticles_AllWithFeedIDs(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &InternalRepository{pool: mock}

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int32(1)))

	feedID1 := "feed-uuid-1"
	now := time.Now().Truncate(time.Microsecond)
	rows := pgxmock.NewRows([]string{"id", "title", "content", "user_id", "feed_id", "created_at"}).
		AddRow("art-1", "Title 1", "Content 1", "user-1", &feedID1, now)

	mock.ExpectQuery("SELECT a.id").
		WithArgs(10).
		WillReturnRows(rows)

	articles, _, _, count, err := repo.ListUntaggedArticles(context.Background(), nil, "", 10)
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

	repo := &InternalRepository{pool: mock}

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int32(0)))

	mock.ExpectQuery("SELECT a.id").
		WithArgs(10).
		WillReturnRows(pgxmock.NewRows([]string{"id", "title", "content", "user_id", "feed_id", "created_at"}))

	articles, nextCreatedAt, nextID, count, err := repo.ListUntaggedArticles(context.Background(), nil, "", 10)
	require.NoError(t, err)
	assert.Equal(t, int32(0), count)
	assert.Empty(t, articles)
	assert.Nil(t, nextCreatedAt)
	assert.Empty(t, nextID)

	require.NoError(t, mock.ExpectationsWereMet())
}
