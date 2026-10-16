package alt_db

import (
	"alt/domain"
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
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

func TestFetchFeedLinksForExport_ScopedByUserContext(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	userID := uuid.MustParse("3f2504e0-4f89-11d3-9a0c-0305e82c3301")
	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "alice@example.com",
		TenantID:  userID,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	mock.ExpectQuery(regexp.QuoteMeta(feedLinksForExportByUserQuery)).
		WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"url", "title"}).
			AddRow("https://alice.example.com/feed.xml", "Alice Blog"))

	links, err := repo.FetchFeedLinksForExport(ctx)
	require.NoError(t, err)
	require.Len(t, links, 1)
	assert.Equal(t, "https://alice.example.com/feed.xml", links[0].URL)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteSubscription_DeletesSubscriptionAndOrphanFeedLinkOnly(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &SubscriptionRepository{pool: mock}
	userID := uuid.MustParse("3f2504e0-4f89-11d3-9a0c-0305e82c3301")
	feedLinkID := uuid.MustParse("a1b2c3d4-1111-4111-8111-111111111111")

	mock.ExpectExec(regexp.QuoteMeta(deleteSubscriptionQuery)).
		WithArgs(userID, feedLinkID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec(regexp.QuoteMeta(deleteOrphanFeedLinkQuery)).
		WithArgs(feedLinkID).
		WillReturnResult(pgxmock.NewResult("DELETE", 0)) // Another subscriber still exists

	err = repo.DeleteSubscription(context.Background(), userID, feedLinkID)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
