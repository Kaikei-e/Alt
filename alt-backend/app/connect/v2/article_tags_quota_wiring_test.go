package v2

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/connect/v2/middleware"
	"alt/domain"
	"alt/gen/proto/alt/articles/v2/articlesv2connect"
	"alt/orchestrator/gateway/fetch_article_tags_gateway"
	"alt/orchestrator/usecase/fetch_article_tags_usecase"
	"alt/orchestrator/usecase/fetch_latest_article_usecase"
	"alt/orchestrator/usecase/fetch_random_subscription_usecase"
	"alt/orchestrator/usecase/stream_article_tags_usecase"
	"alt/shared/driver/mqhub_connect"
)

type tagFixture struct {
	stored      map[string]bool
	generations atomic.Int32
	failTagger  bool
}

func (f *tagFixture) FetchArticleTags(_ context.Context, id string) ([]*domain.FeedTag, error) {
	if f.stored[id] {
		return []*domain.FeedTag{{ID: "t-" + id, TagName: "stored"}}, nil
	}
	return []*domain.FeedTag{}, nil
}

func (f *tagFixture) FetchCachedArticleTags(ctx context.Context, id string) ([]*domain.FeedTag, error) {
	return f.FetchArticleTags(ctx, id)
}

func (f *tagFixture) UpsertArticleTags(context.Context, string, string, []domain.TagUpsert) (int32, error) {
	return 1, nil
}

func (f *tagFixture) FetchArticleByID(_ context.Context, id string) (*domain.ArticleContent, error) {
	return &domain.ArticleContent{ID: id, Title: "t", Content: "c", FeedID: uuid.NewString()}, nil
}

func (f *tagFixture) IsEnabled() bool { return true }

func (f *tagFixture) GenerateTagsForArticle(context.Context, mqhub_connect.GenerateTagsRequest) (*mqhub_connect.GenerateTagsResponse, error) {
	f.generations.Add(1)
	if f.failTagger {
		return nil, fmt.Errorf("mq-hub unavailable")
	}
	return &mqhub_connect.GenerateTagsResponse{
		Success: true,
		Tags:    []mqhub_connect.GeneratedTag{{ID: "g", Name: "generated", Confidence: 0.9}},
	}, nil
}

type randomFeedFixture struct{}

func (r *randomFeedFixture) FetchRandomSubscription(context.Context) (*domain.Feed, error) {
	return &domain.Feed{ID: uuid.New(), Title: "feed", WebsiteURL: "https://example.com"}, nil
}

func (r *randomFeedFixture) FetchLatestArticleByFeedID(context.Context, uuid.UUID) (*domain.ArticleContent, error) {
	return &domain.ArticleContent{ID: uuid.NewString(), Title: "latest"}, nil
}

func newTagWiredMux(t *testing.T, fx *tagFixture, budget, inFlight int) *http.ServeMux {
	t.Helper()
	container, cfg, logger := testDeps()
	quota := config.DefaultUserQuotaConfig()
	quota.ArticleTags = config.OperationQuotaConfig{Rate: 1, Burst: budget, Window: time.Hour, MaxInFlight: inFlight}
	limiter, err := middleware.NewUserQuotaLimiter(quota, time.Now, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	gcfg := fetch_article_tags_gateway.DefaultConfig()
	gcfg.RetryBackoff = time.Millisecond
	gw := fetch_article_tags_gateway.NewFetchArticleTagsGateway(fx, fx, fx, gcfg, limiter)
	rf := &randomFeedFixture{}

	container.UserQuotaLimiter = limiter
	container.FetchArticleTagsUsecase = fetch_article_tags_usecase.NewFetchArticleTagsUsecase(gw)
	container.StreamArticleTagsUsecase = stream_article_tags_usecase.NewStreamArticleTagsUsecase(fx, gw)
	container.FetchRandomSubscriptionUsecase = fetch_random_subscription_usecase.NewFetchRandomSubscriptionUsecase(rf)
	container.FetchLatestArticleUsecase = fetch_latest_article_usecase.NewFetchLatestArticleUsecase(rf)

	mux := http.NewServeMux()
	SetupConnectHandlers(mux, container, cfg, logger)
	return mux
}

func postUnary(mux *http.ServeMux, token, procedure, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, procedure, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func postStream(mux *http.ServeMux, token, procedure, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, procedure,
		strings.NewReader(fmt.Sprintf("\x00\x00\x00\x00%c%s", rune(len(body)), body)))
	req.Header.Set("Content-Type", "application/connect+json")
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func articleBody() string { return fmt.Sprintf(`{"articleId":%q}`, uuid.NewString()) }

func TestArticleTagGeneration_FetchArticleTagsConnectIsBoundedByBudget(t *testing.T) {
	fx := &tagFixture{stored: map[string]bool{}}
	mux := newTagWiredMux(t, fx, 3, 4)
	token := issueTestBackendToken(t)

	refused := 0
	for i := 0; i < 6; i++ {
		rec := postUnary(mux, token, articlesv2connect.ArticleServiceFetchArticleTagsProcedure, articleBody())
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

func TestArticleTagGeneration_StreamArticleTagsIsBoundedByBudget(t *testing.T) {
	fx := &tagFixture{stored: map[string]bool{}}
	mux := newTagWiredMux(t, fx, 3, 4)
	token := issueTestBackendToken(t)

	refused := 0
	for i := 0; i < 6; i++ {
		rec := postStream(mux, token, articlesv2connect.ArticleServiceStreamArticleTagsProcedure, articleBody())
		if strings.Contains(rec.Body.String(), "resource_exhausted") {
			refused++
		}
	}
	assert.Equal(t, int32(3), fx.generations.Load())
	assert.Equal(t, 3, refused)
}

func TestArticleTagGeneration_FetchRandomFeedFallsBackToStoredTags(t *testing.T) {
	fx := &tagFixture{stored: map[string]bool{}}
	mux := newTagWiredMux(t, fx, 3, 4)
	token := issueTestBackendToken(t)

	withGenerated := 0
	for i := 0; i < 6; i++ {
		rec := postUnary(mux, token, articlesv2connect.ArticleServiceFetchRandomFeedProcedure, `{}`)
		require.Equal(t, http.StatusOK, rec.Code, "a refused generation must not fail the feed")
		if strings.Contains(rec.Body.String(), "generated") {
			withGenerated++
		}
	}
	assert.Equal(t, int32(3), fx.generations.Load())
	assert.Equal(t, 3, withGenerated)
}

func TestArticleTagGeneration_StoredReadsConsumeNothing(t *testing.T) {
	fx := &tagFixture{stored: map[string]bool{}}
	mux := newTagWiredMux(t, fx, 1, 1)
	token := issueTestBackendToken(t)

	for i := 0; i < 30; i++ {
		id := uuid.NewString()
		fx.stored[id] = true
		body := fmt.Sprintf(`{"articleId":%q}`, id)
		stream := postStream(mux, token, articlesv2connect.ArticleServiceStreamArticleTagsProcedure, body)
		assert.NotContains(t, stream.Body.String(), "resource_exhausted")
		assert.Contains(t, stream.Body.String(), "stored")
		unary := postUnary(mux, token, articlesv2connect.ArticleServiceFetchArticleTagsProcedure, body)
		assert.Equal(t, http.StatusOK, unary.Code)
	}
	assert.Equal(t, int32(0), fx.generations.Load())

	// The single token is still there for a real miss.
	rec := postUnary(mux, token, articlesv2connect.ArticleServiceFetchArticleTagsProcedure, articleBody())
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, int32(1), fx.generations.Load())
}

func TestArticleTagGeneration_SlotIsReleasedAfterGeneratorFailure(t *testing.T) {
	fx := &tagFixture{stored: map[string]bool{}, failTagger: true}
	mux := newTagWiredMux(t, fx, 10, 1)
	token := issueTestBackendToken(t)

	for i := 0; i < 5; i++ {
		rec := postUnary(mux, token, articlesv2connect.ArticleServiceFetchArticleTagsProcedure, articleBody())
		assert.Equal(t, http.StatusOK, rec.Code, "call %d: a failed generation must free the single slot", i)
	}
}
