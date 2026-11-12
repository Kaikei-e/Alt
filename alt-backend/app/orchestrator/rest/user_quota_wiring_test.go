package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	connectmiddleware "alt/connect/v2/middleware"
	"alt/di"
	"alt/orchestrator/port/rag_integration_port"
	"alt/orchestrator/rest/rest_feeds"
	"alt/orchestrator/usecase/summarize_article_usecase"
	"alt/utils/logger"
)

type stubQuotaAnswerUsecase struct {
	calls int
}

func (s *stubQuotaAnswerUsecase) Execute(_ context.Context, _ rag_integration_port.AnswerInput) (<-chan string, error) {
	s.calls++
	ch := make(chan string, 1)
	ch <- "ok"
	close(ch)
	return ch, nil
}

func signRESTTestJWT(t *testing.T, secret string, uid uuid.UUID) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":       uid.String(),
		"tenant_id": uid.String(),
		"email":     "quota@example.com",
		"role":      "user",
		"sid":       "sess-1",
		"iss":       "auth-hub",
		"aud":       []string{"alt-backend"},
		"exp":       time.Now().Add(time.Hour).Unix(),
		"iat":       time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

func TestRESTRoutes_EnforceUserQuotaWith429AndRetryAfter(t *testing.T) {
	logger.InitLogger()
	const secret = "rest-quota-test-secret-at-least-32-bytes"
	limiter, err := connectmiddleware.NewUserQuotaLimiter(config.UserQuotaConfig{
		MaxTrackedUsers:   32,
		Summarize:         config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Minute, MaxConcurrent: 1},
		AugurChat:         config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Minute, MaxConcurrent: 1},
		MorningChat:       config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Minute, MaxConcurrent: 1},
		MorningRegenerate: config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Hour, MaxConcurrent: 1},
	})
	require.NoError(t, err)

	answerUC := &stubQuotaAnswerUsecase{}
	repo := &stubStreamArticleRepo{savedID: "22222222-2222-4222-8222-222222222222"}
	container := &di.ApplicationComponents{
		UserQuotaLimiter:        limiter,
		AnswerChatUsecase:       answerUC,
		SummarizeArticleUsecase: summarize_article_usecase.NewUsecase(repo, &stubStreamPreProcessor{}, nil),
	}
	cfg := &config.Config{
		Auth: config.AuthConfig{
			BackendTokenSecret:   secret,
			BackendTokenIssuer:   "auth-hub",
			BackendTokenAudience: "alt-backend",
		},
	}

	e := echo.New()
	v1 := e.Group("/v1")
	RegisterAugurRoutes(e, v1, container, cfg.Auth)
	rest_feeds.RegisterFeedRoutes(v1, container, cfg)

	uid := uuid.New()
	token := signRESTTestJWT(t, secret, uid)
	body := `{"messages":[{"role":"user","content":"hello"}],"stream":false}`

	req1 := httptest.NewRequest(http.MethodPost, "/sse/v1/rag/answer", strings.NewReader(body))
	req1.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req1.Header.Set("X-Alt-Backend-Token", token)
	rec1 := httptest.NewRecorder()
	e.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)
	assert.Equal(t, 1, answerUC.calls)

	req2 := httptest.NewRequest(http.MethodPost, "/sse/v1/rag/answer", strings.NewReader(body))
	req2.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req2.Header.Set("X-Alt-Backend-Token", token)
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
	assert.NotEmpty(t, rec2.Header().Get("Retry-After"))
	assert.Equal(t, 1, answerUC.calls)

	sumBody := `{"feed_url":"https://example.com/a","content":"body text","title":"T","force_refresh":true}`
	sReq1 := httptest.NewRequest(http.MethodPost, "/v1/feeds/summarize/stream", strings.NewReader(sumBody))
	sReq1.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	sReq1.Header.Set("X-Alt-Backend-Token", token)
	sRec1 := httptest.NewRecorder()
	e.ServeHTTP(sRec1, sReq1)
	assert.Equal(t, http.StatusOK, sRec1.Code)
	assert.Len(t, repo.savedSummaryIDs, 1)

	sReq2 := httptest.NewRequest(http.MethodPost, "/v1/feeds/summarize/stream", strings.NewReader(sumBody))
	sReq2.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	sReq2.Header.Set("X-Alt-Backend-Token", token)
	sRec2 := httptest.NewRecorder()
	e.ServeHTTP(sRec2, sReq2)
	assert.Equal(t, http.StatusTooManyRequests, sRec2.Code)
	assert.NotEmpty(t, sRec2.Header().Get("Retry-After"))
	assert.Len(t, repo.savedSummaryIDs, 1)
}
