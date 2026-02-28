package alt_db

import (
	"context"
	"regexp"
	"testing"

	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
)

func TestResetFeedLinkFailures_DuplicateURLs(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	// The query should use IN (not =) to handle duplicate feed_links entries
	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE feed_link_availability SET consecutive_failures = 0
		WHERE feed_link_id IN (SELECT id FROM feed_links WHERE url = $1)`)).
		WithArgs("https://hackernoon.com/feed").
		WillReturnResult(pgxmock.NewResult("UPDATE", 2))

	err = repo.ResetFeedLinkFailures(context.Background(), "https://hackernoon.com/feed")
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDisableFeedLink_DuplicateURLs(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	// The query should use IN (not =) to handle duplicate feed_links entries
	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE feed_link_availability SET is_active = false
		WHERE feed_link_id IN (SELECT id FROM feed_links WHERE url = $1)`)).
		WithArgs("https://hackernoon.com/feed").
		WillReturnResult(pgxmock.NewResult("UPDATE", 2))

	err = repo.DisableFeedLink(context.Background(), "https://hackernoon.com/feed")
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}
