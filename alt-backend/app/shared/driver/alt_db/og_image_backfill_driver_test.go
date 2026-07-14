package alt_db

import (
	"context"
	"testing"

	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

// FetchFeedsMissingOgImage returns recent articles whose feed lacks an RSS
// og:image and which have no scraped article_heads og:image yet — the backfill
// work-list. It must be scoped to the 7-day retention window and exclude
// already-covered articles.
func TestFeedRepository_FetchFeedsMissingOgImage(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}

	limit := 50
	articleID := "11111111-1111-1111-1111-111111111111"
	articleURL := "https://example.com/posts/hello"

	mock.ExpectQuery(`(?s)FROM articles.*7 days.*article_heads`).
		WithArgs(limit).
		WillReturnRows(pgxmock.NewRows([]string{"id", "url"}).
			AddRow(articleID, articleURL))

	candidates, err := repo.FetchFeedsMissingOgImage(context.Background(), limit)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, articleID, candidates[0].ArticleID)
	require.Equal(t, articleURL, candidates[0].URL)
	require.NoError(t, mock.ExpectationsWereMet())
}
