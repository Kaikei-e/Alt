package rest

import (
	"alt/config"
	"alt/di"
	"alt/domain"
	"alt/usecase/recap_articles_usecase"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleRecapArticles_Success(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/recap/articles?from=2025-11-01T00:00:00Z&to=2025-11-02T00:00:00Z&page=2&page_size=100&fields=title,fulltext&lang=ja", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	page := &domain.RecapArticlesPage{
		Total:    1,
		Page:     2,
		PageSize: 100,
		HasMore:  false,
		Articles: []domain.RecapArticle{
			{
				ID:       uuid.New(),
				FullText: "hello world",
				Title:    stringPtr("Sample"),
			},
		},
	}

	stubPort := &recapPortStub{result: page}
	usecase := recap_articles_usecase.NewRecapArticlesUsecase(stubPort, recap_articles_usecase.Config{
		DefaultPageSize: 500,
		MaxPageSize:     2000,
		MaxRangeDays:    8,
	})
	container := &di.ApplicationComponents{RecapArticlesUsecase: usecase}
	cfg := &config.Config{Recap: config.RecapConfig{DefaultPageSize: 500, MaxPageSize: 2000, RateLimitRPS: 10, RateLimitBurst: 10}}

	handler := handleRecapArticles(container, cfg, newRecapRateLimiter(10, 10))
	require.NoError(t, handler(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var response RecapArticlesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, 1, response.Total)
	assert.Equal(t, 100, response.PageSize)
	assert.Len(t, response.Articles, 1)
	assert.Equal(t, "hello world", response.Articles[0].FullText)

	assert.Equal(t, 2, stubPort.lastQuery.Page)
	assert.Equal(t, 100, stubPort.lastQuery.PageSize)
	assert.Equal(t, []string{"title", "fulltext"}, stubPort.lastQuery.Fields)
}

func TestHandleRecapArticles_InvalidParams(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/recap/articles?to=2025-11-02T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	container := &di.ApplicationComponents{}
	cfg := &config.Config{Recap: config.RecapConfig{DefaultPageSize: 500, MaxPageSize: 2000, RateLimitRPS: 5, RateLimitBurst: 5}}

	handler := handleRecapArticles(container, cfg, newRecapRateLimiter(5, 5))
	err := handler(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "from is required")
}

func TestHandleRecapArticles_RateLimited(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/recap/articles?from=2025-11-01T00:00:00Z&to=2025-11-02T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	stubPort := &recapPortStub{}
	usecase := recap_articles_usecase.NewRecapArticlesUsecase(stubPort, recap_articles_usecase.Config{DefaultPageSize: 500, MaxPageSize: 2000, MaxRangeDays: 8})
	container := &di.ApplicationComponents{RecapArticlesUsecase: usecase}
	cfg := &config.Config{Recap: config.RecapConfig{DefaultPageSize: 500, MaxPageSize: 2000, RateLimitRPS: 1, RateLimitBurst: 1}}
	limiter := newRecapRateLimiter(1, 1)
	handler := handleRecapArticles(container, cfg, limiter)

	require.NoError(t, handler(c))
	req2 := httptest.NewRequest(http.MethodGet, "/v1/recap/articles?from=2025-11-01T00:00:00Z&to=2025-11-02T00:00:00Z", nil)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	err := handler(c2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
}

type recapPortStub struct {
	result    *domain.RecapArticlesPage
	err       error
	lastQuery domain.RecapArticlesQuery
}

func (s *recapPortStub) FetchRecapArticles(ctx context.Context, query domain.RecapArticlesQuery) (*domain.RecapArticlesPage, error) {
	s.lastQuery = query
	return s.result, s.err
}

func stringPtr(value string) *string {
	return &value
}
