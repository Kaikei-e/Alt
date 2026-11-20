package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"rag-orchestrator/internal/di"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/infra/config"
	peermw "rag-orchestrator/internal/middleware"
	"rag-orchestrator/internal/usecase"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubSimilarityUC struct{}

func (s *stubSimilarityUC) Execute(_ context.Context, _ usecase.CandidateSimilarityInput) (*usecase.CandidateSimilarityOutput, error) {
	return &usecase.CandidateSimilarityOutput{
		SeedIndexed: true,
		Results: []domain.CandidateSimilarityItem{
			{ArticleID: "cand-1", Similarity: 0.88},
		},
	}, nil
}

func TestRESTServer_CandidateSimilarity_PeerIdentityAndAuthWiring(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	peerMW := peermw.NewPeerIdentityMiddleware([]string{"alt-backend", "alt-harvester"}, log)
	apiAuthMW := peermw.NewAPIAuthMiddleware("test-rag-api-token-minimum-24-characters", true, log)

	app := &di.ApplicationComponents{
		CandidateSimilarityUsecase: &stubSimilarityUC{},
	}
	e := newRESTEcho(app, &config.Config{}, peerMW, apiAuthMW, nil, log)

	makeReq := func() *http.Request {
		body := bytes.NewBufferString(`{"seed_article_id":"seed-1","candidate_article_ids":["cand-1"],"representation":"chunk_mean","user_id":"00000000-0000-0000-0000-000000000001"}`)
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/internal/rag/candidate-similarity", body)
		req.Header.Set("Content-Type", "application/json")
		return req
	}

	t.Run("missing client cert returns 401", func(t *testing.T) {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, makeReq())
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("disallowed peer CN returns 403", func(t *testing.T) {
		req := makeReq()
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{{Subject: pkix.Name{CommonName: "evil-peer"}}},
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("allowed peer without bearer token returns 401", func(t *testing.T) {
		req := makeReq()
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{{Subject: pkix.Name{CommonName: "alt-backend"}}},
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("allowed peer with valid bearer token succeeds", func(t *testing.T) {
		req := makeReq()
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{{Subject: pkix.Name{CommonName: "alt-backend"}}},
		}
		req.Header.Set("Authorization", "Bearer test-rag-api-token-minimum-24-characters")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"seed_indexed":true`)
		assert.Contains(t, rec.Body.String(), `"cand-1"`)
	})
}

func TestRESTServer_IPExtractor_IgnoresSpoofedHeaders(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	e := newRESTEcho(&di.ApplicationComponents{}, &config.Config{}, nil, nil, nil, log)
	require.NotNil(t, e.IPExtractor)

	var resolvedIP string
	e.GET("/test-realip", func(c *echo.Context) error {
		resolvedIP = c.RealIP()
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/test-realip", nil)
	req.RemoteAddr = "203.0.113.42:54321"
	req.Header.Set("X-Forwarded-For", "127.0.0.1, 10.0.0.1")
	req.Header.Set("X-Real-Ip", "10.0.0.2")

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "203.0.113.42", resolvedIP)
	assert.Equal(t, "203.0.113.42", e.IPExtractor(req))
}

type capturedRecord struct {
	level   slog.Level
	message string
	attrs   map[string]slog.Value
}

type capturingSlogHandler struct {
	mu      sync.Mutex
	records []capturedRecord
}

func (h *capturingSlogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *capturingSlogHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]slog.Value, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value
		return true
	})
	h.mu.Lock()
	h.records = append(h.records, capturedRecord{
		level:   r.Level,
		message: r.Message,
		attrs:   attrs,
	})
	h.mu.Unlock()
	return nil
}

func (h *capturingSlogHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *capturingSlogHandler) WithGroup(_ string) slog.Handler      { return h }

func TestRESTServer_RequestLogger_PreservesV4FieldsAndUsesLogger(t *testing.T) {
	capHandler := &capturingSlogHandler{}
	log := slog.New(capHandler)
	e := newRESTEcho(&di.ApplicationComponents{}, &config.Config{}, nil, nil, nil, log)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil)
	req.Host = "rag-orchestrator.local"
	req.RemoteAddr = "198.51.100.7:43210"
	req.Header.Set("User-Agent", "alt-test-client/1.0")
	req.Header.Set("X-Request-Id", "req-logger-check-1")
	req.Header.Set("Content-Length", "0")
	req.RequestURI = "/healthz"

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, capHandler.records, 1)

	got := capHandler.records[0]
	assert.Equal(t, slog.LevelInfo, got.level)
	assert.Equal(t, "REQUEST", got.message)

	expectedKeys := []string{
		"method",
		"uri",
		"status",
		"latency",
		"host",
		"bytes_in",
		"bytes_out",
		"user_agent",
		"remote_ip",
		"request_id",
	}
	for _, k := range expectedKeys {
		assert.Contains(t, got.attrs, k, "missing request log attribute %q", k)
	}
	assert.Equal(t, "GET", got.attrs["method"].String())
	assert.Equal(t, "/healthz", got.attrs["uri"].String())
	assert.Equal(t, int64(http.StatusOK), got.attrs["status"].Int64())
	assert.Equal(t, "rag-orchestrator.local", got.attrs["host"].String())
	assert.Equal(t, "alt-test-client/1.0", got.attrs["user_agent"].String())
	assert.Equal(t, "198.51.100.7", got.attrs["remote_ip"].String())
	assert.Equal(t, "req-logger-check-1", got.attrs["request_id"].String())
}

func TestRESTServer_RequestLogger_HandlesErrorStatusAndSingleWrite(t *testing.T) {
	capHandler := &capturingSlogHandler{}
	log := slog.New(capHandler)
	e := newRESTEcho(&di.ApplicationComponents{}, &config.Config{}, nil, nil, nil, log)
	e.GET("/test-err", func(c *echo.Context) error {
		return echo.NewHTTPError(http.StatusBadRequest, "bad request")
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/test-err", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Len(t, capHandler.records, 1)
	got := capHandler.records[0]
	assert.Equal(t, slog.LevelError, got.level)
	assert.Equal(t, "REQUEST_ERROR", got.message)
	assert.Equal(t, int64(http.StatusBadRequest), got.attrs["status"].Int64())
}
