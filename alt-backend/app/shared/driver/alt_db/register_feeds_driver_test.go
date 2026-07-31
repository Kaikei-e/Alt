package alt_db

import (
	"testing"
	"time"

	"alt/orchestrator/driver/models"

	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

// TestFeedRepository_RegisterMultipleFeedsWithState_ReturnsFeedIDs pins what the
// `RETURNING id` of the feeds upsert actually is: a feeds.id. It is not an
// articles.id — articles rows are written by a different driver entirely, and
// the feed-registration transaction never touches that table. Naming this value
// after articles is what let a feeds.id reach an ArticleID event field
// (ADR-000953).
func TestFeedRepository_RegisterMultipleFeedsWithState_ReturnsFeedIDs(t *testing.T) {
	pubDate := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)

	newFeed := func(url string) models.Feed {
		return models.Feed{
			Title:       "Example Item",
			Description: "Example description",
			WebsiteURL:  url,
			PubDate:     pubDate,
			CreatedAt:   pubDate,
			UpdatedAt:   pubDate,
		}
	}

	type returnedRow struct {
		feedID  string
		created bool
	}

	tests := []struct {
		name     string
		feeds    []models.Feed
		returned []returnedRow
		want     []FeedRegistrationResult
	}{
		{
			name:     "insert surfaces the newly created feeds.id",
			feeds:    []models.Feed{newFeed("https://example.com/a")},
			returned: []returnedRow{{feedID: "11111111-1111-4111-a111-111111111111", created: true}},
			want: []FeedRegistrationResult{
				{FeedID: "11111111-1111-4111-a111-111111111111", Created: true},
			},
		},
		{
			name:  "on-conflict update surfaces the existing feeds.id",
			feeds: []models.Feed{newFeed("https://example.com/a"), newFeed("https://example.com/b")},
			returned: []returnedRow{
				{feedID: "22222222-2222-4222-a222-222222222222", created: false},
				{feedID: "33333333-3333-4333-a333-333333333333", created: true},
			},
			want: []FeedRegistrationResult{
				{FeedID: "22222222-2222-4222-a222-222222222222", Created: false},
				{FeedID: "33333333-3333-4333-a333-333333333333", Created: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			mock.ExpectBegin()
			batch := mock.ExpectBatch()
			for i, row := range tt.returned {
				feed := tt.feeds[i]
				batch.ExpectQuery("INSERT INTO feeds").
					WithArgs(feed.Title, feed.Description, feed.WebsiteURL, feed.PubDate,
						feed.CreatedAt, feed.UpdatedAt, feed.FeedLinkID, feed.OgImageURL).
					WillReturnRows(pgxmock.NewRows([]string{"id", "created"}).AddRow(row.feedID, row.created))
			}
			mock.ExpectCommit()

			repo := &FeedRepository{pool: mock}

			got, err := repo.RegisterMultipleFeedsWithState(t.Context(), tt.feeds)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
