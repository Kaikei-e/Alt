package resolve_article_usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
)

type stubArticleRepo struct {
	byID    *domain.ArticleContent
	byURL   *domain.ArticleContent
	savedID string
	err     error

	askedID  string
	askedURL string
	savedURL string
}

func (s *stubArticleRepo) FetchArticleByID(_ context.Context, articleID string) (*domain.ArticleContent, error) {
	s.askedID = articleID
	return s.byID, s.err
}

func (s *stubArticleRepo) FetchArticleByURL(_ context.Context, articleURL string) (*domain.ArticleContent, error) {
	s.askedURL = articleURL
	return s.byURL, s.err
}

func (s *stubArticleRepo) SaveArticle(_ context.Context, url, _, _ string) (string, error) {
	s.savedURL = url
	return s.savedID, s.err
}

type stubExtractor struct {
	content string
	title   string
	err     error
	url     string
}

func (s *stubExtractor) ExtractArticleContent(_ context.Context, urlStr string) (string, string, error) {
	s.url = urlStr
	return s.content, s.title, s.err
}

func TestResolveArticle_DBContentPrioritizedOverRequestContent(t *testing.T) {
	articleID := "test-article-id-123"
	repo := &stubArticleRepo{
		byID: &domain.ArticleContent{
			ID:      articleID,
			Title:   "DB Title",
			Content: "This is clean content from database",
			URL:     "https://example.com/article",
		},
	}
	uc := New(repo, &stubExtractor{})

	requestContent := "<html><body>This is raw HTML content from request that should be IGNORED</body></html>"
	resolvedArticleID, resolvedTitle, resolvedContent, err := uc.ResolveArticle(context.Background(), "", articleID, requestContent, "")

	require.NoError(t, err)
	assert.Equal(t, articleID, resolvedArticleID)
	assert.Equal(t, "DB Title", resolvedTitle, "Title should come from the stored article")
	assert.Equal(t, "This is clean content from database", resolvedContent, "Stored content wins over request content")
	assert.Equal(t, articleID, repo.askedID)
}

func TestResolveArticle_FallbackToRequestContentWhenDBEmpty(t *testing.T) {
	articleID := "test-article-id-456"
	repo := &stubArticleRepo{
		byID: &domain.ArticleContent{
			ID:      articleID,
			Title:   "DB Title",
			Content: "",
			URL:     "https://example.com/article",
		},
	}
	uc := New(repo, &stubExtractor{})

	requestContent := "Request content as fallback"
	resolvedArticleID, resolvedTitle, resolvedContent, err := uc.ResolveArticle(context.Background(), "", articleID, requestContent, "Request Title")

	require.NoError(t, err)
	assert.Equal(t, articleID, resolvedArticleID)
	assert.Equal(t, "Request Title", resolvedTitle, "Should use provided title when DB title would be overwritten by empty string")
	assert.Equal(t, requestContent, resolvedContent, "Should fallback to request content when stored content is empty")
}

func TestResolveArticle_ErrorWhenDBEmptyAndNoRequestContent(t *testing.T) {
	articleID := "test-article-id-789"
	repo := &stubArticleRepo{
		byID: &domain.ArticleContent{
			ID:      articleID,
			Title:   "DB Title",
			Content: "",
			URL:     "https://example.com/article",
		},
	}
	uc := New(repo, &stubExtractor{})

	resolvedArticleID, resolvedTitle, resolvedContent, err := uc.ResolveArticle(context.Background(), "", articleID, "", "")

	require.Error(t, err, "Should return error when both the stored article and the request are empty")
	assert.Contains(t, err.Error(), "content is empty")
	assert.Empty(t, resolvedArticleID)
	assert.Empty(t, resolvedTitle)
	assert.Empty(t, resolvedContent)
}

func TestResolveArticle_FallbackToRequestContentWhenArticleNotInDB(t *testing.T) {
	repo := &stubArticleRepo{byID: nil}
	uc := New(repo, &stubExtractor{})

	articleID := "non-existent-article-id"
	requestContent := "Fallback content when article not in DB"
	resolvedArticleID, resolvedTitle, resolvedContent, err := uc.ResolveArticle(context.Background(), "", articleID, requestContent, "Request Title")

	require.NoError(t, err)
	assert.Equal(t, articleID, resolvedArticleID)
	assert.Equal(t, "Request Title", resolvedTitle)
	assert.Equal(t, requestContent, resolvedContent, "Should use request content when the article is not stored")
}

func TestResolveArticle_RequiresFeedURLOrArticleID(t *testing.T) {
	uc := New(&stubArticleRepo{}, &stubExtractor{})
	_, _, _, err := uc.ResolveArticle(context.Background(), "", "", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "feed_url or article_id is required")
}

func TestResolveArticle_ExistingArticleByURL(t *testing.T) {
	repo := &stubArticleRepo{
		byURL: &domain.ArticleContent{
			ID:      "url-article-id",
			Title:   "URL Title",
			Content: "URL Content",
		},
	}
	uc := New(repo, &stubExtractor{})
	id, title, content, err := uc.ResolveArticle(context.Background(), "https://example.com/feed", "", "", "")
	require.NoError(t, err)
	assert.Equal(t, "url-article-id", id)
	assert.Equal(t, "URL Title", title)
	assert.Equal(t, "URL Content", content)
}

func TestResolveArticle_SavesContentWhenProvided(t *testing.T) {
	repo := &stubArticleRepo{
		savedID: "new-article-id",
	}
	uc := New(repo, &stubExtractor{})
	id, title, content, err := uc.ResolveArticle(context.Background(), "https://example.com/feed", "", "Custom Content", "Custom Title")
	require.NoError(t, err)
	assert.Equal(t, "new-article-id", id)
	assert.Equal(t, "Custom Title", title)
	assert.Equal(t, "Custom Content", content)
	assert.Equal(t, "https://example.com/feed", repo.savedURL)
}

func TestResolveArticle_FetchesViaExtractorWhenContentEmpty(t *testing.T) {
	repo := &stubArticleRepo{
		savedID: "extracted-article-id",
	}
	extractor := &stubExtractor{
		content: "Extracted Body",
		title:   "Extracted Title",
	}
	uc := New(repo, extractor)
	id, title, content, err := uc.ResolveArticle(context.Background(), "https://example.com/feed", "", "", "")
	require.NoError(t, err)
	assert.Equal(t, "extracted-article-id", id)
	assert.Equal(t, "Extracted Title", title)
	assert.Equal(t, "Extracted Body", content)
	assert.Equal(t, "https://example.com/feed", extractor.url)
}

func TestResolveArticle_ExtractorError(t *testing.T) {
	repo := &stubArticleRepo{}
	extractor := &stubExtractor{
		err: errors.New("network failure"),
	}
	uc := New(repo, extractor)
	_, _, _, err := uc.ResolveArticle(context.Background(), "https://example.com/feed", "", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch article content")
}
