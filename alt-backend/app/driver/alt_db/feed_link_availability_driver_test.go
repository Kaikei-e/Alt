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

	repo := &FeedRepository{pool: mock}

	// The query should use UPSERT to create row if missing, and handle duplicate feed_links entries
	mock.ExpectExec(regexp.QuoteMeta(
		`INSERT INTO feed_link_availability (feed_link_id, is_active, consecutive_failures)
		SELECT id, true, 0 FROM feed_links WHERE url = $1
		ON CONFLICT (feed_link_id) DO UPDATE SET
			is_active = true,
			consecutive_failures = 0,
			last_failure_at = NULL,
			last_failure_reason = NULL`)).
		WithArgs("https://hackernoon.com/feed").
		WillReturnResult(pgxmock.NewResult("INSERT", 2))

	err = repo.ResetFeedLinkFailures(context.Background(), "https://hackernoon.com/feed")
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResetFeedLinkFailures_CreatesRowIfMissing(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}

	// When no feed_link_availability row exists, the UPSERT should INSERT a new row
	mock.ExpectExec(regexp.QuoteMeta(
		`INSERT INTO feed_link_availability (feed_link_id, is_active, consecutive_failures)
		SELECT id, true, 0 FROM feed_links WHERE url = $1
		ON CONFLICT (feed_link_id) DO UPDATE SET
			is_active = true,
			consecutive_failures = 0,
			last_failure_at = NULL,
			last_failure_reason = NULL`)).
		WithArgs("https://zenn.dev/topics/database/feed").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.ResetFeedLinkFailures(context.Background(), "https://zenn.dev/topics/database/feed")
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDisableFeedLink_DuplicateURLs(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}

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
