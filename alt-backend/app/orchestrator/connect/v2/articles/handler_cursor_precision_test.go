package articles

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/domain"
	articlesv2 "alt/gen/proto/alt/articles/v2"
	"alt/orchestrator/usecase/fetch_articles_usecase"
	"alt/shared/usecase/fetch_articles_by_tag_usecase"
	"alt/utils/logger"
)

type fakeArticlesPort struct {
	articles   []*domain.Article
	lastCursor *time.Time
}

func (f *fakeArticlesPort) FetchArticlesWithCursor(_ context.Context, cursor *time.Time, _ int) ([]*domain.Article, error) {
	f.lastCursor = cursor
	return f.articles, nil
}

func (f *fakeArticlesPort) FetchArticleIDsWithCursor(_ context.Context, cursor *time.Time, _ int) ([]uuid.UUID, error) {
	f.lastCursor = cursor
	return nil, nil
}

type fakeArticlesByTagPort struct {
	articles   []*domain.TagTrailArticle
	lastCursor *time.Time
}

func (f *fakeArticlesByTagPort) FetchArticlesByTag(_ context.Context, _ string, cursor *time.Time, _ int) ([]*domain.TagTrailArticle, error) {
	f.lastCursor = cursor
	return f.articles, nil
}

func (f *fakeArticlesByTagPort) FetchArticlesByTagName(_ context.Context, _ string, cursor *time.Time, _ int) ([]*domain.TagTrailArticle, error) {
	f.lastCursor = cursor
	return f.articles, nil
}

func TestFetchArticlesCursor_RoundTripKeepsSubSecondPrecision(t *testing.T) {
	logger.InitLogger()

	firstPageTail := time.Date(2026, time.March, 2, 10, 0, 0, 123456000, time.UTC)
	sameSecond := firstPageTail.Add(-500 * time.Microsecond)

	port := &fakeArticlesPort{articles: []*domain.Article{
		{ID: uuid.New(), Title: "first", URL: "https://example.com/1", PublishedAt: firstPageTail},
		{ID: uuid.New(), Title: "second", URL: "https://example.com/2", PublishedAt: sameSecond},
	}}

	handler := NewHandler(ArticleHandlerDeps{
		FetchArticlesCursor: fetch_articles_usecase.NewFetchArticlesCursorUsecase(port),
	}, &config.Config{}, slog.Default())
	ctx := createAuthContext()

	page1, err := handler.FetchArticlesCursor(ctx, connect.NewRequest(&articlesv2.FetchArticlesCursorRequest{Limit: 1}))
	require.NoError(t, err)
	require.True(t, page1.Msg.HasMore)
	require.NotNil(t, page1.Msg.NextCursor)

	_, err = handler.FetchArticlesCursor(ctx, connect.NewRequest(&articlesv2.FetchArticlesCursorRequest{
		Limit:  1,
		Cursor: page1.Msg.NextCursor,
	}))
	require.NoError(t, err)
	require.NotNil(t, port.lastCursor)

	assert.True(t, port.lastCursor.Equal(firstPageTail),
		"cursor %q became %q, so every row between them is skipped",
		firstPageTail.Format(time.RFC3339Nano), port.lastCursor.Format(time.RFC3339Nano))
}

func TestFetchArticlesByTag_RoundTripKeepsSubSecondPrecision(t *testing.T) {
	logger.InitLogger()

	firstPageTail := time.Date(2026, time.March, 2, 10, 0, 0, 123456000, time.UTC)
	sameSecond := firstPageTail.Add(-500 * time.Microsecond)

	port := &fakeArticlesByTagPort{articles: []*domain.TagTrailArticle{
		{ID: uuid.New().String(), Title: "first", Link: "https://example.com/1", PublishedAt: firstPageTail},
		{ID: uuid.New().String(), Title: "second", Link: "https://example.com/2", PublishedAt: sameSecond},
	}}

	handler := NewHandler(ArticleHandlerDeps{
		FetchArticlesByTag: fetch_articles_by_tag_usecase.NewFetchArticlesByTagUsecase(port),
	}, &config.Config{}, slog.Default())
	ctx := createAuthContext()
	tagName := "go"

	page1, err := handler.FetchArticlesByTag(ctx, connect.NewRequest(&articlesv2.FetchArticlesByTagRequest{
		TagName: &tagName,
		Limit:   1,
	}))
	require.NoError(t, err)
	require.True(t, page1.Msg.HasMore)
	require.NotNil(t, page1.Msg.NextCursor)

	_, err = handler.FetchArticlesByTag(ctx, connect.NewRequest(&articlesv2.FetchArticlesByTagRequest{
		TagName: &tagName,
		Limit:   1,
		Cursor:  page1.Msg.NextCursor,
	}))
	require.NoError(t, err)
	require.NotNil(t, port.lastCursor)

	assert.True(t, port.lastCursor.Equal(firstPageTail),
		"cursor %q became %q, so every row between them is skipped",
		firstPageTail.Format(time.RFC3339Nano), port.lastCursor.Format(time.RFC3339Nano))
}
