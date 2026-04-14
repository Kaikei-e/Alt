package bootstrap

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pre-processor/handler"

	"github.com/stretchr/testify/require"
)

func newTestHTTPServer(t *testing.T, serviceSecret string) *Dependencies {
	t.Helper()
	t.Setenv("SERVICE_SECRET", serviceSecret)
	t.Setenv("SERVICE_SECRET_FILE", "")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Dependencies{
		SummarizeHandler: handler.NewSummarizeHandler(nil, nil, nil, nil, logger),
		Logger:           logger,
	}
}

func TestHTTPServer_SummarizeRequiresServiceToken(t *testing.T) {
	deps := newTestHTTPServer(t, "unit-test-secret")
	srv := NewHTTPServer(deps, false, "")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	srv.ServeHTTP(res, req)

	require.Equal(t, http.StatusUnauthorized, res.Code, "summarize must reject requests without X-Service-Token")
}

func TestHTTPServer_SummarizeStreamRequiresServiceToken(t *testing.T) {
	deps := newTestHTTPServer(t, "unit-test-secret")
	srv := NewHTTPServer(deps, false, "")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize/stream", strings.NewReader(`{}`))
	res := httptest.NewRecorder()

	srv.ServeHTTP(res, req)

	require.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestHTTPServer_SummarizeQueueRequiresServiceToken(t *testing.T) {
	deps := newTestHTTPServer(t, "unit-test-secret")
	srv := NewHTTPServer(deps, false, "")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize/queue", strings.NewReader(`{}`))
	res := httptest.NewRecorder()

	srv.ServeHTTP(res, req)

	require.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestHTTPServer_SummarizeStatusRequiresServiceToken(t *testing.T) {
	deps := newTestHTTPServer(t, "unit-test-secret")
	srv := NewHTTPServer(deps, false, "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/summarize/status/abc", nil)
	res := httptest.NewRecorder()

	srv.ServeHTTP(res, req)

	require.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestHTTPServer_HealthIsPublic(t *testing.T) {
	deps := newTestHTTPServer(t, "unit-test-secret")
	srv := NewHTTPServer(deps, false, "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	res := httptest.NewRecorder()

	srv.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code, "health must stay accessible without credentials")
}

func TestHTTPServer_WrongTokenRejected(t *testing.T) {
	deps := newTestHTTPServer(t, "unit-test-secret")
	srv := NewHTTPServer(deps, false, "")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", strings.NewReader(`{}`))
	req.Header.Set("X-Service-Token", "not-the-secret")
	res := httptest.NewRecorder()

	srv.ServeHTTP(res, req)

	require.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestHTTPServer_CorrectTokenPassesAuth(t *testing.T) {
	deps := newTestHTTPServer(t, "unit-test-secret")
	srv := NewHTTPServer(deps, false, "")

	// Empty body so handler rejects at validation (before any nil repo call).
	req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Token", "unit-test-secret")
	res := httptest.NewRecorder()

	srv.ServeHTTP(res, req)

	require.NotEqual(t, http.StatusUnauthorized, res.Code, "valid token must pass auth layer")
}
