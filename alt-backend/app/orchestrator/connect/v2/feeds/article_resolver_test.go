package feeds

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/domain"
	"alt/orchestrator/usecase/resolve_article_usecase"
	"alt/utils/logger"
)

type stubExtractor struct {
	content string
	title   string
	err     error
}

func (e *stubExtractor) ExtractArticleContent(_ context.Context, _ string) (string, string, error) {
	return e.title, e.content, e.err
}

type stubArticleStore struct {
	byID    *domain.ArticleContent
	byURL   *domain.ArticleContent
	savedID string
	err     error

	askedID        string
	askedURL       string
	savedURL       string
	savedSummaryID string
	savedSummary   string
}

func (s *stubArticleStore) FetchArticleByID(_ context.Context, articleID string) (*domain.ArticleContent, error) {
	s.askedID = articleID
	return s.byID, s.err
}

func (s *stubArticleStore) FetchArticleByURL(_ context.Context, articleURL string) (*domain.ArticleContent, error) {
	s.askedURL = articleURL
	return s.byURL, s.err
}

func (s *stubArticleStore) SaveArticle(_ context.Context, url, _, _ string) (string, error) {
	s.savedURL = url
	return s.savedID, s.err
}

func (s *stubArticleStore) SaveArticleSummary(_ context.Context, articleID, _, _, summary string) error {
	s.savedSummaryID = articleID
	s.savedSummary = summary
	return s.err
}

func newResolveArticleHandler(store *stubArticleStore) *Handler {
	logger.InitLogger()
	uc := resolve_article_usecase.New(store, &stubExtractor{})
	return NewHandler(FeedHandlerDeps{
		ArticleStore:   store,
		ResolveArticle: uc,
	}, &config.Config{
		Server: config.ServerConfig{SSEInterval: 5 * time.Second},
	}, slog.Default())
}

func TestResolveArticle_DBContentPrioritizedOverRequestContent(t *testing.T) {
	articleID := "test-article-id-123"
	store := &stubArticleStore{byID: &domain.ArticleContent{
		ID:      articleID,
		Title:   "DB Title",
		Content: "This is clean content from database",
		URL:     "https://example.com/article",
	}}
	handler := newResolveArticleHandler(store)

	requestContent := "<html><body>This is raw HTML content from request that should be IGNORED</body></html>"
	resolvedArticleID, resolvedTitle, resolvedContent, err := handler.resolveArticle(createAuthContext(), "", articleID, requestContent, "")

	require.NoError(t, err)
	assert.Equal(t, articleID, resolvedArticleID)
	assert.Equal(t, "DB Title", resolvedTitle, "Title should come from the stored article")
	assert.Equal(t, "This is clean content from database", resolvedContent, "Stored content wins over request content")
	assert.Equal(t, articleID, store.askedID)
}

func TestResolveArticle_FallbackToRequestContentWhenDBEmpty(t *testing.T) {
	articleID := "test-article-id-456"
	store := &stubArticleStore{byID: &domain.ArticleContent{
		ID:      articleID,
		Title:   "DB Title",
		Content: "",
		URL:     "https://example.com/article",
	}}
	handler := newResolveArticleHandler(store)

	requestContent := "Request content as fallback"
	resolvedArticleID, resolvedTitle, resolvedContent, err := handler.resolveArticle(createAuthContext(), "", articleID, requestContent, "Request Title")

	require.NoError(t, err)
	assert.Equal(t, articleID, resolvedArticleID)
	assert.Equal(t, "Request Title", resolvedTitle, "Should use provided title when DB title would be overwritten by empty string")
	assert.Equal(t, requestContent, resolvedContent, "Should fallback to request content when stored content is empty")
}

func TestResolveArticle_ErrorWhenDBEmptyAndNoRequestContent(t *testing.T) {
	articleID := "test-article-id-789"
	store := &stubArticleStore{byID: &domain.ArticleContent{
		ID:      articleID,
		Title:   "DB Title",
		Content: "",
		URL:     "https://example.com/article",
	}}
	handler := newResolveArticleHandler(store)

	resolvedArticleID, resolvedTitle, resolvedContent, err := handler.resolveArticle(createAuthContext(), "", articleID, "", "")

	require.Error(t, err, "Should return error when both the stored article and the request are empty")
	assert.Contains(t, err.Error(), "content is empty")
	assert.Empty(t, resolvedArticleID)
	assert.Empty(t, resolvedTitle)
	assert.Empty(t, resolvedContent)
}

func TestResolveArticle_FallbackToRequestContentWhenArticleNotInDB(t *testing.T) {
	store := &stubArticleStore{byID: nil}
	handler := newResolveArticleHandler(store)

	articleID := "non-existent-article-id"
	requestContent := "Fallback content when article not in DB"
	resolvedArticleID, resolvedTitle, resolvedContent, err := handler.resolveArticle(createAuthContext(), "", articleID, requestContent, "Request Title")

	require.NoError(t, err)
	assert.Equal(t, articleID, resolvedArticleID)
	assert.Equal(t, "Request Title", resolvedTitle)
	assert.Equal(t, requestContent, resolvedContent, "Should use request content when the article is not stored")
}

func TestNewHandler_NilResolveArticlePanics(t *testing.T) {
	assert.Panics(t, func() {
		NewHandler(FeedHandlerDeps{
			ResolveArticle: nil,
		}, &config.Config{}, slog.Default())
	})
}
