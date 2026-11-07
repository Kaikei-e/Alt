package alt_db

import (
	"alt/domain"
	"context"
	"errors"
	"regexp"
	"testing"

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
