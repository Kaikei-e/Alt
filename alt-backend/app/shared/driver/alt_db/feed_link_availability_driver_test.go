package alt_db

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetFeedLinkFailures_DuplicateURLs(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}

	// The query should use UPSERT to create row if missing, and handle duplicate feed_links entries
	mock.ExpectExec(regexp.QuoteMeta(resetFeedLinkFailuresQuery)).
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
	mock.ExpectExec(regexp.QuoteMeta(resetFeedLinkFailuresQuery)).
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
	mock.ExpectExec(regexp.QuoteMeta(disableFeedLinkQuery)).
		WithArgs("https://hackernoon.com/feed").
		WillReturnResult(pgxmock.NewResult("UPDATE", 2))

	err = repo.DisableFeedLink(context.Background(), "https://hackernoon.com/feed")
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}

// RecordFeedLinkFailure replaces the IncrementFeedLinkFailures → ShouldDisable
// → DisableFeedLink sequence the collector used to run itself (capability
// catalog §4-4).
//
// The point of the merge is the transaction, so these tests pin the boundary —
// Begin before the increment, Commit after the disable — and not only the
// returned values. A version that issued the same two statements outside a
// transaction would satisfy every assertion about the values alone.

func availabilityRows(feedLinkID uuid.UUID, isActive bool, failures int) *pgxmock.Rows {
	return pgxmock.NewRows([]string{"feed_link_id", "is_active", "consecutive_failures", "last_failure_at", "last_failure_reason"}).
		AddRow(feedLinkID, isActive, failures, (*time.Time)(nil), (*string)(nil))
}

func TestRecordFeedLinkFailure_IncrementsBelowThresholdWithoutDisabling(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	feedLinkID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	failedAt := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	reason := "403 Forbidden"

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(incrementFeedLinkFailuresQuery)).
		WithArgs("https://example.com/feed.xml", "403 Forbidden").
		WillReturnRows(pgxmock.NewRows([]string{"feed_link_id", "is_active", "consecutive_failures", "last_failure_at", "last_failure_reason"}).
			AddRow(feedLinkID, true, 3, &failedAt, &reason))
	mock.ExpectCommit()

	availability, disabledNow, err := repo.RecordFeedLinkFailure(context.Background(),
		"https://example.com/feed.xml", "403 Forbidden", 5)

	require.NoError(t, err)
	require.NotNil(t, availability)
	assert.Equal(t, 3, availability.ConsecutiveFailures)
	assert.True(t, availability.IsActive)
	assert.False(t, disabledNow, "3 of 5 failures must not disable the link")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Reaching the threshold disables inside the same transaction as the
// increment, and the returned row reflects the disable rather than the
// pre-disable snapshot the RETURNING clause saw.
func TestRecordFeedLinkFailure_DisablesAtThresholdInSameTransaction(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	feedLinkID := uuid.MustParse("22222222-2222-4222-8222-222222222222")

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(incrementFeedLinkFailuresQuery)).
		WithArgs("https://dead.example.com/feed.xml", "404 Not Found").
		WillReturnRows(availabilityRows(feedLinkID, true, 5))
	mock.ExpectExec(regexp.QuoteMeta(disableFeedLinkQuery)).
		WithArgs("https://dead.example.com/feed.xml").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()

	availability, disabledNow, err := repo.RecordFeedLinkFailure(context.Background(),
		"https://dead.example.com/feed.xml", "404 Not Found", 5)

	require.NoError(t, err)
	require.NotNil(t, availability)
	assert.True(t, disabledNow)
	assert.False(t, availability.IsActive,
		"the response must describe the row after the disable, not the RETURNING snapshot before it")
	require.NoError(t, mock.ExpectationsWereMet())
}

// A link that is already inactive keeps counting failures but reports
// disabledNow = false, and issues no second UPDATE. The caller logs the
// transition; reporting the state instead would re-raise the same alert on
// every subsequent poll of a feed that has been dead for weeks.
func TestRecordFeedLinkFailure_AlreadyDisabledReportsNoTransition(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	feedLinkID := uuid.MustParse("33333333-3333-4333-8333-333333333333")

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(incrementFeedLinkFailuresQuery)).
		WithArgs("https://dead.example.com/feed.xml", "404 Not Found").
		WillReturnRows(availabilityRows(feedLinkID, false, 9))
	mock.ExpectCommit()

	availability, disabledNow, err := repo.RecordFeedLinkFailure(context.Background(),
		"https://dead.example.com/feed.xml", "404 Not Found", 5)

	require.NoError(t, err)
	assert.False(t, disabledNow)
	assert.False(t, availability.IsActive)
	require.NoError(t, mock.ExpectationsWereMet())
}

// A non-positive threshold means "never disable". It is an explicit value
// rather than an unset one, so a caller that wants counting without
// auto-disable says so and a caller that forgot the argument gets counting —
// not a feed switched off on its first failure.
func TestRecordFeedLinkFailure_NonPositiveThresholdNeverDisables(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	feedLinkID := uuid.MustParse("44444444-4444-4444-8444-444444444444")

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(incrementFeedLinkFailuresQuery)).
		WithArgs("https://example.com/feed.xml", "boom").
		WillReturnRows(availabilityRows(feedLinkID, true, 99))
	mock.ExpectCommit()

	_, disabledNow, err := repo.RecordFeedLinkFailure(context.Background(),
		"https://example.com/feed.xml", "boom", 0)

	require.NoError(t, err)
	assert.False(t, disabledNow)
	require.NoError(t, mock.ExpectationsWereMet())
}

// A failed disable rolls the increment back. Committing the increment and
// dropping the disable would leave a link one failure past its threshold that
// no later call can disable either — every subsequent poll would also be past
// the threshold and take the same failing branch.
func TestRecordFeedLinkFailure_RollsBackWhenDisableFails(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	feedLinkID := uuid.MustParse("55555555-5555-4555-8555-555555555555")

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(incrementFeedLinkFailuresQuery)).
		WithArgs("https://dead.example.com/feed.xml", "404").
		WillReturnRows(availabilityRows(feedLinkID, true, 5))
	mock.ExpectExec(regexp.QuoteMeta(disableFeedLinkQuery)).
		WithArgs("https://dead.example.com/feed.xml").
		WillReturnError(errors.New("deadlock detected"))
	mock.ExpectRollback()

	_, _, err = repo.RecordFeedLinkFailure(context.Background(),
		"https://dead.example.com/feed.xml", "404", 5)

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// An unsubscribed URL matches no feed_links row, so the INSERT ... SELECT
// inserts nothing and RETURNING yields none. That surfaces as an error rather
// than as a zero-failure availability: a caller told "0 failures, active" for
// a URL nobody subscribes to would keep polling it forever.
func TestRecordFeedLinkFailure_UnknownURLIsAnError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(incrementFeedLinkFailuresQuery)).
		WithArgs("https://unknown.example.com/feed.xml", "boom").
		WillReturnError(pgx.ErrNoRows)
	mock.ExpectRollback()

	_, _, err = repo.RecordFeedLinkFailure(context.Background(),
		"https://unknown.example.com/feed.xml", "boom", 5)

	require.Error(t, err)
	assert.ErrorIs(t, err, pgx.ErrNoRows,
		"the caller distinguishes an unknown URL from a database fault, so the sentinel must survive the wrap")
	require.NoError(t, mock.ExpectationsWereMet())
}
