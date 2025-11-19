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
	"time"

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
	require.NoError(t, handler(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
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
	require.NoError(t, handler(c2))
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
}

func TestGetSevenDayRecap_AttachesClusterDraft(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/recap/7days", nil)
	req.Header.Set("X-Genre-Draft-Id", "draft-123")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	summary := &domain.RecapSummary{
		JobID:         "job-1",
		ExecutedAt:    time.Now(),
		WindowStart:   time.Now(),
		WindowEnd:     time.Now(),
		TotalArticles: 1,
		Genres:        []domain.RecapGenre{},
	}

	loader := &clusterDraftLoaderStub{draft: sampleClusterDraft()}
	handler := NewRecapHandler(&recapUsecaseStub{summary: summary}, loader)

	require.NoError(t, handler.GetSevenDayRecap(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var response domain.RecapSummary
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.NotNil(t, response.ClusterDraft)
	assert.Equal(t, "draft-123", response.ClusterDraft.ID)
	assert.Equal(t, "draft-123", loader.lastID)
}

func TestGetSevenDayRecap_SkipsDraftWhenHeaderMissing(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/recap/7days", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	summary := &domain.RecapSummary{
		JobID:         "job-1",
		ExecutedAt:    time.Now(),
		WindowStart:   time.Now(),
		WindowEnd:     time.Now(),
		TotalArticles: 1,
	}

	loader := &clusterDraftLoaderStub{draft: sampleClusterDraft()}
	handler := NewRecapHandler(&recapUsecaseStub{summary: summary}, loader)

	require.NoError(t, handler.GetSevenDayRecap(c))
	var response domain.RecapSummary
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Nil(t, response.ClusterDraft)
	assert.Empty(t, loader.lastID)
}

type recapUsecaseStub struct {
	summary *domain.RecapSummary
	err     error
}

func (s *recapUsecaseStub) GetSevenDayRecap(ctx context.Context) (*domain.RecapSummary, error) {
	return s.summary, s.err
}

type clusterDraftLoaderStub struct {
	draft  *domain.ClusterDraft
	err    error
	lastID string
}

func (s *clusterDraftLoaderStub) LoadDraft(draftID string) (*domain.ClusterDraft, error) {
	s.lastID = draftID
	if s.err != nil {
		return nil, s.err
	}
	if s.draft != nil && s.draft.ID == draftID {
		draftCopy := *s.draft
		return &draftCopy, nil
	}
	return nil, nil
}

func sampleClusterDraft() *domain.ClusterDraft {
	return &domain.ClusterDraft{
		ID:           "draft-123",
		Description:  "mock draft",
		Source:       "test",
		GeneratedAt:  time.Now(),
		TotalEntries: 2,
		Genres: []domain.ClusterGenre{
			{
				Genre:        "society_justice",
				SampleSize:   2,
				ClusterCount: 1,
				Clusters: []domain.ClusterSegment{
					{
						ClusterID:                "cluster-0",
						Label:                    "Cluster 1",
						Count:                    2,
						MarginMean:               0.1,
						MarginStd:                0.01,
						TopBoostMean:             0.2,
						GraphBoostAvailableRatio: 0.5,
						TagCountMean:             3,
						TagEntropyMean:           0.4,
						TopTags:                  []string{"tag"},
						RepresentativeArticles: []domain.ClusterArticle{
							{
								ArticleID:      "a1",
								Margin:         0.1,
								TopBoost:       0.2,
								Strategy:       "graph_boost",
								TagCount:       3,
								CandidateCount: 3,
								TopTags:        []string{"tag"},
							},
						},
					},
				},
			},
		},
	}
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
