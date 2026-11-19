package alt_db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetFeedInflowByUser(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	userID := uuid.New()
	linkID := uuid.New()
	from := time.Date(2026, 10, 19, 1, 0, 0, 0, time.FixedZone("JST", 9*3600))
	to := time.Date(2026, 11, 18, 1, 0, 0, 0, time.FixedZone("JST", 9*3600))
	capVal := 10

	mock.ExpectQuery(`FROM user_feed_subscriptions ufs`).
		WithArgs(userID, from.UTC(), to.UTC()).
		WillReturnRows(pgxmock.NewRows([]string{"feed_link_id", "url", "daily_cap", "inflow_count"}).
			AddRow(linkID, "https://example.com/rss", &capVal, int64(25)))

	inflows, err := repo.GetFeedInflowByUser(context.Background(), userID, from, to)
	require.NoError(t, err)
	require.Len(t, inflows, 1)
	assert.Equal(t, linkID, inflows[0].FeedLinkID)
	assert.Equal(t, "https://example.com/rss", inflows[0].FeedURL)
	require.NotNil(t, inflows[0].DailyCap)
	assert.Equal(t, 10, *inflows[0].DailyCap)
	assert.Equal(t, int64(25), inflows[0].InflowCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchGetFeedLinkIDsByArticleIDs(t *testing.T) {
	t.Run("empty input skips db", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := &FeedRepository{pool: mock}
		got, err := repo.BatchGetFeedLinkIDsByArticleIDs(context.Background(), uuid.New(), nil)
		require.NoError(t, err)
		assert.Empty(t, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("maps article ids to feed link ids", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := &FeedRepository{pool: mock}
		userID := uuid.New()
		artID := uuid.New()
		linkID := uuid.New()

		mock.ExpectQuery(`FROM articles a\s+INNER JOIN feeds f`).
			WithArgs(userID, []string{artID.String()}).
			WillReturnRows(pgxmock.NewRows([]string{"id", "feed_link_id"}).
				AddRow(artID, linkID))

		got, err := repo.BatchGetFeedLinkIDsByArticleIDs(context.Background(), userID, []uuid.UUID{artID})
		require.NoError(t, err)
		assert.Equal(t, map[uuid.UUID]uuid.UUID{artID: linkID}, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
