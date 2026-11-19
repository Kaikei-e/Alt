package alt_db

import (
	"alt/domain"
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInsertSubscription_ForeignKeyViolationReturnsErrFeedNotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &SubscriptionRepository{pool: mock}
	userID := uuid.New()
	feedLinkID := uuid.New()

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO user_feed_subscriptions (user_id, feed_link_id)")).
		WithArgs(userID, feedLinkID).
		WillReturnError(&pgconn.PgError{
			Code:           "23503",
			ConstraintName: "fk_ufs_feed_link_id",
		})

	err = repo.InsertSubscription(context.Background(), userID, feedLinkID)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrFeedNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertSubscription_DatabaseFailureIsNotErrFeedNotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &SubscriptionRepository{pool: mock}
	userID := uuid.New()
	feedLinkID := uuid.New()

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO user_feed_subscriptions (user_id, feed_link_id)")).
		WithArgs(userID, feedLinkID).
		WillReturnError(errors.New("connection refused"))

	err = repo.InsertSubscription(context.Background(), userID, feedLinkID)
	require.Error(t, err)
	assert.NotErrorIs(t, err, domain.ErrFeedNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchSubscriptions_IncludesDailyCap(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &SubscriptionRepository{pool: mock}
	userID := uuid.New()
	capVal := 7
	now := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT fl\.id, fl\.url`).
		WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "url", "is_subscribed", "created_at", "daily_cap"}).
			AddRow("link-1", "https://example.com/a.xml", true, now, &capVal).
			AddRow("link-2", "https://example.com/b.xml", false, now, (*int)(nil)))

	sources, err := repo.FetchSubscriptions(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, sources, 2)
	require.NotNil(t, sources[0].DailyCap)
	assert.Equal(t, 7, *sources[0].DailyCap)
	assert.Nil(t, sources[1].DailyCap)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSetSubscriptionDailyCap(t *testing.T) {
	t.Run("sets positive cap", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := &SubscriptionRepository{pool: mock}
		userID := uuid.New()
		linkID := uuid.New()
		capVal := 5

		mock.ExpectExec(regexp.QuoteMeta("UPDATE user_feed_subscriptions")).
			WithArgs(userID, linkID, &capVal).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		err = repo.SetSubscriptionDailyCap(context.Background(), userID, linkID, &capVal)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("clears cap when nil", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := &SubscriptionRepository{pool: mock}
		userID := uuid.New()
		linkID := uuid.New()

		mock.ExpectExec(regexp.QuoteMeta("UPDATE user_feed_subscriptions")).
			WithArgs(userID, linkID, (*int)(nil)).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		err = repo.SetSubscriptionDailyCap(context.Background(), userID, linkID, nil)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns ErrFeedNotFound when not subscribed", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := &SubscriptionRepository{pool: mock}
		userID := uuid.New()
		linkID := uuid.New()
		capVal := 5

		mock.ExpectExec(regexp.QuoteMeta("UPDATE user_feed_subscriptions")).
			WithArgs(userID, linkID, &capVal).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))

		err = repo.SetSubscriptionDailyCap(context.Background(), userID, linkID, &capVal)
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrFeedNotFound)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rejects non-positive cap before touching db", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := &SubscriptionRepository{pool: mock}
		zero := 0
		err = repo.SetSubscriptionDailyCap(context.Background(), uuid.New(), uuid.New(), &zero)
		require.Error(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
