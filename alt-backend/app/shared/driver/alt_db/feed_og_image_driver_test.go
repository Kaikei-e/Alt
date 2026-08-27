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
		WillReturnRows(pgxmock.NewRows([]string{"feed_id", "page_url", "og_image_url", "suppressed", "attempts", "retry_after_seconds"}).
			AddRow(fetchable, "https://example.com/a", "", false, int32(2), int64(0)).
			AddRow(suppressed, "https://example.com/b", "", true, int32(3), int64(20)).
			AddRow(held, "https://example.com/c", "https://cdn.example.com/c.png", false, int32(1), int64(0)))

	targets, err := repo.FetchFeedOgImageTargets(context.Background(), []string{fetchable, suppressed, held})
	require.NoError(t, err)
	require.Len(t, targets, 3)

	require.True(t, targets[0].NeedsFetch(), "no image and no standing refusal is the only fetchable case")
	require.False(t, targets[1].NeedsFetch(), "a standing refusal must suppress the fetch")
	require.False(t, targets[2].NeedsFetch(), "an image we already hold must suppress the fetch")

	require.Equal(t, 2, targets[0].Attempts,
		"the attempts already spent decide the next bar, so they must reach the caller")
	require.Equal(t, int64(20), targets[1].RetryAfterSeconds,
		"a suppressed feed must say how much of its bar is left; without it the caller can only answer 'never'")
	require.Zero(t, targets[0].RetryAfterSeconds,
		"an expired bar is no bar")

	require.NoError(t, mock.ExpectationsWereMet())
}

// The remaining-seconds projection is computed in SQL rather than by returning
// the raw retry_after instant, because the caller has no business comparing
// timestamps against a clock that is not the database's.
//
// A row that has never been refused carries retry_after IS NULL, and that must
// arrive as 0 rather than as a negative number or a NULL scan error.
func TestFeedRepository_FetchFeedOgImageTargets_ProjectsRemainingSeconds(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	feedID := "44444444-4444-4444-4444-444444444444"

	mock.ExpectQuery(`(?s)GREATEST\(0, EXTRACT\(EPOCH FROM \(foi\.retry_after - NOW\(\)\)\)\)`).
		WithArgs([]string{feedID}).
		WillReturnRows(pgxmock.NewRows([]string{"feed_id", "page_url", "og_image_url", "suppressed", "attempts", "retry_after_seconds"}).
			AddRow(feedID, "https://example.com/a", "", false, int32(0), int64(0)))

	targets, err := repo.FetchFeedOgImageTargets(context.Background(), []string{feedID})
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.Zero(t, targets[0].Attempts,
		"a feed with no row has spent no attempts; RetryAfter normalises the zero to the first rung")
	require.Zero(t, targets[0].RetryAfterSeconds)
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

// A resolution resets the attempt counter; only a refusal increments it.
//
// The counter's whole job is to say how much patience this feed has already
// cost, and a success is the proof that whatever was wrong is over. Counting
// successes too meant a feed that had resolved cleanly for a week walked into
// the largest bar on its first transient 502 — the escalation punishing the
// well-behaved feeds instead of the failing ones.
//
// It is asserted on the SQL because the arithmetic happens in the statement:
// there is no round trip to read the old value and no Go-side branch to test.
func TestFeedRepository_SaveFeedOgImage_ResolutionResetsAttempts(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	feedID := "11111111-1111-1111-1111-111111111111"

	mock.ExpectExec(`attempts = CASE WHEN EXCLUDED\.state = 'resolved' THEN 1 ELSE feed_og_images\.attempts \+ 1 END`).
		WithArgs(feedID, "resolved", "https://cdn.example.com/a.png", "", nil).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.SaveFeedOgImage(context.Background(), feedID, "https://cdn.example.com/a.png", "", 0)
	require.NoError(t, err)
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
