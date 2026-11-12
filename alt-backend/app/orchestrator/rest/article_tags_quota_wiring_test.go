package rest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	connectmiddleware "alt/connect/v2/middleware"
	"alt/di"
	"alt/domain"
	"alt/orchestrator/gateway/fetch_article_tags_gateway"
	"alt/orchestrator/usecase/fetch_article_tags_usecase"
	"alt/shared/driver/mqhub_connect"
	"alt/utils/logger"
)

type restTagFixture struct {
	stored      map[string]bool
	generations atomic.Int32
	failTagger  bool
}

func (f *restTagFixture) FetchArticleTags(_ context.Context, id string) ([]*domain.FeedTag, error) {
	if f.stored[id] {
		return []*domain.FeedTag{{ID: "t", TagName: "stored", CreatedAt: time.Now()}}, nil
	}
	return []*domain.FeedTag{}, nil
}

func (f *restTagFixture) UpsertArticleTags(context.Context, string, string, []domain.TagUpsert) (int32, error) {
	return 1, nil
}

func (f *restTagFixture) FetchArticleByID(_ context.Context, id string) (*domain.ArticleContent, error) {
	return &domain.ArticleContent{ID: id, Title: "t", Content: "c", FeedID: uuid.NewString()}, nil
}

func (f *restTagFixture) IsEnabled() bool { return true }

func (f *restTagFixture) GenerateTagsForArticle(context.Context, mqhub_connect.GenerateTagsRequest) (*mqhub_connect.GenerateTagsResponse, error) {
	f.generations.Add(1)
	if f.failTagger {
		return nil, fmt.Errorf("mq-hub unavailable")
	}
	return &mqhub_connect.GenerateTagsResponse{
		Success: true,
		Tags:    []mqhub_connect.GeneratedTag{{ID: "g", Name: "generated", Confidence: 0.9}},
	}, nil
}

func newTagRouteServer(t *testing.T, fx *restTagFixture, budget, inFlight int) (*echo.Echo, string) {
	t.Helper()
	logger.InitLogger()
	pub, priv := newRESTTestKeyPair(t)

	quota := config.DefaultUserQuotaConfig()
	quota.ArticleTags = config.OperationQuotaConfig{Rate: 1, Burst: budget, Window: time.Hour, MaxInFlight: inFlight}
	limiter, err := connectmiddleware.NewUserQuotaLimiter(quota, time.Now, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	gcfg := fetch_article_tags_gateway.DefaultConfig()
	gcfg.RetryBackoff = time.Millisecond
	gw := fetch_article_tags_gateway.NewFetchArticleTagsGateway(fx, fx, fx, gcfg, limiter)

	container := &di.ApplicationComponents{
		UserQuotaLimiter:        limiter,
		FetchArticleTagsUsecase: fetch_article_tags_usecase.NewFetchArticleTagsUsecase(gw),
	}
	cfg := &config.Config{Auth: config.AuthConfig{
		BackendTokenPublicKey: pub,
		BackendTokenIssuer:    "auth-hub",
		BackendTokenAudience:  "alt-backend",
	}}
	cfg.Server.ReadTimeout = 5 * time.Second

	e := echo.New()
	RegisterRoutes(context.Background(), e, container, cfg)
	return e, signRESTTestJWT(t, priv, uuid.New())
}

func getTags(e *echo.Echo, token, id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/v1/articles/"+id+"/tags", nil)
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestArticleTagsRoute_GenerationIsBoundedByBudget(t *testing.T) {
	fx := &restTagFixture{stored: map[string]bool{}}
	e, token := newTagRouteServer(t, fx, 3, 4)

	refused := 0
	for i := 0; i < 6; i++ {
		rec := getTags(e, token, uuid.NewString())
		if rec.Code == http.StatusTooManyRequests {
			refused++
			assert.NotEmpty(t, rec.Header().Get("Retry-After"))
		} else {
			assert.Equal(t, http.StatusOK, rec.Code)
		}
	}
	assert.Equal(t, int32(3), fx.generations.Load())
	assert.Equal(t, 3, refused)
}

func TestArticleTagsRoute_StoredReadsConsumeNothing(t *testing.T) {
	fx := &restTagFixture{stored: map[string]bool{}}
	e, token := newTagRouteServer(t, fx, 1, 1)

	for i := 0; i < 30; i++ {
		id := uuid.NewString()
		fx.stored[id] = true
		assert.Equal(t, http.StatusOK, getTags(e, token, id).Code)
	}
	assert.Equal(t, int32(0), fx.generations.Load())
	assert.Equal(t, http.StatusOK, getTags(e, token, uuid.NewString()).Code)
	assert.Equal(t, int32(1), fx.generations.Load())
}

func TestArticleTagsRoute_SlotIsReleasedAfterGeneratorFailure(t *testing.T) {
	fx := &restTagFixture{stored: map[string]bool{}, failTagger: true}
	e, token := newTagRouteServer(t, fx, 10, 1)

	for i := 0; i < 5; i++ {
		assert.Equal(t, http.StatusOK, getTags(e, token, uuid.NewString()).Code, "call %d", i)
	}
}
