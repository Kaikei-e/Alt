package handler

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBFFHandler_TruncatedBackendBody_IsNotServedAsSuccess drives the shape of
// failure alt-backend produces when it dies mid-body: the status line and
// headers arrive, the body does not. What the BFF holds afterwards is a prefix
// of a JSON document, and serving that prefix under the 200 the status line
// promised is strictly worse than a 502 — the truncated bytes are charged to
// the dependency-class breaker as a success and stored in the cache, so every
// caller for the endpoint's whole TTL gets a parse error out of an
// X-Cache: HIT instead of an error it can retry.
func TestBFFHandler_TruncatedBackendBody_IsNotServedAsSuccess(t *testing.T) {
	const endpoint = "/alt.feeds.v2.FeedService/GetUnreadCount"
	const fullBody = `{"count":42}`

	var truncate atomic.Bool
	truncate.Store(true)

	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !truncate.Load() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fullBody))
			return
		}
		// Promise more bytes than we deliver and flush the prefix so the
		// client really does see a 200, then abort the connection:
		// io.ReadAll on the client side ends in io.ErrUnexpectedEOF.
		w.Header().Set("Content-Length", "64")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"count":4`))
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCache:          true,
		EnableCircuitBreaker: true,
		CacheMaxSize:         100,
		// Well above the single failure this test provokes, so the breaker
		// stays closed and the recovery request below is really answered by
		// the backend rather than rejected by an open circuit.
		CBFailureThreshold: 5,
		CBSuccessThreshold: 1,
		CBOpenTimeout:      time.Minute,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	req := httptest.NewRequest("POST", endpoint, nil)
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadGateway, rec.Code,
		"a body that ended mid-stream must surface as 502, not as the 200 its status line claimed")

	stats := handler.GetCircuitBreakerClassStats()
	require.NotNil(t, stats)
	require.NotNil(t, stats.Projection)
	assert.Equal(t, int64(1), stats.Projection.TotalFailures,
		"the truncated response must be charged to the breaker as a failure")
	assert.Equal(t, int64(0), stats.Projection.TotalSuccesses,
		"and never as a success — otherwise a dying backend keeps the circuit closed")

	cacheStats := handler.GetCacheStats()
	require.NotNil(t, cacheStats)
	assert.Equal(t, 0, cacheStats.Size, "a truncated body must not enter the cache")

	// The backend recovers. The next caller must get the real body from the
	// backend, not the truncated prefix out of a cache entry.
	truncate.Store(false)

	req2 := httptest.NewRequest("POST", endpoint, nil)
	req2.Header.Set("X-Alt-Backend-Token", token)
	rec2 := httptest.NewRecorder()

	handler.ServeHTTP(rec2, req2)

	assert.Equal(t, http.StatusOK, rec2.Code)
	assert.Equal(t, "MISS", rec2.Header().Get("X-Cache"))
	assert.Equal(t, fullBody, rec2.Body.String())
}

// TestBFFHandler_TruncatedBackendBody_DedupPathAlsoFails is the same defect
// reached through the deduplicator: executeRequest is shared, but the dedup
// path returns its result to every in-flight caller of the same key, so a
// truncated body swallowed there is fanned out instead of merely cached.
func TestBFFHandler_TruncatedBackendBody_DedupPathAlsoFails(t *testing.T) {
	const endpoint = "/alt.feeds.v2.FeedService/GetUnreadCount"

	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "64")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"count":4`))
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCache:          true,
		EnableDedup:          true,
		EnableCircuitBreaker: true,
		CacheMaxSize:         100,
		CBFailureThreshold:   5,
		CBSuccessThreshold:   1,
		CBOpenTimeout:        time.Minute,
		DedupWindow:          100 * time.Millisecond,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	req := httptest.NewRequest("POST", endpoint, nil)
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadGateway, rec.Code,
		"the dedup path must not fan a truncated body out to its waiters as a 200")

	cacheStats := handler.GetCacheStats()
	require.NotNil(t, cacheStats)
	assert.Equal(t, 0, cacheStats.Size, "a truncated body must not enter the cache")
}
