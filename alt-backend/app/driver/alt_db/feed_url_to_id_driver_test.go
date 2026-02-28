package alt_db

import (
	"context"
	"testing"

	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetFeedIDByURL_UsesFeedLinksTable(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	// The query should JOIN feed_links → feeds to return feeds.id (not feed_links.id)
	mock.ExpectQuery(`SELECT f\.id FROM feeds f INNER JOIN feed_links fl ON f\.feed_link_id = fl\.id WHERE fl\.url = \$1`).
		WithArgs("https://www.theguardian.com/world/rss").
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("feed-uuid-1"))

	feedID, err := repo.GetFeedIDByURL(context.Background(), "https://www.theguardian.com/world/rss")
	require.NoError(t, err)
	assert.Equal(t, "feed-uuid-1", feedID)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFeedIDByURL_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	mock.ExpectQuery(`SELECT f\.id FROM feeds f INNER JOIN feed_links fl ON f\.feed_link_id = fl\.id WHERE fl\.url = \$1`).
		WithArgs("https://nonexistent.com/rss").
		WillReturnRows(pgxmock.NewRows([]string{"id"}))

	_, err = repo.GetFeedIDByURL(context.Background(), "https://nonexistent.com/rss")
	require.Error(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFeedIDByURL_NilPool(t *testing.T) {
	repo := &AltDBRepository{pool: nil}

	_, err := repo.GetFeedIDByURL(context.Background(), "https://example.com/rss")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection not available")
}

func TestGetFeedIDByArticleURL_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	// The query should look up feeds.id by feeds.link (article URL, not RSS URL)
	mock.ExpectQuery(`SELECT id FROM feeds WHERE link = \$1`).
		WithArgs("https://dev.to/some-article").
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("feed-uuid-1"))

	feedID, err := repo.GetFeedIDByArticleURL(context.Background(), "https://dev.to/some-article")
	require.NoError(t, err)
	assert.Equal(t, "feed-uuid-1", feedID)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFeedIDByArticleURL_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	mock.ExpectQuery(`SELECT id FROM feeds WHERE link = \$1`).
		WithArgs("https://nonexistent.com/article").
		WillReturnRows(pgxmock.NewRows([]string{"id"}))

	_, err = repo.GetFeedIDByArticleURL(context.Background(), "https://nonexistent.com/article")
	require.Error(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFeedIDByArticleURL_NilPool(t *testing.T) {
	repo := &AltDBRepository{pool: nil}

	_, err := repo.GetFeedIDByArticleURL(context.Background(), "https://example.com/article")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection not available")
}
