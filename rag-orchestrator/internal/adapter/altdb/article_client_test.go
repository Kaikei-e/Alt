package altdb

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// C-001c: GetRecentArticles must send X-Service-Token when the client is
// constructed with a non-empty service token, so alt-backend's
// /v1/internal/articles/recent RequireServiceAuth accepts the call.
func TestHTTPArticleClient_GetRecentArticles_SendsServiceTokenHeader(t *testing.T) {
	var capturedToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedToken = r.Header.Get("X-Service-Token")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"articles":[],"since":"2026-04-14T00:00:00Z","until":"2026-04-14T01:00:00Z","count":0}`))
	}))
	defer srv.Close()

	client := NewHTTPArticleClient(srv.URL, 5*time.Second, "test-service-token", testLogger())
	_, err := client.GetRecentArticles(context.Background(), 24, 10)
	require.NoError(t, err)
	assert.Equal(t, "test-service-token", capturedToken,
		"X-Service-Token must be forwarded to alt-backend /v1/internal/articles/recent")
}

// When the client is configured with an empty service token, the header must
// not be sent at all (behaviour parity with pre-processor).
func TestHTTPArticleClient_GetRecentArticles_OmitsHeaderWhenEmpty(t *testing.T) {
	var seen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, seen = r.Header["X-Service-Token"]
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"articles":[],"since":"2026-04-14T00:00:00Z","until":"2026-04-14T01:00:00Z","count":0}`))
	}))
	defer srv.Close()

	client := NewHTTPArticleClient(srv.URL, 5*time.Second, "", testLogger())
	_, err := client.GetRecentArticles(context.Background(), 24, 10)
	require.NoError(t, err)
	assert.False(t, seen, "X-Service-Token must not be set when service token is empty")
}
