package rest

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
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
	"alt/domain"
	"alt/orchestrator/port/preprocessor_summarize_port"
	"alt/orchestrator/port/rag_integration_port"
	"alt/orchestrator/rest/rest_feeds"
	"alt/orchestrator/usecase/fetch_article_summaries_usecase"
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

func newRESTTestKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return pub, priv
}

func signRESTTestJWT(t *testing.T, priv ed25519.PrivateKey, uid uuid.UUID) string {
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
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	signed, err := tok.SignedString(priv)
	require.NoError(t, err)
	return signed
}

func TestRESTRoutes_EnforceUserQuotaWith429AndRetryAfter(t *testing.T) {
	logger.InitLogger()
	pub, priv := newRESTTestKeyPair(t)
	limiter, err := connectmiddleware.NewUserQuotaLimiter(config.UserQuotaConfig{
		MaxTrackedUsers: 32,
		Summarize:       config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Minute, MaxInFlight: 1},
		AugurChat:       config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Minute, MaxInFlight: 1},
		MorningChat:     config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Minute, MaxInFlight: 1},
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
			BackendTokenPublicKey: pub,
			BackendTokenIssuer:    "auth-hub",
			BackendTokenAudience:  "alt-backend",
		},
	}

	e := echo.New()
	v1 := e.Group("/v1")
	RegisterAugurRoutes(e, v1, container, cfg.Auth)
	rest_feeds.RegisterFeedRoutes(v1, container, cfg)

	uid := uuid.New()
	token := signRESTTestJWT(t, priv, uid)
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

type stubQuotaSummarizeRepo struct {
	savedSummaryIDs []string
}

func (s *stubQuotaSummarizeRepo) FetchArticleByURL(_ context.Context, articleURL string) (*domain.ArticleContent, error) {
	return &domain.ArticleContent{
		ID:      "33333333-3333-4333-8333-333333333333",
		Title:   "Stored Title",
		Content: "Stored article body for " + articleURL,
	}, nil
}

func (s *stubQuotaSummarizeRepo) FetchArticleByID(_ context.Context, articleID string) (*domain.ArticleContent, error) {
	return &domain.ArticleContent{
		ID:      articleID,
		Title:   "Stored Title",
		Content: "Stored article body",
	}, nil
}

func (s *stubQuotaSummarizeRepo) FetchArticlesByURLs(_ context.Context, urls []string) (map[string]*domain.ArticleContent, error) {
	out := make(map[string]*domain.ArticleContent, len(urls))
	for _, u := range urls {
		out[u] = &domain.ArticleContent{
			ID:      "33333333-3333-4333-8333-333333333333",
			Title:   "Stored Title",
			Content: "Stored article body for " + u,
		}
	}
	return out, nil
}

func (s *stubQuotaSummarizeRepo) SaveArticle(_ context.Context, _, _, _ string) (string, error) {
	return "33333333-3333-4333-8333-333333333333", nil
}

func (s *stubQuotaSummarizeRepo) SaveArticleSummary(_ context.Context, articleID, _, _, _ string) error {
	s.savedSummaryIDs = append(s.savedSummaryIDs, articleID)
	return nil
}

func (s *stubQuotaSummarizeRepo) FetchArticleSummaryByArticleID(_ context.Context, _ string) (*domain.FeedSummary, error) {
	return nil, nil
}

type stubQuotaPreProcessor struct {
	summarizeCalls int
	streamCalls    int
	queueCalls     int
}

func (s *stubQuotaPreProcessor) Summarize(_ context.Context, _, _, _ string) (string, error) {
	s.summarizeCalls++
	return "generated summary", nil
}

func (s *stubQuotaPreProcessor) StreamSummarize(_ context.Context, _, _, _ string) (io.ReadCloser, error) {
	s.streamCalls++
	return io.NopCloser(strings.NewReader("data: streamed summary\n\n")), nil
}

func (s *stubQuotaPreProcessor) QueueSummarize(_ context.Context, _, _ string) (string, error) {
	s.queueCalls++
	return "job-quota-1", nil
}

func (s *stubQuotaPreProcessor) GetSummarizeStatus(_ context.Context, _ string) (*preprocessor_summarize_port.SummarizeStatus, error) {
	return nil, nil
}

type stubQuotaBatchSummarizer struct {
	calls int
}

func (s *stubQuotaBatchSummarizer) EnsureSummary(_ context.Context, _, _, _ string) (string, bool, error) {
	s.calls++
	return "batch summary", false, nil
}

func newSummarizeQuotaTestServer(t *testing.T, pub ed25519.PublicKey, limiter *connectmiddleware.UserQuotaLimiter) (*echo.Echo, *stubQuotaPreProcessor, *stubQuotaBatchSummarizer) {
	t.Helper()
	repo := &stubQuotaSummarizeRepo{}
	preproc := &stubQuotaPreProcessor{}
	batchSummarizer := &stubQuotaBatchSummarizer{}
	summarizeUC := summarize_article_usecase.NewUsecase(repo, preproc, nil)
	batchSummariesUC := fetch_article_summaries_usecase.NewUsecase(repo, nil, batchSummarizer)

	container := &di.ApplicationComponents{
		UserQuotaLimiter:             limiter,
		SummarizeArticleUsecase:      summarizeUC,
		FetchArticleSummariesUsecase: batchSummariesUC,
	}
	cfg := &config.Config{
		Auth: config.AuthConfig{
			BackendTokenPublicKey: pub,
			BackendTokenIssuer:    "auth-hub",
			BackendTokenAudience:  "alt-backend",
		},
	}

	e := echo.New()
	v1 := e.Group("/v1")
	rest_feeds.RegisterFeedRoutes(v1, container, cfg)
	return e, preproc, batchSummarizer
}

func TestNonStreamingSummarizeRoutes_EnforcePerUserQuotaAndIsolateUsers(t *testing.T) {
	logger.InitLogger()
	t.Setenv("FEED_ALLOWED_HOSTS", "example.com")
	pub, priv := newRESTTestKeyPair(t)

	cases := []struct {
		name       string
		path       string
		body       string
		okStatus   int
		countCalls func(*stubQuotaPreProcessor, *stubQuotaBatchSummarizer) int
	}{
		{
			name:     "POST /v1/feeds/summarize",
			path:     "/v1/feeds/summarize",
			body:     `{"feed_url":"https://example.com/a"}`,
			okStatus: http.StatusOK,
			countCalls: func(p *stubQuotaPreProcessor, _ *stubQuotaBatchSummarizer) int {
				return p.summarizeCalls
			},
		},
		{
			name:     "POST /v1/feeds/summarize/queue",
			path:     "/v1/feeds/summarize/queue",
			body:     `{"feed_url":"https://example.com/a"}`,
			okStatus: http.StatusAccepted,
			countCalls: func(p *stubQuotaPreProcessor, _ *stubQuotaBatchSummarizer) int {
				return p.queueCalls
			},
		},
		{
			name:     "POST /v1/feeds/fetch/summary",
			path:     "/v1/feeds/fetch/summary",
			body:     `{"feed_urls":["https://example.com/a"]}`,
			okStatus: http.StatusOK,
			countCalls: func(_ *stubQuotaPreProcessor, b *stubQuotaBatchSummarizer) int {
				return b.calls
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			limiter, err := connectmiddleware.NewUserQuotaLimiter(config.UserQuotaConfig{
				MaxTrackedUsers: 32,
				Summarize:       config.OperationQuotaConfig{Rate: 2, Burst: 2, Window: time.Hour, MaxInFlight: 2},
				AugurChat:       config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Minute, MaxInFlight: 1},
				MorningChat:     config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Minute, MaxInFlight: 1},
			})
			require.NoError(t, err)

			e, preproc, batchSummarizer := newSummarizeQuotaTestServer(t, pub, limiter)
			userAToken := signRESTTestJWT(t, priv, uuid.New())
			userBToken := signRESTTestJWT(t, priv, uuid.New())

			doCall := func(tok string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
				req.Header.Set("X-Alt-Backend-Token", tok)
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)
				return rec
			}

			rec1 := doCall(userAToken)
			assert.Equal(t, tc.okStatus, rec1.Code)
			rec2 := doCall(userAToken)
			assert.Equal(t, tc.okStatus, rec2.Code)
			assert.Equal(t, 2, tc.countCalls(preproc, batchSummarizer))

			rec3 := doCall(userAToken)
			assert.Equal(t, http.StatusTooManyRequests, rec3.Code)
			assert.NotEmpty(t, rec3.Header().Get("Retry-After"))
			assert.Contains(t, rec3.Body.String(), "rate limit exceeded")
			assert.Equal(t, 2, tc.countCalls(preproc, batchSummarizer))

			recOtherUser := doCall(userBToken)
			assert.Equal(t, tc.okStatus, recOtherUser.Code)
			assert.Equal(t, 3, tc.countCalls(preproc, batchSummarizer))
		})
	}
}

func TestSummarizeRoutes_ShareSinglePerUserBudgetAcrossStreamingAndNonStreaming(t *testing.T) {
	logger.InitLogger()
	t.Setenv("FEED_ALLOWED_HOSTS", "example.com")
	pub, priv := newRESTTestKeyPair(t)

	limiter, err := connectmiddleware.NewUserQuotaLimiter(config.UserQuotaConfig{
		MaxTrackedUsers: 32,
		Summarize:       config.OperationQuotaConfig{Rate: 2, Burst: 2, Window: time.Hour, MaxInFlight: 2},
		AugurChat:       config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Minute, MaxInFlight: 1},
		MorningChat:     config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Minute, MaxInFlight: 1},
	})
	require.NoError(t, err)

	e, preproc, batchSummarizer := newSummarizeQuotaTestServer(t, pub, limiter)
	userAToken := signRESTTestJWT(t, priv, uuid.New())
	userBToken := signRESTTestJWT(t, priv, uuid.New())

	doPost := func(path, body, tok string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set("X-Alt-Backend-Token", tok)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	streamRec := doPost("/v1/feeds/summarize/stream", `{"feed_url":"https://example.com/a","content":"body","title":"T","force_refresh":true}`, userAToken)
	assert.Equal(t, http.StatusOK, streamRec.Code)

	syncRec := doPost("/v1/feeds/summarize", `{"feed_url":"https://example.com/a"}`, userAToken)
	assert.Equal(t, http.StatusOK, syncRec.Code)

	queueRec := doPost("/v1/feeds/summarize/queue", `{"feed_url":"https://example.com/a"}`, userAToken)
	assert.Equal(t, http.StatusTooManyRequests, queueRec.Code)
	assert.NotEmpty(t, queueRec.Header().Get("Retry-After"))
	assert.Equal(t, 0, preproc.queueCalls)

	batchRec := doPost("/v1/feeds/fetch/summary", `{"feed_urls":["https://example.com/a"]}`, userAToken)
	assert.Equal(t, http.StatusTooManyRequests, batchRec.Code)
	assert.NotEmpty(t, batchRec.Header().Get("Retry-After"))
	assert.Equal(t, 0, batchSummarizer.calls)

	otherUserQueueRec := doPost("/v1/feeds/summarize/queue", `{"feed_url":"https://example.com/a"}`, userBToken)
	assert.Equal(t, http.StatusAccepted, otherUserQueueRec.Code)
	assert.Equal(t, 1, preproc.queueCalls)
}

func TestSummarizeRoutes_PanicWhenLimiterNil(t *testing.T) {
	logger.InitLogger()
	t.Setenv("FEED_ALLOWED_HOSTS", "example.com")
	pub, priv := newRESTTestKeyPair(t)

	e, _, _ := newSummarizeQuotaTestServer(t, pub, nil)
	tok := signRESTTestJWT(t, priv, uuid.New())

	for _, path := range []string{
		"/v1/feeds/summarize",
		"/v1/feeds/summarize/queue",
		"/v1/feeds/fetch/summary",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"feed_url":"https://example.com/a","feed_urls":["https://example.com/a"]}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			req.Header.Set("X-Alt-Backend-Token", tok)
			rec := httptest.NewRecorder()
			assert.Panics(t, func() {
				e.ServeHTTP(rec, req)
			})
		})
	}
}
