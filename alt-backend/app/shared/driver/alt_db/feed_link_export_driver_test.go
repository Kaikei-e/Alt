package alt_db

import (
	"context"
	"errors"
	"regexp"
	"testing"

	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchFeedLinksForExport_ReturnsLinksWithNewestTitle(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}

	mock.ExpectQuery(regexp.QuoteMeta(feedLinksForExportQuery)).
		WillReturnRows(pgxmock.NewRows([]string{"url", "title"}).
			AddRow("https://a.example.com/feed.xml", "A Blog").
			AddRow("https://b.example.com/feed.xml", ""))

	links, err := repo.FetchFeedLinksForExport(context.Background())

	require.NoError(t, err)
	require.Len(t, links, 2)
	assert.Equal(t, "A Blog", links[0].Title)
	assert.Empty(t, links[1].Title,
		"a link with no collected feed keeps an empty title; the hostname fallback is the renderer's choice, not the driver's")
	require.NoError(t, mock.ExpectationsWereMet())
}

// No subscriptions is an empty slice, not nil. The OPML writer ranges over the
// result and emits a valid empty document; a nil would work today but makes
// "no subscriptions" and "the query failed" look alike at a glance.
func TestFetchFeedLinksForExport_EmptyIsNotNil(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}

	mock.ExpectQuery(regexp.QuoteMeta(feedLinksForExportQuery)).
		WillReturnRows(pgxmock.NewRows([]string{"url", "title"}))

	links, err := repo.FetchFeedLinksForExport(context.Background())

	require.NoError(t, err)
	require.NotNil(t, links)
	assert.Empty(t, links)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchFeedLinksForExport_QueryFailure(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}

	mock.ExpectQuery(regexp.QuoteMeta(feedLinksForExportQuery)).
		WillReturnError(errors.New("connection reset"))

	links, err := repo.FetchFeedLinksForExport(context.Background())

	require.Error(t, err)
	assert.Nil(t, links)
	require.NoError(t, mock.ExpectationsWereMet())
}
