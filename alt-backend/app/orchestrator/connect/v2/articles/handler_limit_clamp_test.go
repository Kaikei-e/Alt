package articles

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	articlesv2 "alt/gen/proto/alt/articles/v2"

	"alt/config"
	"alt/domain"
	"alt/orchestrator/usecase/fetch_articles_usecase"
	"alt/shared/usecase/fetch_articles_by_tag_usecase"
	"alt/utils/logger"
)

// The article cursor RPCs ask their usecase for limit+1 rows so they can answer
// has_more without a COUNT. The usecases reject anything above 100, so the
// handler ceiling must leave room for that extra probe row — clamping the
// caller's limit to 100 turned the documented maximum page size into an opaque
// Internal error instead of a page.

// limitRecordingArticlesPort records the limit the handler asked for, so the
// test can prove the probe row stays inside the usecase's ceiling.
type limitRecordingArticlesPort struct {
	articles  []*domain.Article
	lastLimit int
}

func (f *limitRecordingArticlesPort) FetchArticlesWithCursor(_ context.Context, _ *time.Time, limit int) ([]*domain.Article, error) {
	f.lastLimit = limit
	return f.articles, nil
}

func (f *limitRecordingArticlesPort) FetchArticleIDsWithCursor(_ context.Context, _ *time.Time, limit int) ([]uuid.UUID, error) {
	f.lastLimit = limit
	return nil, nil
}

type limitRecordingByTagPort struct {
	articles  []*domain.TagTrailArticle
	lastLimit int
}

func (f *limitRecordingByTagPort) FetchArticlesByTag(_ context.Context, _ string, _ *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	f.lastLimit = limit
	return f.articles, nil
}

func (f *limitRecordingByTagPort) FetchArticlesByTagName(_ context.Context, _ string, _ *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	f.lastLimit = limit
	return f.articles, nil
}

func TestFetchArticlesCursor_MaxLimitIsServedNotRejected(t *testing.T) {
	logger.InitLogger()

	port := &limitRecordingArticlesPort{}
	handler := NewHandler(ArticleHandlerDeps{
		FetchArticlesCursor: fetch_articles_usecase.NewFetchArticlesCursorUsecase(port),
	}, &config.Config{}, slog.Default())

	_, err := handler.FetchArticlesCursor(createAuthContext(),
		connect.NewRequest(&articlesv2.FetchArticlesCursorRequest{Limit: 100}))

	require.NoError(t, err)
	require.LessOrEqual(t, port.lastLimit, 100,
		"the has_more probe row pushed the fetch past the usecase ceiling")
}

func TestFetchArticlesByTag_MaxLimitIsServedNotRejected(t *testing.T) {
	logger.InitLogger()

	port := &limitRecordingByTagPort{}
	handler := NewHandler(ArticleHandlerDeps{
		FetchArticlesByTag: fetch_articles_by_tag_usecase.NewFetchArticlesByTagUsecase(port),
	}, &config.Config{}, slog.Default())

	tagName := "go"
	_, err := handler.FetchArticlesByTag(createAuthContext(),
		connect.NewRequest(&articlesv2.FetchArticlesByTagRequest{TagName: &tagName, Limit: 100}))

	require.NoError(t, err)
	require.LessOrEqual(t, port.lastLimit, 100,
		"the has_more probe row pushed the fetch past the usecase ceiling")
}

// An over-sized limit must be clamped down to the same served ceiling rather
// than rejected, which is what the handlers already promised by clamping.
func TestFetchArticlesCursor_OverMaxLimitIsClampedNotRejected(t *testing.T) {
	logger.InitLogger()

	port := &limitRecordingArticlesPort{}
	handler := NewHandler(ArticleHandlerDeps{
		FetchArticlesCursor: fetch_articles_usecase.NewFetchArticlesCursorUsecase(port),
	}, &config.Config{}, slog.Default())

	_, err := handler.FetchArticlesCursor(createAuthContext(),
		connect.NewRequest(&articlesv2.FetchArticlesCursorRequest{Limit: 50000}))

	require.NoError(t, err)
	require.LessOrEqual(t, port.lastLimit, 100,
		"the has_more probe row pushed the fetch past the usecase ceiling")
}
