package v2

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/connect/v2/middleware"
	"alt/gen/proto/alt/articles/v2/articlesv2connect"
	"alt/gen/proto/alt/augur/v2/augurv2connect"
	"alt/gen/proto/alt/morning_letter/v2/morningletterv2connect"
)

func TestSetupConnectHandlers_WiresUserQuotaLimiterBeforeUsecase(t *testing.T) {
	container, cfg, logger := testDeps()
	limiter, err := middleware.NewUserQuotaLimiter(withDefaultRetrieveAndTagClasses(config.UserQuotaConfig{
		MaxTrackedUsers: 16,
		Summarize:       config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Minute, MaxInFlight: 1},
		AugurChat:       config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Minute, MaxInFlight: 1},
		MorningChat:     config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Minute, MaxInFlight: 1},
	}), time.Now, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	container.UserQuotaLimiter = limiter

	mux := http.NewServeMux()
	SetupConnectHandlers(mux, container, cfg, logger)
	token := issueTestBackendToken(t)

	// 1st call consumes the single token (reaching the nil MorningLetterUsecase and panicking).
	func() {
		defer func() { _ = recover() }()
		req1 := httptest.NewRequest(http.MethodPost, morningletterv2connect.MorningLetterServiceStreamChatProcedure, strings.NewReader("\x00\x00\x00\x00\x02{}"))
		req1.Header.Set("Content-Type", "application/connect+json")
		req1.Header.Set("X-Alt-Backend-Token", token)
		mux.ServeHTTP(httptest.NewRecorder(), req1)
	}()

	// 2nd call for the same user must be rejected by the quota interceptor (resource_exhausted end-of-stream error)
	// before reaching MorningLetterUsecase (no panic).
	req2 := httptest.NewRequest(http.MethodPost, morningletterv2connect.MorningLetterServiceStreamChatProcedure, strings.NewReader("\x00\x00\x00\x00\x02{}"))
	req2.Header.Set("Content-Type", "application/connect+json")
	req2.Header.Set("X-Alt-Backend-Token", token)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	assert.Contains(t, rec2.Body.String(), "resource_exhausted")
}

func withDefaultRetrieveAndTagClasses(c config.UserQuotaConfig) config.UserQuotaConfig {
	d := config.DefaultUserQuotaConfig()
	c.AugurRetrieve = d.AugurRetrieve
	c.ArticleTags = d.ArticleTags
	return c
}

func TestSetupConnectHandlers_GatesRetrieveContextAndArticleTags(t *testing.T) {
	container, cfg, logger := testDeps()
	quota := config.DefaultUserQuotaConfig()
	quota.AugurRetrieve = config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Minute, MaxInFlight: 1}
	quota.ArticleTags = config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Minute, MaxInFlight: 1}
	limiter, err := middleware.NewUserQuotaLimiter(quota, time.Now, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	container.UserQuotaLimiter = limiter

	mux := http.NewServeMux()
	SetupConnectHandlers(mux, container, cfg, logger)
	token := issueTestBackendToken(t)

	t.Run("RetrieveContext", func(t *testing.T) {
		call := func() *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodPost, augurv2connect.AugurServiceRetrieveContextProcedure, strings.NewReader(`{"query":"ai"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alt-Backend-Token", token)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			return rec
		}
		func() {
			defer func() { _ = recover() }()
			call()
		}()
		rec := call()
		assert.Equal(t, http.StatusTooManyRequests, rec.Code)
		assert.NotEmpty(t, rec.Header().Get("Retry-After"))
	})

	t.Run("StreamArticleTags", func(t *testing.T) {
		call := func() *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodPost, articlesv2connect.ArticleServiceStreamArticleTagsProcedure,
				strings.NewReader("\x00\x00\x00\x00\x34{\"articleId\":\"11111111-1111-4111-8111-111111111111\"}"))
			req.Header.Set("Content-Type", "application/connect+json")
			req.Header.Set("X-Alt-Backend-Token", token)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			return rec
		}
		first := call()
		assert.NotContains(t, first.Body.String(), "resource_exhausted")
		second := call()
		assert.Contains(t, second.Body.String(), "resource_exhausted")
	})
}
