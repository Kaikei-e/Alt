package rest_feeds

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/di"
	"alt/domain"
	"alt/orchestrator/usecase/fetch_article_summaries_usecase"
	"alt/utils/batch_article_fetcher"
	"alt/utils/logger"
)

type stubFetchInoreaderSummaryUsecase struct {
	summaries []*domain.InoreaderSummary
	err       error
	lastURLs  []string
}

func (s *stubFetchInoreaderSummaryUsecase) Execute(_ context.Context, urls []string) ([]*domain.InoreaderSummary, error) {
	s.lastURLs = urls
	return s.summaries, s.err
}

type stubArticleRepo struct {
	articles map[string]*domain.ArticleContent
	err      error
}

func (s *stubArticleRepo) FetchArticlesByURLs(_ context.Context, _ []string) (map[string]*domain.ArticleContent, error) {
	return s.articles, s.err
}

func (s *stubArticleRepo) SaveArticle(_ context.Context, _, _, _ string) (string, error) {
	return "saved-id", nil
}

type stubArticleBatchFetcher struct{}

func (s *stubArticleBatchFetcher) FetchMultiple(_ context.Context, _ []string) map[string]*batch_article_fetcher.FetchResult {
	return nil
}

type stubArticleSummarizer struct {
	summaries map[string]string
}

func (s *stubArticleSummarizer) EnsureSummary(_ context.Context, articleID, _, _ string) (string, bool, error) {
	return s.summaries[articleID], true, nil
}

func withAuthenticatedUser(req *http.Request) *http.Request {
	uid := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	ctx := domain.SetUserContext(req.Context(), &domain.UserContext{
		UserID:    uid,
		TenantID:  uid,
		Email:     "user@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	return req.WithContext(ctx)
}

func TestRestHandleFetchInoreaderSummary(t *testing.T) {
	logger.InitLogger()

	t.Run("empty feed_urls returns 400 Bad Request", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/v1/feeds/fetch/summary/provided", bytes.NewBufferString(`{"feed_urls":[]}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := RestHandleFetchInoreaderSummary(&di.ApplicationComponents{})
		err := handler(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "feed_urls is required and cannot be empty")
	})

	t.Run("more than 50 feed_urls returns 400 Bad Request", func(t *testing.T) {
		urls := make([]string, 51)
		for i := range urls {
			urls[i] = fmt.Sprintf("https://example.com/feed/%d", i)
		}
		payload, err := json.Marshal(FeedSummaryRequest{FeedURLs: urls})
		require.NoError(t, err)

		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/v1/feeds/fetch/summary/provided", bytes.NewReader(payload))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := RestHandleFetchInoreaderSummary(&di.ApplicationComponents{})
		err = handler(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "50")
	})

	t.Run("SSRF private IP and loopback URLs rejected with 400 Bad Request", func(t *testing.T) {
		ssrfURLs := []string{
			"http://127.0.0.1/feed.xml",
			"http://localhost/feed.xml",
			"http://10.0.0.1/internal/feed",
			"http://192.168.1.1/admin",
			"http://169.254.169.254/latest/meta-data/",
		}
		for _, targetURL := range ssrfURLs {
			t.Run(targetURL, func(t *testing.T) {
				payload, err := json.Marshal(FeedSummaryRequest{FeedURLs: []string{targetURL}})
				require.NoError(t, err)

				e := echo.New()
				req := httptest.NewRequest(http.MethodPost, "/v1/feeds/fetch/summary/provided", bytes.NewReader(payload))
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
				rec := httptest.NewRecorder()
				c := e.NewContext(req, rec)

				handler := RestHandleFetchInoreaderSummary(&di.ApplicationComponents{})
				err = handler(c)
				require.NoError(t, err)
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "URL not allowed for security reasons")
			})
		}
	})

	t.Run("valid request maps matched_articles and total_matched", func(t *testing.T) {
		t.Setenv("FEED_ALLOWED_HOSTS", "example.com")
		author := "Alice"
		pubAt := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
		fetchedAt := time.Date(2026, 10, 20, 12, 5, 0, 0, time.UTC)

		stubUC := &stubFetchInoreaderSummaryUsecase{
			summaries: []*domain.InoreaderSummary{
				{
					ArticleURL:  "https://example.com/post-1",
					Title:       "First Post",
					Author:      &author,
					Content:     "<p>Summary 1</p>",
					ContentType: "text/html",
					PublishedAt: pubAt,
					FetchedAt:   fetchedAt,
					InoreaderID: "ino-1",
				},
			},
		}
		container := &di.ApplicationComponents{
			FetchInoreaderSummaryUsecase: stubUC,
		}

		payload, err := json.Marshal(FeedSummaryRequest{
			FeedURLs: []string{"https://example.com/post-1", "https://example.com/post-2"},
		})
		require.NoError(t, err)

		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/v1/feeds/fetch/summary/provided", bytes.NewReader(payload))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := RestHandleFetchInoreaderSummary(container)
		err = handler(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var got FeedSummaryProvidedResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, 1, got.TotalMatched)
		assert.Equal(t, 2, got.RequestedCount)
		require.Len(t, got.MatchedArticles, 1)
		assert.Equal(t, "https://example.com/post-1", got.MatchedArticles[0].ArticleURL)
		assert.Equal(t, "First Post", got.MatchedArticles[0].Title)
		assert.Equal(t, "Alice", got.MatchedArticles[0].Author)
		assert.Equal(t, "<p>Summary 1</p>", got.MatchedArticles[0].Content)
		assert.Equal(t, "text/html", got.MatchedArticles[0].ContentType)
		assert.Equal(t, "2026-10-20T12:00:00Z", got.MatchedArticles[0].PublishedAt)
		assert.Equal(t, "2026-10-20T12:05:00Z", got.MatchedArticles[0].FetchedAt)
		assert.Equal(t, "ino-1", got.MatchedArticles[0].InoreaderID)
	})
}

func TestRestHandleFetchArticleSummary(t *testing.T) {
	logger.InitLogger()

	t.Run("unauthenticated returns 401 Unauthorized", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/v1/feeds/fetch/summary", bytes.NewBufferString(`{"feed_urls":["https://example.com/a"]}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := RestHandleFetchArticleSummary(&di.ApplicationComponents{}, &config.Config{})
		err := handler(c)
		var httpErr *echo.HTTPError
		require.ErrorAs(t, err, &httpErr)
		assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
	})

	t.Run("empty feed_urls returns 400 Bad Request", func(t *testing.T) {
		e := echo.New()
		req := withAuthenticatedUser(httptest.NewRequest(http.MethodPost, "/v1/feeds/fetch/summary", bytes.NewBufferString(`{"feed_urls":[]}`)))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := RestHandleFetchArticleSummary(&di.ApplicationComponents{}, &config.Config{})
		err := handler(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "feed_urls is required and cannot be empty")
	})

	t.Run("more than 50 feed_urls returns 400 Bad Request", func(t *testing.T) {
		urls := make([]string, 51)
		for i := range urls {
			urls[i] = fmt.Sprintf("https://example.com/feed/%d", i)
		}
		payload, err := json.Marshal(FeedSummaryRequest{FeedURLs: urls})
		require.NoError(t, err)

		e := echo.New()
		req := withAuthenticatedUser(httptest.NewRequest(http.MethodPost, "/v1/feeds/fetch/summary", bytes.NewReader(payload)))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := RestHandleFetchArticleSummary(&di.ApplicationComponents{}, &config.Config{})
		err = handler(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "50")
	})

	t.Run("SSRF private IP and loopback URLs rejected with 400 Bad Request", func(t *testing.T) {
		ssrfURLs := []string{
			"http://127.0.0.1/feed.xml",
			"http://localhost/feed.xml",
			"http://10.0.0.1/internal/feed",
			"http://172.16.0.1/internal/feed",
			"http://192.168.1.1/admin",
		}
		for _, targetURL := range ssrfURLs {
			t.Run(targetURL, func(t *testing.T) {
				payload, err := json.Marshal(FeedSummaryRequest{FeedURLs: []string{targetURL}})
				require.NoError(t, err)

				e := echo.New()
				req := withAuthenticatedUser(httptest.NewRequest(http.MethodPost, "/v1/feeds/fetch/summary", bytes.NewReader(payload)))
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
				rec := httptest.NewRecorder()
				c := e.NewContext(req, rec)

				handler := RestHandleFetchArticleSummary(&di.ApplicationComponents{}, &config.Config{})
				err = handler(c)
				require.NoError(t, err)
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "URL not allowed for security reasons")
			})
		}
	})

	t.Run("valid request maps matched_articles and total_matched", func(t *testing.T) {
		t.Setenv("FEED_ALLOWED_HOSTS", "example.com")
		repo := &stubArticleRepo{
			articles: map[string]*domain.ArticleContent{
				"https://example.com/a1": {
					ID:    "art-101",
					Title: "Article 101",
				},
			},
		}
		summarizer := &stubArticleSummarizer{
			summaries: map[string]string{
				"art-101": "Concise AI summary of Article 101",
			},
		}
		uc := fetch_article_summaries_usecase.NewUsecase(repo, &stubArticleBatchFetcher{}, summarizer)
		container := &di.ApplicationComponents{
			FetchArticleSummariesUsecase: uc,
		}

		payload, err := json.Marshal(FeedSummaryRequest{
			FeedURLs: []string{"https://example.com/a1"},
		})
		require.NoError(t, err)

		e := echo.New()
		req := withAuthenticatedUser(httptest.NewRequest(http.MethodPost, "/v1/feeds/fetch/summary", bytes.NewReader(payload)))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := RestHandleFetchArticleSummary(container, &config.Config{})
		err = handler(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var got FeedSummaryProvidedResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, 1, got.TotalMatched)
		assert.Equal(t, 1, got.RequestedCount)
		require.Len(t, got.MatchedArticles, 1)
		assert.Equal(t, "https://example.com/a1", got.MatchedArticles[0].ArticleURL)
		assert.Equal(t, "Article 101", got.MatchedArticles[0].Title)
		assert.Equal(t, "Concise AI summary of Article 101", got.MatchedArticles[0].Content)
		assert.Equal(t, "text/html", got.MatchedArticles[0].ContentType)
		assert.Equal(t, "art-101", got.MatchedArticles[0].InoreaderID)
	})
}
