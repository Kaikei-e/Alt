package alt_db

import (
	"alt/utils/logger"
	"context"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

func TestFetchInoreaderSummariesByURLs_QueriesArticlesTable(t *testing.T) {
	logger.InitLogger()

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	urls := []string{"https://example.com/a1", "https://example.com/a2"}
	pubAt := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	fetchedAt := time.Date(2026, 4, 1, 10, 5, 0, 0, time.UTC)

	mock.ExpectQuery(`FROM articles a\s+WHERE a\.url = ANY\(\$1\)`).
		WithArgs(urls).
		WillReturnRows(
			pgxmock.NewRows([]string{
				"article_url",
				"title",
				"author",
				"content",
				"content_type",
				"published_at",
				"fetched_at",
				"inoreader_id",
			}).AddRow(
				"https://example.com/a1",
				"Article 1",
				nil,
				"<p>Summary 1</p>",
				"html",
				pubAt,
				fetchedAt,
				"11111111-1111-1111-1111-111111111111",
			),
		)

	got, err := repo.FetchInoreaderSummariesByURLs(context.Background(), urls)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "https://example.com/a1", got[0].ArticleURL)
	require.Equal(t, "Article 1", got[0].Title)
	require.Nil(t, got[0].Author)
	require.Equal(t, "<p>Summary 1</p>", got[0].Content)
	require.Equal(t, "html", got[0].ContentType)
	require.Equal(t, pubAt, got[0].PublishedAt)
	require.Equal(t, fetchedAt, got[0].FetchedAt)
	require.Equal(t, "11111111-1111-1111-1111-111111111111", got[0].InoreaderID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchInoreaderSummariesByURLs_EmptySlice(t *testing.T) {
	logger.InitLogger()

	repo := &FeedRepository{pool: nil}
	got, err := repo.FetchInoreaderSummariesByURLs(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, got)
}
