package bootstrap

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	connectv2 "pre-processor/connect/v2"
	"pre-processor/domain"
	"pre-processor/handler"
	"pre-processor/test/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makePreProcessorCertRequest(method, path, cn string, body []byte) *http.Request {
	var bodyReader *bytes.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	} else {
		bodyReader = bytes.NewReader([]byte{})
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if cn != "" {
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				{Subject: pkix.Name{CommonName: cn}},
			},
		}
	}
	return req
}

func TestC02_HTTPServer_Plaintext_Anonymous_Rejection(t *testing.T) {
	t.Setenv("MTLS_ALLOWED_PEERS", "alt-backend")
	deps := newTestHTTPServer(t, "unit-test-secret")
	srv := NewHTTPServer(deps, false, "")

	// 1. Health endpoints must stay reachable anonymously
	for _, healthPath := range []string{"/api/v1/health", "/health"} {
		req := httptest.NewRequest(http.MethodGet, healthPath, nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "health endpoint %s must be reachable anonymously", healthPath)
	}

	// 2. Business REST endpoints must reject anonymous plaintext callers with 401
	businessPaths := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/summarize"},
		{http.MethodPost, "/api/v1/summarize/stream"},
		{http.MethodPost, "/api/v1/summarize/queue"},
		{http.MethodGet, "/api/v1/summarize/status/test-job-id"},
	}

	for _, bp := range businessPaths {
		req := httptest.NewRequest(bp.method, bp.path, strings.NewReader(`{"content":"test"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "business endpoint %s %s must reject anonymous plaintext request with 401", bp.method, bp.path)
	}
}

func TestC02_ConnectServer_Plaintext_Anonymous_Rejection(t *testing.T) {
	t.Setenv("MTLS_ALLOWED_PEERS", "alt-backend")
	deps := newTestHTTPServer(t, "unit-test-secret")
	connectHandler := connectv2.CreateConnectServer(deps.APIRepo, deps.SummaryRepo, deps.ArticleRepo, deps.JobRepo, deps.Logger)

	// 1. Health endpoint stays reachable
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	connectHandler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "Connect /health must stay reachable")

	// 2. Connect-RPC business procedure must reject anonymous plaintext callers with 401
	rpcReq := httptest.NewRequest(http.MethodPost, "/services.preprocessor.v2.PreProcessorService/Summarize", strings.NewReader(`{}`))
	rpcReq.Header.Set("Content-Type", "application/json")
	rpcRec := httptest.NewRecorder()
	connectHandler.ServeHTTP(rpcRec, rpcReq)
	assert.Equal(t, http.StatusUnauthorized, rpcRec.Code, "Connect RPC must reject anonymous plaintext caller with 401")
}

func TestC02_MTLSServer_SafeMux_PeerGated(t *testing.T) {
	t.Setenv("MTLS_ALLOWED_PEERS", "alt-backend")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
	mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
	mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
	mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	summarizeHandler := handler.NewSummarizeHandler(mockAPIRepo, mockSummaryRepo, mockArticleRepo, mockJobRepo, logger)
	deps := &Dependencies{
		APIRepo:          mockAPIRepo,
		SummaryRepo:      mockSummaryRepo,
		ArticleRepo:      mockArticleRepo,
		JobRepo:          mockJobRepo,
		SummarizeHandler: summarizeHandler,
		Logger:           logger,
	}

	httpServer := NewHTTPServer(deps, false, "")
	connectHandler := connectv2.CreateConnectServer(deps.APIRepo, deps.SummaryRepo, deps.ArticleRepo, deps.JobRepo, deps.Logger)

	allowed := []string{"alt-backend"}
	safeMux := NewMTLSMuxHandler(httpServer, connectHandler, allowed, deps.Logger)

	// 1. Unauthenticated health endpoints stay reachable on :9443 (deep health check removed - no fabricated health)
	for _, path := range []string{"/health", "/api/v1/health"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		safeMux.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "%s must be reachable unauthenticated on mTLS mux", path)
	}

	// 2. Anonymous request (no client cert) to REST and Connect -> 401
	restAnonReq := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", strings.NewReader(`{"content":"test"}`))
	restAnonReq.Header.Set("Content-Type", "application/json")
	restAnonRec := httptest.NewRecorder()
	safeMux.ServeHTTP(restAnonRec, restAnonReq)
	assert.Equal(t, http.StatusUnauthorized, restAnonRec.Code, "REST on mTLS mux without client cert must return 401")

	connectAnonReq := httptest.NewRequest(http.MethodPost, "/services.preprocessor.v2.PreProcessorService/Summarize", strings.NewReader(`{}`))
	connectAnonReq.Header.Set("Content-Type", "application/json")
	connectAnonRec := httptest.NewRecorder()
	safeMux.ServeHTTP(connectAnonRec, connectAnonReq)
	assert.Equal(t, http.StatusUnauthorized, connectAnonRec.Code, "Connect on mTLS mux without client cert must return 401")

	// 3. Unknown peer (CN="unauthorized-service") -> 403 Forbidden
	restForbiddenReq := makePreProcessorCertRequest(http.MethodPost, "/api/v1/summarize", "unauthorized-service", []byte(`{"content":"test"}`))
	restForbiddenReq.Header.Set("Content-Type", "application/json")
	restForbiddenRec := httptest.NewRecorder()
	safeMux.ServeHTTP(restForbiddenRec, restForbiddenReq)
	assert.Equal(t, http.StatusForbidden, restForbiddenRec.Code, "REST on mTLS mux with unauthorized CN must return 403")

	connectForbiddenReq := makePreProcessorCertRequest(http.MethodPost, "/services.preprocessor.v2.PreProcessorService/Summarize", "unauthorized-service", []byte(`{}`))
	connectForbiddenReq.Header.Set("Content-Type", "application/json")
	connectForbiddenRec := httptest.NewRecorder()
	safeMux.ServeHTTP(connectForbiddenRec, connectForbiddenReq)
	assert.Equal(t, http.StatusForbidden, connectForbiddenRec.Code, "Connect on mTLS mux with unauthorized CN must return 403")

	// 4. Allowed peer (CN="alt-backend") passes peer gate and executes actual usecase returning 200 OK
	mockArticleRepo.EXPECT().
		FindByID(gomock.Any(), "test-rest-article").
		Return(&domain.Article{
			ID:      "test-rest-article",
			UserID:  "user-1",
			Content: "This is test content",
			Title:   "Test Title",
		}, nil).
		Times(1)
	mockAPIRepo.EXPECT().
		SummarizeArticle(gomock.Any(), gomock.Any(), "high").
		Return(&domain.SummarizedContent{
			ArticleID:       "test-rest-article",
			SummaryJapanese: "REST要約成功",
		}, nil).
		Times(1)
	mockSummaryRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	restAllowedReq := makePreProcessorCertRequest(http.MethodPost, "/api/v1/summarize", "alt-backend", []byte(`{"article_id":"test-rest-article"}`))
	restAllowedReq.Header.Set("Content-Type", "application/json")
	restAllowedRec := httptest.NewRecorder()
	safeMux.ServeHTTP(restAllowedRec, restAllowedReq)
	assert.Equal(t, http.StatusOK, restAllowedRec.Code, "REST with allowed CN must succeed with 200 OK")
	assert.Contains(t, restAllowedRec.Body.String(), "REST要約成功", "REST response must contain usecase summary")

	mockArticleRepo.EXPECT().
		FindByID(gomock.Any(), "test-connect-article").
		Return(&domain.Article{
			ID:      "test-connect-article",
			UserID:  "user-1",
			Content: "This is test content",
			Title:   "Test Title",
		}, nil).
		Times(1)
	mockAPIRepo.EXPECT().
		SummarizeArticle(gomock.Any(), gomock.Any(), "high").
		Return(&domain.SummarizedContent{
			ArticleID:       "test-connect-article",
			SummaryJapanese: "Connect要約成功",
		}, nil).
		Times(1)
	mockSummaryRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	connectAllowedReq := makePreProcessorCertRequest(http.MethodPost, "/services.preprocessor.v2.PreProcessorService/Summarize", "alt-backend", []byte(`{"article_id":"test-connect-article"}`))
	connectAllowedReq.Header.Set("Content-Type", "application/json")
	connectAllowedRec := httptest.NewRecorder()
	safeMux.ServeHTTP(connectAllowedRec, connectAllowedReq)
	assert.Equal(t, http.StatusOK, connectAllowedRec.Code, "Connect with allowed CN must succeed with 200 OK")
	assert.Contains(t, connectAllowedRec.Body.String(), "Connect要約成功", "Connect response must contain usecase summary")

	// 5. 1MiB body limit enforced on mTLS mux REST business route
	oversized := bytes.Repeat([]byte("a"), 2*1024*1024) // 2 MiB
	oversizedReq := makePreProcessorCertRequest(http.MethodPost, "/api/v1/summarize", "alt-backend", oversized)
	oversizedReq.Header.Set("Content-Type", "application/octet-stream")
	oversizedRec := httptest.NewRecorder()
	safeMux.ServeHTTP(oversizedRec, oversizedReq)
	require.Equal(t, http.StatusRequestEntityTooLarge, oversizedRec.Code, "oversized body on mTLS mux must be rejected with 413")
}
