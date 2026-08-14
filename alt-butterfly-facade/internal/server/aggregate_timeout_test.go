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

// TestServer_Aggregate_FanOutHonoursRequestTimeout pins the deadline on the
// /v1/aggregate fan-out. AggregationHandler blocks on a WaitGroup until every
// query goroutine finishes, and the backend client runs with
// http.Client.Timeout unset, so a fetcher built on a deadline-less context
// leaks N goroutines per request whenever alt-backend stops answering.
func TestServer_Aggregate_FanOutHonoursRequestTimeout(t *testing.T) {
	release := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept the request and never write a response.
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
		strings.NewReader(`{"queries":["feed_stats","unread_count"]}`),
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
		t.Fatal("/v1/aggregate never returned: the fan-out fetcher builds its request on a deadline-less context, so a silent backend strands every query goroutine")
	}

	require.Equal(t, http.StatusOK, rec.Code)

	var resp handler.AggregationResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Results, 2)

	for query, result := range resp.Results {
		assert.Equal(t, http.StatusBadGateway, result.StatusCode, "query %q", query)
		assert.Contains(t, result.Error, "context deadline exceeded",
			"query %q must fail on the request deadline, not on something else", query)
	}
}
