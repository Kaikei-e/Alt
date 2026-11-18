package alt_db

import (
	"context"
	"testing"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchRSSFeedURLs_RequiresActiveAndSubscribed(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	wantID := uuid.New()
	wantURL := "https://example.com/subscribed.xml"

	// Must parenthesize the availability OR-clause and gate on EXISTS in user_feed_subscriptions.
	expectedSQL := `(?s)SELECT fl\.id, fl\.url FROM feed_links fl.*LEFT JOIN feed_link_availability fla ON fl\.id = fla\.feed_link_id.*WHERE \(fla\.is_active IS NULL OR fla\.is_active = true\).*AND EXISTS \(\s*SELECT 1 FROM user_feed_subscriptions ufs WHERE ufs\.feed_link_id = fl\.id\s*\)`

	mock.ExpectQuery(expectedSQL).
		WillReturnRows(pgxmock.NewRows([]string{"id", "url"}).AddRow(wantID, wantURL))

	links, err := repo.FetchRSSFeedURLs(context.Background())
	require.NoError(t, err)
	require.Len(t, links, 1)
	assert.Equal(t, wantID, links[0].ID)
	assert.Equal(t, wantURL, links[0].URL)
	require.NoError(t, mock.ExpectationsWereMet())
}
