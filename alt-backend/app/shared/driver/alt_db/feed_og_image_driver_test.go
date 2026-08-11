package alt_db

import (
	"context"
	"testing"
	"time"

	"alt/domain"

	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

// FetchFeedOgImageTargets answers, for the feeds a reader has brought into
// view, what the resolver needs in order to decide whether to touch the origin
// at all: the page URL, any image already held, and whether a previous refusal
// still stands.
//
// The already-held URL must come from both places one can live — the RSS
// column and a previous on-demand resolution — because a hit in either means
// there is nothing to fetch.
func TestFeedRepository_FetchFeedOgImageTargets(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}

	fetchable := "11111111-1111-1111-1111-111111111111"
	suppressed := "22222222-2222-2222-2222-222222222222"
	held := "33333333-3333-3333-3333-333333333333"

	mock.ExpectQuery(`(?s)website_url.*FROM feeds.*LEFT JOIN feed_og_images`).
		WithArgs([]string{fetchable, suppressed, held}).
		WillReturnRows(pgxmock.NewRows([]string{"feed_id", "page_url", "og_image_url", "suppressed"}).
			AddRow(fetchable, "https://example.com/a", "", false).
			AddRow(suppressed, "https://example.com/b", "", true).
			AddRow(held, "https://example.com/c", "https://cdn.example.com/c.png", false))

	targets, err := repo.FetchFeedOgImageTargets(context.Background(), []string{fetchable, suppressed, held})
	require.NoError(t, err)
	require.Len(t, targets, 3)

	require.True(t, targets[0].NeedsFetch(), "no image and no standing refusal is the only fetchable case")
	require.False(t, targets[1].NeedsFetch(), "a standing refusal must suppress the fetch")
	require.False(t, targets[2].NeedsFetch(), "an image we already hold must suppress the fetch")

	require.NoError(t, mock.ExpectationsWereMet())
}

// An empty id list must not reach the database. Grids ask on every viewport
// change, and most of those changes reveal nothing new.
func TestFeedRepository_FetchFeedOgImageTargets_NoIDs(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}

	targets, err := repo.FetchFeedOgImageTargets(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, targets)
	require.NoError(t, mock.ExpectationsWereMet())
}

// A resolved image is stored as state='resolved' carrying the URL.
func TestFeedRepository_SaveFeedOgImage_Resolved(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	feedID := "11111111-1111-1111-1111-111111111111"
	imageURL := "https://cdn.example.com/a.png"

	mock.ExpectExec(`(?s)INSERT INTO feed_og_images.*ON CONFLICT`).
		WithArgs(feedID, "resolved", imageURL, "", nil).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	require.NoError(t, repo.SaveFeedOgImage(context.Background(), feedID, imageURL, "", 0))
	require.NoError(t, mock.ExpectationsWereMet())
}

// A refusal is stored with no URL, so the CHECK constraint keeps it from
// masquerading as a cache hit, and with the reason an operator would need.
//
// A zero retry window must be written as NULL rather than as "now": NULL is
// "not within this retention window", which is the settled answer a robots.txt
// disallow deserves, while a timestamp of now would make the next scroll
// re-request the origin immediately.
func TestFeedRepository_SaveFeedOgImage_RefusalWithoutRetry(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	feedID := "22222222-2222-2222-2222-222222222222"

	mock.ExpectExec(`(?s)INSERT INTO feed_og_images.*ON CONFLICT`).
		WithArgs(feedID, "unavailable", nil, string(domain.OgImageRefusedByRobots), nil).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.SaveFeedOgImage(context.Background(), feedID, "", domain.OgImageRefusedByRobots, 0)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// A refusal that is worth retrying carries a retry_after instant.
func TestFeedRepository_SaveFeedOgImage_RefusalWithRetry(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	feedID := "33333333-3333-3333-3333-333333333333"

	mock.ExpectExec(`(?s)INSERT INTO feed_og_images.*ON CONFLICT`).
		WithArgs(feedID, "unavailable", nil, string(domain.OgImageRefusedForbidden), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.SaveFeedOgImage(context.Background(), feedID, "", domain.OgImageRefusedForbidden, 24*time.Hour)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// The retention sweep is what keeps on-demand resolution inside the same
// copyright window as every other scraped artifact.
func TestFeedRepository_CleanupExpiredFeedOgImages(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}

	mock.ExpectExec(`DELETE FROM feed_og_images WHERE created_at <`).
		WithArgs(pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("DELETE", 7))

	purged, err := repo.CleanupExpiredFeedOgImages(context.Background(), 7*24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, int64(7), purged)
	require.NoError(t, mock.ExpectationsWereMet())
}
