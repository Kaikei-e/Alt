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

// Authentication to alt-backend is established at the TLS transport layer
// (mTLS). GetRecentArticles no longer sends an X-Service-Token app header.
func TestHTTPArticleClient_GetRecentArticles_OmitsServiceTokenHeader(t *testing.T) {
	var seen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, seen = r.Header["X-Service-Token"]
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"articles":[],"since":"2026-04-14T00:00:00Z","until":"2026-04-14T01:00:00Z","count":0}`))
	}))
	defer srv.Close()

	client := NewHTTPArticleClient(srv.URL, 5*time.Second, "any-ignored-value", testLogger())
	_, err := client.GetRecentArticles(context.Background(), 24, 10)
	require.NoError(t, err)
	assert.False(t, seen, "X-Service-Token must not be set; auth is transport-layer")
}
