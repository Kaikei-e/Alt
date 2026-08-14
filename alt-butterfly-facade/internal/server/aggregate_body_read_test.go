package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt-butterfly-facade/internal/handler"
)

// TestServer_Aggregate_TruncatedBackendBodyIsNotBankedAsSuccess pins the other
// half of the fan-out deadline. A backend that answers with headers promptly
// and then stalls mid-body has its body read cut off at RequestTimeout, so the
// read error is the normal way the deadline surfaces once the response line has
// already arrived. Swallowing it reports the upstream's original 200 with a
// truncated JSON prefix in Data — the caller cannot tell a short answer from a
// complete one.
func TestServer_Aggregate_TruncatedBackendBodyIsNotBankedAsSuccess(t *testing.T) {
	release := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Answer the status line, then die halfway through the body.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"feedAmount":{"amount":`))
		w.(http.Flusher).Flush()

		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer backend.Close()
	defer close(release)

	secret := []byte("test-secret")
	cfg := Config{
		BackendURL:       backend.URL,
		Secret:           secret,
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   150 * time.Millisecond,
		StreamingTimeout: 5 * time.Minute,
	}
	srv := NewServerWithTransport(cfg, nil, http.DefaultTransport)

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/aggregate",
		strings.NewReader(`{"queries":["feed_stats"]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, secret))
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.ServeHTTP(rec, req)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("/v1/aggregate never returned")
	}

	require.Equal(t, http.StatusOK, rec.Code)

	var resp handler.AggregationResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp),
		"a truncated backend body must not leave the aggregate response unencodable: %q", rec.Body.String())
	require.Len(t, resp.Results, 1)

	result := resp.Results["feed_stats"]
	require.NotNil(t, result)
	assert.Equal(t, http.StatusBadGateway, result.StatusCode,
		"a body cut off by the request deadline is a failed fetch, not the upstream's 200")
	assert.Contains(t, result.Error, "context deadline exceeded")
	assert.Empty(t, result.Data, "the truncated prefix must not be handed to the caller as data")
}
