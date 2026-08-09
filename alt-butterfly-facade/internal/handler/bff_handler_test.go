package handler

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt-butterfly-facade/internal/cache"
	"alt-butterfly-facade/internal/client"
	"alt-butterfly-facade/internal/resilience"
)

func TestNewBFFHandler(t *testing.T) {
	secret := []byte("test-secret")
	config := BFFConfig{
		EnableCache:          true,
		EnableCircuitBreaker: true,
		EnableDedup:          true,
		CacheMaxSize:         100,
		CBFailureThreshold:   5,
		CBSuccessThreshold:   2,
		CBOpenTimeout:        30 * time.Second,
		DedupWindow:          100 * time.Millisecond,
	}

	backendClient := client.NewBackendClientWithTransport(
		"http://localhost:9101",
		30*time.Second,
		5*time.Minute,
		http.DefaultTransport,
	)

	handler := NewBFFHandler(
		backendClient,
		secret,
		"auth-hub",
		"alt-backend",
		nil,
		config,
	)

	assert.NotNil(t, handler)
	assert.NotNil(t, handler.responseCache)
	assert.NotNil(t, handler.mutationCB)
	assert.NotNil(t, handler.projectionCB)
	assert.NotNil(t, handler.nonCritical)
	assert.NotNil(t, handler.externalContentCB)
	assert.NotNil(t, handler.telemetryHealth)
	assert.NotNil(t, handler.deduplicator)
}

func TestNewBFFHandler_DisabledFeatures(t *testing.T) {
	secret := []byte("test-secret")
	config := BFFConfig{
		EnableCache:          false,
		EnableCircuitBreaker: false,
		EnableDedup:          false,
	}

	backendClient := client.NewBackendClientWithTransport(
		"http://localhost:9101",
		30*time.Second,
		5*time.Minute,
		http.DefaultTransport,
	)

	handler := NewBFFHandler(
		backendClient,
		secret,
		"auth-hub",
		"alt-backend",
		nil,
		config,
	)

	assert.NotNil(t, handler)
	assert.Nil(t, handler.responseCache)
	assert.Nil(t, handler.mutationCB)
	assert.Nil(t, handler.projectionCB)
	assert.Nil(t, handler.nonCritical)
	assert.Nil(t, handler.externalContentCB)
	assert.Nil(t, handler.telemetryHealth)
	assert.Nil(t, handler.deduplicator)
}

func TestBFFHandler_ServeHTTP_MissingToken(t *testing.T) {
	handler := createTestBFFHandler(t, BFFConfig{})

	req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetFeedStats", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestBFFHandler_ServeHTTP_InvalidToken(t *testing.T) {
	handler := createTestBFFHandler(t, BFFConfig{})

	req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetFeedStats", nil)
	req.Header.Set("X-Alt-Backend-Token", "invalid-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestBFFHandler_ServeHTTP_Success(t *testing.T) {
	// Create mock backend
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, BFFConfig{})

	req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetFeedStats", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Alt-Backend-Token", createTestToken(t, secret))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ok")
}

func TestBFFHandler_CircuitBreaker_OpensOnFailures(t *testing.T) {
	// Create mock backend that always fails
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCircuitBreaker: true,
		CBFailureThreshold:   3,
		CBSuccessThreshold:   1,
		CBOpenTimeout:        1 * time.Hour, // Long timeout to ensure it stays open
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	// Make requests until circuit opens (GetFeedStats is non-critical)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetFeedStats", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// Non-critical circuit should be open now
	assert.Equal(t, resilience.StateOpen, handler.nonCritical.forEndpoint("/alt.feeds.v2.FeedService/GetFeedStats").State())

	// Next non-critical request should fail immediately without hitting backend
	req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetFeedStats", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// TestBFFHandler_CircuitBreaker_NonCriticalOpenDoesNotBlockMarkAsRead verifies
// that Article/other non-critical failures do not trip Critical Feed Mutations.
func TestBFFHandler_CircuitBreaker_NonCriticalOpenDoesNotBlockMarkAsRead(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/alt.article.v2.ArticleService/FetchArticleContent" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCircuitBreaker: true,
		CBFailureThreshold:   3,
		CBSuccessThreshold:   1,
		CBOpenTimeout:        1 * time.Hour,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/alt.article.v2.ArticleService/FetchArticleContent", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	assert.Equal(t, resilience.StateOpen, handler.nonCritical.forEndpoint("/alt.article.v2.ArticleService/FetchArticleContent").State())
	assert.Equal(t, resilience.StateClosed, handler.mutationCB.State())
	assert.Equal(t, resilience.StateClosed, handler.projectionCB.State())

	req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/MarkAsRead", bytes.NewReader([]byte(`{"articleUrl":"https://example.com"}`)))
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestBFFHandler_CircuitBreaker_ProjectionOpenDoesNotBlockMarkAsRead verifies
// GetAllFeeds failures do not share MarkAsRead's failure budget.
func TestBFFHandler_CircuitBreaker_ProjectionOpenDoesNotBlockMarkAsRead(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/alt.feeds.v2.FeedService/GetAllFeeds" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCircuitBreaker: true,
		CBFailureThreshold:   3,
		CBSuccessThreshold:   1,
		CBOpenTimeout:        1 * time.Hour,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetAllFeeds", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	assert.Equal(t, resilience.StateOpen, handler.projectionCB.State())
	assert.Equal(t, resilience.StateClosed, handler.mutationCB.State())

	req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/MarkAsRead", bytes.NewReader([]byte(`{"articleUrl":"https://example.com"}`)))
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestBFFHandler_CircuitBreaker_CriticalOpenBlocksMarkAsRead verifies that
// when the mutation breaker is open, MarkAsRead is rejected with 503.
func TestBFFHandler_CircuitBreaker_CriticalOpenBlocksMarkAsRead(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCircuitBreaker: true,
		CBFailureThreshold:   3,
		CBSuccessThreshold:   1,
		CBOpenTimeout:        1 * time.Hour,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/MarkAsRead", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	assert.Equal(t, resilience.StateOpen, handler.mutationCB.State())

	req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/MarkAsRead", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestBFFHandler_ErrorNormalization(t *testing.T) {
	// Create mock backend that returns 502
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableErrorNormalization: true,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetFeedStats", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should return the error (backend returned 502)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestBFFHandler_Cache_HitAndMiss(t *testing.T) {
	callCount := 0
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"count": ` + string(rune('0'+callCount)) + `}`))
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCache:     true,
		CacheMaxSize:    100,
		CacheDefaultTTL: 30 * time.Second,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	// First request - cache miss
	req1 := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetDetailedFeedStats", bytes.NewReader([]byte(`{}`)))
	req1.Header.Set("X-Alt-Backend-Token", token)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	assert.Equal(t, http.StatusOK, rec1.Code)
	assert.Equal(t, "MISS", rec1.Header().Get("X-Cache"))

	// Second request - cache hit
	req2 := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetDetailedFeedStats", bytes.NewReader([]byte(`{}`)))
	req2.Header.Set("X-Alt-Backend-Token", token)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	assert.Equal(t, http.StatusOK, rec2.Code)
	assert.Equal(t, "HIT", rec2.Header().Get("X-Cache"))

	// Backend should only be called once
	assert.Equal(t, 1, callCount)
}

func TestBFFHandler_GetCacheStats(t *testing.T) {
	config := BFFConfig{
		EnableCache:  true,
		CacheMaxSize: 100,
	}
	handler := createTestBFFHandler(t, config)

	stats := handler.GetCacheStats()
	require.NotNil(t, stats)
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)
}

func TestBFFHandler_GetCircuitBreakerStats(t *testing.T) {
	config := BFFConfig{
		EnableCircuitBreaker: true,
		CBFailureThreshold:   5,
		CBSuccessThreshold:   2,
		CBOpenTimeout:        30 * time.Second,
	}
	handler := createTestBFFHandler(t, config)

	stats := handler.GetCircuitBreakerStats()
	require.NotNil(t, stats)
	assert.Equal(t, resilience.StateClosed, stats.State)
}

func TestBFFHandler_NormalizedError_Unauthorized(t *testing.T) {
	config := BFFConfig{
		EnableErrorNormalization: true,
	}
	handler := createTestBFFHandler(t, config)

	req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetFeedStats", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	var normalized NormalizedError
	err := json.Unmarshal(rec.Body.Bytes(), &normalized)
	require.NoError(t, err)
	assert.Equal(t, "INVALID_TOKEN", normalized.Code)
	assert.False(t, normalized.IsRetryable)
}

func TestBFFHandler_ApplyConnectTimeout_WithHeader(t *testing.T) {
	handler := createTestBFFHandler(t, BFFConfig{})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Connect-Timeout-Ms", "120000")

	newReq, cancel := handler.applyConnectTimeout(req)
	defer cancel()

	deadline, ok := newReq.Context().Deadline()
	assert.True(t, ok)
	remaining := time.Until(deadline)
	assert.Greater(t, remaining, 119*time.Second)
	assert.LessOrEqual(t, remaining, 120*time.Second)
}

func TestBFFHandler_ApplyConnectTimeout_WithoutHeader(t *testing.T) {
	handler := createTestBFFHandler(t, BFFConfig{})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)

	newReq, cancel := handler.applyConnectTimeout(req)
	defer cancel()

	deadline, ok := newReq.Context().Deadline()
	assert.True(t, ok)
	remaining := time.Until(deadline)
	assert.Greater(t, remaining, 29*time.Second)
	assert.LessOrEqual(t, remaining, 30*time.Second)
}

func TestBFFHandler_ServeHTTP_StreamingDelegatesToProxyHandler(t *testing.T) {
	// Streaming requests (e.g. StreamChat) must be delegated to ProxyHandler
	// which properly streams response chunks with flushing, instead of being
	// buffered by BFFHandler's io.ReadAll path.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/alt.augur.v2.AugurService/StreamChat", r.URL.Path)

		w.Header().Set("Content-Type", "application/connect+proto")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		for i := 0; i < 3; i++ {
			w.Write([]byte("chunk"))
			flusher.Flush()
		}
	}))
	defer backend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCache:          true,
		EnableCircuitBreaker: true,
		EnableDedup:          true,
		CacheMaxSize:         100,
		CBFailureThreshold:   5,
		CBSuccessThreshold:   2,
		CBOpenTimeout:        30 * time.Second,
		DedupWindow:          100 * time.Millisecond,
	}
	handler := createTestBFFHandlerWithBackend(t, backend.URL, secret, config)
	token := createTestToken(t, secret)

	req := httptest.NewRequest("POST", "/alt.augur.v2.AugurService/StreamChat", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	// Verify the response contains streamed chunks
	assert.Contains(t, rec.Body.String(), "chunk")
	// Streaming responses should NOT have X-Cache header (bypasses BFF features)
	assert.Empty(t, rec.Header().Get("X-Cache"))
}

func TestBFFHandler_ServeHTTP_UnaryStillUsesBFFFeatures(t *testing.T) {
	// Unary requests should still go through BFF features (cache, dedup, etc.)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer backend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCache:  true,
		CacheMaxSize: 100,
	}
	handler := createTestBFFHandlerWithBackend(t, backend.URL, secret, config)
	token := createTestToken(t, secret)

	req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetFeedStats", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	// Unary requests should go through BFF path and have X-Cache header
	assert.Equal(t, "MISS", rec.Header().Get("X-Cache"))
}

// TestBFFHandler_StreamingProcedure_CircuitBreaker_RecordsFailureOnConnectionError
// locks down the second half of the streaming/BFF-features contract: even
// though streaming procedures bypass cache/dedup (see
// TestBFFHandler_ServeHTTP_StreamingDelegatesToProxyHandler above) and are
// forwarded via ProxyHandler for true chunk-level streaming, the circuit
// breaker must still observe connection failures for those requests.
// ServeHTTP's isStreamingProcedure branch currently returns via
// h.proxyHandler.ServeHTTP(w, r) without ever touching h.circuitBreaker, so
// this failure is invisible to the breaker today and TotalFailures stays 0.
func TestBFFHandler_StreamingProcedure_CircuitBreaker_RecordsFailureOnConnectionError(t *testing.T) {
	// Server that is closed immediately so the streaming forward fails to
	// connect (connection refused), simulating a backend outage.
	deadBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCircuitBreaker: true,
		CBFailureThreshold:   5,
		CBSuccessThreshold:   2,
		CBOpenTimeout:        30 * time.Second,
	}
	handler := createTestBFFHandlerWithBackend(t, deadBackend.URL, secret, config)
	token := createTestToken(t, secret)

	req := httptest.NewRequest("POST", "/alt.augur.v2.AugurService/StreamChat", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	stats := handler.GetCircuitBreakerStats()
	require.NotNil(t, stats)
	assert.Equal(t, int64(1), stats.TotalFailures, "streaming connection failure must be recorded by the circuit breaker")
}

// TestBFFHandler_StreamingProcedure_CircuitBreaker_RecordsSuccessOnHealthyStream
// verifies the complementary case: a streaming request that connects and
// gets a successful initial response must record a circuit breaker success,
// so a breaker previously opened by streaming failures can also recover
// via streaming traffic instead of only via unary traffic.
func TestBFFHandler_StreamingProcedure_CircuitBreaker_RecordsSuccessOnHealthyStream(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/connect+proto")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)
		w.Write([]byte("chunk"))
		flusher.Flush()
	}))
	defer backend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCircuitBreaker: true,
		CBFailureThreshold:   5,
		CBSuccessThreshold:   2,
		CBOpenTimeout:        30 * time.Second,
	}
	handler := createTestBFFHandlerWithBackend(t, backend.URL, secret, config)
	token := createTestToken(t, secret)

	req := httptest.NewRequest("POST", "/alt.augur.v2.AugurService/StreamChat", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	stats := handler.GetCircuitBreakerStats()
	require.NotNil(t, stats)
	assert.Equal(t, int64(1), stats.TotalSuccesses, "streaming initial-response success must be recorded by the circuit breaker")
}

func TestBFFHandler_MarkAsRead_InvalidatesUnreadProjectionCache(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCache:          true,
		CacheMaxSize:         100,
		EnableCircuitBreaker: false,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)
	userID := "550e8400-e29b-41d4-a716-446655440000"
	unread := "/alt.feeds.v2.FeedService/GetUnreadFeeds"

	// Seed unread projection cache for this user (two body hashes) + other user.
	handler.responseCache.Set(
		cache.BuildCacheKey(userID, unread, []byte(`{"cursor":"1"}`)),
		&cache.CacheEntry{Response: []byte(`{"stale":1}`), StatusCode: 200, CachedAt: time.Now(), TTL: 30 * time.Second},
	)
	handler.responseCache.Set(
		cache.BuildCacheKey(userID, unread, []byte(`{"cursor":"2"}`)),
		&cache.CacheEntry{Response: []byte(`{"stale":2}`), StatusCode: 200, CachedAt: time.Now(), TTL: 30 * time.Second},
	)
	handler.responseCache.Set(
		cache.BuildCacheKey("other-user", unread, []byte(`{"cursor":"1"}`)),
		&cache.CacheEntry{Response: []byte(`{"other":1}`), StatusCode: 200, CachedAt: time.Now(), TTL: 30 * time.Second},
	)

	req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/MarkAsRead", bytes.NewReader([]byte(`{"articleUrl":"https://example.com"}`)))
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	_, found := handler.responseCache.Get(cache.BuildCacheKey(userID, unread, []byte(`{"cursor":"1"}`)))
	assert.False(t, found)
	_, found = handler.responseCache.Get(cache.BuildCacheKey(userID, unread, []byte(`{"cursor":"2"}`)))
	assert.False(t, found)
	_, found = handler.responseCache.Get(cache.BuildCacheKey("other-user", unread, []byte(`{"cursor":"1"}`)))
	assert.True(t, found)
}

func TestBFFHandler_ApplyConnectTimeout_CappedAt5Minutes(t *testing.T) {
	handler := createTestBFFHandler(t, BFFConfig{})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Connect-Timeout-Ms", "999999999")

	newReq, cancel := handler.applyConnectTimeout(req)
	defer cancel()

	deadline, ok := newReq.Context().Deadline()
	assert.True(t, ok)
	remaining := time.Until(deadline)
	assert.Greater(t, remaining, 4*time.Minute+59*time.Second)
	assert.LessOrEqual(t, remaining, 5*time.Minute)
}

// Helper functions

func createTestBFFHandler(t *testing.T, config BFFConfig) *BFFHandler {
	secret := []byte("test-secret-key")
	backendClient := client.NewBackendClientWithTransport(
		"http://localhost:9101",
		30*time.Second,
		5*time.Minute,
		http.DefaultTransport,
	)

	return NewBFFHandler(
		backendClient,
		secret,
		"auth-hub",
		"alt-backend",
		nil,
		config,
	)
}

func createTestBFFHandlerWithBackend(t *testing.T, backendURL string, secret []byte, config BFFConfig) *BFFHandler {
	backendClient := client.NewBackendClientWithTransport(
		backendURL,
		30*time.Second,
		5*time.Minute,
		http.DefaultTransport,
	)

	return NewBFFHandler(
		backendClient,
		secret,
		"auth-hub",
		"alt-backend",
		nil,
		config,
	)
}

func createTestToken(t *testing.T, secret []byte) string {
	claims := jwt.MapClaims{
		"sub": "550e8400-e29b-41d4-a716-446655440000",
		"iss": "auth-hub",
		"aud": []string{"alt-backend"},
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(secret)
	require.NoError(t, err)
	return signedToken
}

// TestBFFHandler_CircuitBreaker_HalfOpenTrialServedFromCacheDoesNotWedge drives
// the exact wedge sequence: alt-backend blips, the unread-projection breaker
// trips, OpenTimeout elapses, and the request that takes the single half-open
// trial permit is answered from a still-warm cache entry — it never reaches
// the backend and so records neither success nor failure. The permit must be
// handed back; otherwise every later unread-projection request is rejected
// with 503 forever, long after alt-backend recovered.
func TestBFFHandler_CircuitBreaker_HalfOpenTrialServedFromCacheDoesNotWedge(t *testing.T) {
	var backendFails atomic.Bool
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if backendFails.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"count":1}`))
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCache:          true,
		EnableCircuitBreaker: true,
		CacheMaxSize:         100,
		CBFailureThreshold:   3,
		CBSuccessThreshold:   1,
		CBOpenTimeout:        50 * time.Millisecond,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	// Two distinct request bodies so they land on distinct cache keys: one is
	// warmed while the backend is healthy, the other always misses the cache
	// and therefore always needs the backend.
	warmBody := []byte(`{"warm":true}`)
	coldBody := []byte(`{"cold":true}`)
	do := func(body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetUnreadCount", bytes.NewReader(body))
		req.Header.Set("X-Alt-Backend-Token", token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	// Warm the 15s-TTL GetUnreadCount cache entry
	require.Equal(t, http.StatusOK, do(warmBody).Code)

	// alt-backend blips: consecutive failures trip the projection breaker
	backendFails.Store(true)
	for i := 0; i < 3; i++ {
		do(coldBody)
	}
	require.Equal(t, resilience.StateOpen, handler.projectionCB.State())

	// OpenTimeout elapses; this request takes the single trial permit but is
	// answered from cache without ever touching the backend
	time.Sleep(60 * time.Millisecond)
	trial := do(warmBody)
	require.Equal(t, http.StatusOK, trial.Code)
	require.Equal(t, "HIT", trial.Header().Get("X-Cache"))

	// alt-backend recovers seconds later; the next real read must reach it
	backendFails.Store(false)
	rec := do(coldBody)

	assert.Equal(t, http.StatusOK, rec.Code,
		"a half-open trial answered from cache must not wedge the projection breaker shut")
	assert.Equal(t, resilience.StateClosed, handler.projectionCB.State())
}

// TestBFFHandler_CircuitBreaker_HalfOpenParallelRequestsReleasePermits runs the
// wedge shape under parallel load: while the projection breaker is half-open,
// concurrent unread-projection requests race between cache hits (which record
// no outcome) and real backend calls (which do). However each request exits,
// its trial permit must come back, so the class keeps admitting requests
// instead of wedging. Exercised under -race for the permit accounting.
func TestBFFHandler_CircuitBreaker_HalfOpenParallelRequestsReleasePermits(t *testing.T) {
	var backendFails atomic.Bool
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if backendFails.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"count":1}`))
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCache:          true,
		EnableCircuitBreaker: true,
		CacheMaxSize:         100,
		CBFailureThreshold:   3,
		// High enough that the storm never closes the circuit, so every request
		// keeps contending for the half-open trial slot.
		CBSuccessThreshold: 1_000_000,
		CBOpenTimeout:      300 * time.Millisecond,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	warmBody := []byte(`{"warm":true}`)
	coldBody := []byte(`{"cold":true}`)
	do := func(body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetUnreadCount", bytes.NewReader(body))
		req.Header.Set("X-Alt-Backend-Token", token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	// Warm the cache, then trip the projection breaker
	require.Equal(t, http.StatusOK, do(warmBody).Code)
	backendFails.Store(true)
	for i := 0; i < 3; i++ {
		do(coldBody)
	}
	require.Equal(t, resilience.StateOpen, handler.projectionCB.State())

	// OpenTimeout elapses and alt-backend recovers
	time.Sleep(320 * time.Millisecond)
	backendFails.Store(false)

	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				do(warmBody) // cache hit: records no outcome
				return
			}
			do(coldBody) // reaches the backend: records an outcome
		}(i)
	}
	wg.Wait()

	rec := do(coldBody)
	assert.Equal(t, http.StatusOK, rec.Code,
		"parallel half-open requests must all hand their trial permit back")
	assert.Equal(t, 0, handler.projectionCB.Stats().HalfOpenInFlight,
		"no trial permit may be stranded after the storm")
}

// TestBFFHandler_CircuitBreaker_ClientErrorIsNeutralForFailureBudget pins the
// two-sided rule for a 4xx: it is neither a dependency failure nor a
// dependency success. The backend answered; it just answered "no".
//
// Recording it as a failure lets one rate-limited or paywalled publisher
// (alt-backend maps a publisher 429 / 404 / 403 straight through) open the
// breaker for every endpoint sharing the class. Recording it as a success
// resets consecFailures and hides a genuine backend outage. The sequence
// below (500, 404, 500, 500 against a threshold of 3) distinguishes all three
// semantics: under "4xx is a failure" the breaker opens on the third request
// and the fourth never reaches the backend; under "4xx is a success" the
// breaker is still closed after the fourth.
func TestBFFHandler_CircuitBreaker_ClientErrorIsNeutralForFailureBudget(t *testing.T) {
	var backendHits atomic.Int32
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if backendHits.Add(1) == 2 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCircuitBreaker: true,
		CBFailureThreshold:   3,
		CBSuccessThreshold:   1,
		CBOpenTimeout:        1 * time.Hour,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	for i := 0; i < 4; i++ {
		req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetFeedStats", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	assert.Equal(t, int32(4), backendHits.Load(),
		"the 404 must not charge the failure budget: the breaker may not open before the fourth 500")
	nonCriticalCB := handler.nonCritical.forEndpoint("/alt.feeds.v2.FeedService/GetFeedStats")
	assert.Equal(t, resilience.StateOpen, nonCriticalCB.State(),
		"the 404 must not reset the failure budget either: three 5xx still open the breaker")
	assert.Equal(t, int64(3), nonCriticalCB.Stats().TotalFailures)
	assert.Equal(t, int64(0), nonCriticalCB.Stats().TotalSuccesses)
}

// TestBFFHandler_CircuitBreaker_RateLimitedUpstreamNeverOpensBreaker is the
// user-visible half of the same rule: swiping past article after article from
// a publisher that answers 429 must never take the class down.
func TestBFFHandler_CircuitBreaker_RateLimitedUpstreamNeverOpensBreaker(t *testing.T) {
	for _, status := range []int{
		http.StatusTooManyRequests,
		http.StatusNotFound,
		http.StatusForbidden,
		http.StatusBadRequest,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer mockBackend.Close()

			secret := []byte("test-secret-key")
			config := BFFConfig{
				EnableCircuitBreaker: true,
				CBFailureThreshold:   3,
				CBSuccessThreshold:   1,
				CBOpenTimeout:        1 * time.Hour,
			}
			handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
			token := createTestToken(t, secret)

			for i := 0; i < 10; i++ {
				req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetFeedStats", bytes.NewReader([]byte(`{}`)))
				req.Header.Set("X-Alt-Backend-Token", token)
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				require.Equal(t, status, rec.Code, "the upstream status must reach the client, not a breaker 503")
			}

			nonCriticalCB := handler.nonCritical.forEndpoint("/alt.feeds.v2.FeedService/GetFeedStats")
			assert.Equal(t, resilience.StateClosed, nonCriticalCB.State())
			assert.Equal(t, int64(0), nonCriticalCB.Stats().TotalFailures)
			assert.Equal(t, int64(0), nonCriticalCB.Stats().TotalSuccesses)
		})
	}
}

// TestBFFHandler_StreamingProcedure_ClientErrorIsNeutral applies the same rule
// to serveStreaming's initial-status decision.
func TestBFFHandler_StreamingProcedure_ClientErrorIsNeutral(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer backend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCircuitBreaker: true,
		CBFailureThreshold:   5,
		CBSuccessThreshold:   2,
		CBOpenTimeout:        30 * time.Second,
	}
	handler := createTestBFFHandlerWithBackend(t, backend.URL, secret, config)
	token := createTestToken(t, secret)

	req := httptest.NewRequest("POST", "/alt.augur.v2.AugurService/StreamChat", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Alt-Backend-Token", token)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	stats := handler.nonCritical.forEndpoint("/alt.augur.v2.AugurService/StreamChat").Stats()
	assert.Equal(t, int64(0), stats.TotalFailures, "a streamed 404 is an answer, not an outage")
	assert.Equal(t, int64(0), stats.TotalSuccesses, "nor may it reset the failure budget")
}

// TestBFFHandler_CircuitOpen_EmitsWireValidConnectError locks the shape of the
// rejection the client actually sees. connect-es parses the unary error body
// with codeFromString, which only accepts the protocol's lowercase snake_case
// code names; anything else parses as undefined and the client silently falls
// back to the HTTP-status-derived code, dropping every non-standard field on
// the floor. Retry guidance therefore has to travel in Retry-After, which
// connect-es surfaces as ConnectError.metadata.
func TestBFFHandler_CircuitOpen_EmitsWireValidConnectError(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCircuitBreaker:     true,
		EnableErrorNormalization: true,
		CBFailureThreshold:       2,
		CBSuccessThreshold:       1,
		CBOpenTimeout:            30 * time.Second,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetFeedStats", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
	require.Equal(t, resilience.StateOpen, handler.nonCritical.forEndpoint("/alt.feeds.v2.FeedService/GetFeedStats").State())

	req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetFeedStats", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, "30", rec.Header().Get("Retry-After"),
		"the open timeout must reach the client somewhere it can read it")

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "unavailable", body["code"],
		"Connect codes are lowercase snake_case; SCREAMING_SNAKE parses as undefined in connect-es")
	assert.Contains(t, body["message"], "circuit breaker")
}

// TestBFFHandler_CircuitBreaker_ExternalContentOpenDoesNotBlockOtherRPCs is
// ADR-000950's argument applied to the class it left behind. That ADR split
// Critical Feed Mutations and Unread Projection Reads out of the shared
// breaker, but FetchArticleContent — whose upstream hop leaves the cluster for
// a publisher — stayed in the non-critical megabucket, so a slow or hostile
// third-party site still rejects every other unclassified RPC.
func TestBFFHandler_CircuitBreaker_ExternalContentOpenDoesNotBlockOtherRPCs(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/alt.articles.v2.ArticleService/FetchArticleContent" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCircuitBreaker:              true,
		CBFailureThreshold:                3,
		CBSuccessThreshold:                1,
		CBOpenTimeout:                     1 * time.Hour,
		CBExternalContentFailureThreshold: 3,
		CBExternalContentOpenTimeout:      1 * time.Hour,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/alt.articles.v2.ArticleService/FetchArticleContent", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	assert.Equal(t, resilience.StateOpen, handler.externalContentCB.State())
	assert.Equal(t, resilience.StateClosed, handler.nonCritical.stats().State,
		"nothing has hit a non-critical endpoint yet, so the class-wide rollup must stay closed")
	assert.Equal(t, resilience.StateClosed, handler.mutationCB.State())
	assert.Equal(t, resilience.StateClosed, handler.projectionCB.State())

	for _, ep := range []string{
		"/alt.feeds.v2.FeedService/GetFeedStats",
		"/alt.feeds.v2.FeedService/MarkAsRead",
		"/alt.feeds.v2.FeedService/GetAllFeeds",
	} {
		req := httptest.NewRequest("POST", ep, bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, ep)
	}
}

// TestBFFHandler_CircuitBreaker_UnclassifiedEndpointDoesNotBlackoutOtherReads
// reproduces the finding this whole change exists to fix, literally: five
// 500s on an unclassified endpoint nobody has ever heard of must not black
// out KnowledgeHome / FetchArticlesByTag / Search for OpenTimeout. Before
// per-endpoint isolation, every one of these shared a single nonCriticalCB
// instance, so this is exactly the collateral-damage scenario the adversarial
// review reproduced against the prior fix (which only carved out the three
// telemetry endpoints and left this shared budget in place).
func TestBFFHandler_CircuitBreaker_UnclassifiedEndpointDoesNotBlackoutOtherReads(t *testing.T) {
	const obscureEndpoint = "/alt.feeds.v2.FeedService/GetFeedStats"
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == obscureEndpoint {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCircuitBreaker: true,
		CBFailureThreshold:   5,
		CBSuccessThreshold:   2,
		CBOpenTimeout:        30 * time.Second,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("POST", obscureEndpoint, bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	}
	require.Equal(t, resilience.StateOpen, handler.nonCritical.forEndpoint(obscureEndpoint).State(),
		"the obscure endpoint's own breaker must have tripped")

	for _, ep := range []string{
		"/alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome",
		"/alt.articles.v2.ArticleService/FetchArticlesByTag",
		"/alt.search.v2.SearchService/SearchArticles",
	} {
		req := httptest.NewRequest("POST", ep, bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "%s must not blackout as collateral damage from %s", ep, obscureEndpoint)
	}
}

// TestBFFHandler_CircuitBreaker_TelemetryFailuresDoNotOpenNonCriticalBreaker
// reproduces the incident directly: TrackHomeAction is fire-and-forget
// telemetry the frontend discards (.catch(() => {})), so five failing calls
// must not trip the shared non-critical breaker and black out unrelated
// reads such as FetchArticlesByTag.
func TestBFFHandler_CircuitBreaker_TelemetryFailuresDoNotOpenNonCriticalBreaker(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/alt.knowledge_home.v1.KnowledgeHomeService/TrackHomeAction" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCircuitBreaker: true,
		CBFailureThreshold:   5,
		CBSuccessThreshold:   2,
		CBOpenTimeout:        1 * time.Hour,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("POST", "/alt.knowledge_home.v1.KnowledgeHomeService/TrackHomeAction", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	}

	assert.Equal(t, resilience.StateClosed,
		handler.nonCritical.forEndpoint("/alt.articles.v2.ArticleService/FetchArticlesByTag").State(),
		"discarded telemetry failures must not open FetchArticlesByTag's breaker")

	req := httptest.NewRequest("POST", "/alt.articles.v2.ArticleService/FetchArticlesByTag", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "an unrelated read must not black out as collateral damage")
}

// TestBFFHandler_CircuitBreaker_NonCriticalOpenDoesNotBlockTelemetry is the
// bulkhead in the other direction: an unrelated read tripping the shared
// non-critical breaker must not stop fire-and-forget telemetry writes either.
func TestBFFHandler_CircuitBreaker_NonCriticalOpenDoesNotBlockTelemetry(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/alt.feeds.v2.FeedService/GetFeedStats" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCircuitBreaker: true,
		CBFailureThreshold:   3,
		CBSuccessThreshold:   1,
		CBOpenTimeout:        1 * time.Hour,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetFeedStats", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
	require.Equal(t, resilience.StateOpen, handler.nonCritical.forEndpoint("/alt.feeds.v2.FeedService/GetFeedStats").State())

	req := httptest.NewRequest("POST", "/alt.knowledge_home.v1.KnowledgeHomeService/TrackHomeAction", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "telemetry must not be gated by an unrelated class's breaker")
}

// TestBFFHandler_ExternalContentBreaker_UsesItsOwnBudget proves the class is
// wired to its own tuning and not to the internal defaults: it survives past
// the internal threshold, and it re-probes after the shorter open timeout.
func TestBFFHandler_ExternalContentBreaker_UsesItsOwnBudget(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCircuitBreaker:              true,
		CBFailureThreshold:                2,
		CBSuccessThreshold:                1,
		CBOpenTimeout:                     1 * time.Hour,
		CBExternalContentFailureThreshold: 4,
		CBExternalContentOpenTimeout:      20 * time.Millisecond,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	fetch := func() {
		req := httptest.NewRequest("POST", "/alt.articles.v2.ArticleService/FetchArticleContent", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	for i := 0; i < 3; i++ {
		fetch()
	}
	assert.Equal(t, resilience.StateClosed, handler.externalContentCB.State(),
		"the internal failure threshold must not govern the external-content class")

	fetch()
	require.Equal(t, resilience.StateOpen, handler.externalContentCB.State())

	time.Sleep(40 * time.Millisecond)
	assert.Equal(t, resilience.StateHalfOpen, handler.externalContentCB.State(),
		"the external-content class must re-probe on its own shorter open timeout")
}

// TestBFFHandler_CircuitOpen_RetryAfterUsesTheRejectingClassTimeout keeps the
// client's back-off hint honest: the external-content class re-probes sooner
// than the internal classes, so telling the client to wait CBOpenTimeout would
// keep Visual Preview dark long after the breaker reopened.
func TestBFFHandler_CircuitOpen_RetryAfterUsesTheRejectingClassTimeout(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCircuitBreaker:              true,
		EnableErrorNormalization:          true,
		CBFailureThreshold:                2,
		CBSuccessThreshold:                1,
		CBOpenTimeout:                     30 * time.Second,
		CBExternalContentFailureThreshold: 2,
		CBExternalContentOpenTimeout:      5 * time.Second,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/alt.articles.v2.ArticleService/FetchArticleContent", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		req.Header.Set("Connect-Protocol-Version", "1")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if i == 2 {
			require.Equal(t, http.StatusServiceUnavailable, rec.Code)
			assert.Equal(t, "5", rec.Header().Get("Retry-After"))
		}
	}
}

func TestBFFHandler_GetCircuitBreakerClassStats_IncludesExternalContent(t *testing.T) {
	config := BFFConfig{
		EnableCircuitBreaker:              true,
		CBFailureThreshold:                5,
		CBSuccessThreshold:                2,
		CBOpenTimeout:                     30 * time.Second,
		CBExternalContentFailureThreshold: 20,
		CBExternalContentOpenTimeout:      5 * time.Second,
	}
	handler := createTestBFFHandler(t, config)

	stats := handler.GetCircuitBreakerClassStats()
	require.NotNil(t, stats)
	require.NotNil(t, stats.ExternalContent)
	assert.Equal(t, resilience.StateClosed, stats.ExternalContent.State)
}

// decodeLogRecords parses the slog JSON stream a test captured.
func decodeLogRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	var out []map[string]any
	for {
		var rec map[string]any
		err := dec.Decode(&rec)
		if err != nil {
			break
		}
		out = append(out, rec)
	}
	return out
}

func circuitTransitionRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, rec := range decodeLogRecords(t, buf) {
		if rec["msg"] == "circuit_breaker_transition" {
			out = append(out, rec)
		}
	}
	return out
}

// TestBFFHandler_CircuitBreaker_LogsTransitionsAndAggregatesRejections is the
// wiring pin for the defect this observability work addresses: the BFF used to
// open a breaker, serve 503s, and recover without writing a single line. Every
// transition must now be logged with the class and endpoint that caused it,
// OPEN at WARN because it is user-visible degradation, and the requests
// refused while open must be summarised rather than logged one by one.
func TestBFFHandler_CircuitBreaker_LogsTransitionsAndAggregatesRejections(t *testing.T) {
	var failing atomic.Bool
	failing.Store(true)
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer mockBackend.Close()

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	secret := []byte("test-secret-key")
	backendClient := client.NewBackendClientWithTransport(
		mockBackend.URL, 30*time.Second, 5*time.Minute, http.DefaultTransport,
	)
	handler := NewBFFHandler(backendClient, secret, "auth-hub", "alt-backend", logger, BFFConfig{
		EnableCircuitBreaker: true,
		CBFailureThreshold:   3,
		CBSuccessThreshold:   1,
		CBOpenTimeout:        80 * time.Millisecond,

		CBExternalContentFailureThreshold: 20,
		CBExternalContentOpenTimeout:      5 * time.Second,
	})
	token := createTestToken(t, secret)

	const endpoint = "/alt.feeds.v2.FeedService/GetFeedStats"
	call := func() int {
		req := httptest.NewRequest("POST", endpoint, bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := 0; i < 3; i++ {
		call()
	}
	require.Equal(t, resilience.StateOpen, handler.nonCritical.forEndpoint(endpoint).State())

	const rejections = 50
	for i := 0; i < rejections; i++ {
		require.Equal(t, http.StatusServiceUnavailable, call())
	}

	tripped := circuitTransitionRecords(t, &logs)
	require.Len(t, tripped, 1, "an open breaker must not log one line per rejected request")
	assert.Equal(t, "WARN", tripped[0]["level"], "OPEN is user-visible degradation")
	assert.Equal(t, "CLOSED", tripped[0]["from"])
	assert.Equal(t, "OPEN", tripped[0]["to"])
	assert.Equal(t, "non_critical", tripped[0]["class"])
	assert.Equal(t, endpoint, tripped[0]["endpoint"])
	assert.Equal(t, float64(3), tripped[0]["consecutive_failures"])
	assert.Equal(t, float64(3), tripped[0]["failure_threshold"])
	assert.Equal(t, float64(0.08), tripped[0]["open_timeout_seconds"])

	// Backend recovers: the probe and the return to CLOSED must both be visible
	// so an operator sees recovery without polling /v1/bff/stats.
	failing.Store(false)
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, http.StatusOK, call())

	all := circuitTransitionRecords(t, &logs)
	require.Len(t, all, 3)
	assert.Equal(t, "INFO", all[1]["level"])
	assert.Equal(t, "HALF_OPEN", all[1]["to"])
	assert.Equal(t, float64(rejections), all[1]["rejected_since_last_report"],
		"the rejections an operator never saw individually must be summarised here")
	assert.Equal(t, "INFO", all[2]["level"])
	assert.Equal(t, "CLOSED", all[2]["to"])
	assert.Equal(t, float64(0), all[2]["rejected_since_last_report"])
}

// TestBFFHandler_TelemetryFailures_AreLoggedAndCounted is the wiring pin for
// the second defect the adversarial review found: ClassTelemetry has no
// breaker (by design — see breakerFor), but that used to also mean no counter
// and no log, so a 100%-failing TrackHomeAction — including the snooze /
// dismiss_recall writes it carries — was completely invisible at the BFF.
// This proves both halves of the fix: a single WARN line once the endpoint
// crosses DegradedThreshold (not one per discarded request), and the totals
// surfacing on GetCircuitBreakerClassStats().Telemetry.
func TestBFFHandler_TelemetryFailures_AreLoggedAndCounted(t *testing.T) {
	var failing atomic.Bool
	failing.Store(true)
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer mockBackend.Close()

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	secret := []byte("test-secret-key")
	backendClient := client.NewBackendClientWithTransport(
		mockBackend.URL, 30*time.Second, 5*time.Minute, http.DefaultTransport,
	)
	handler := NewBFFHandler(backendClient, secret, "auth-hub", "alt-backend", logger, BFFConfig{
		EnableCircuitBreaker: true,
		CBFailureThreshold:   3,
		CBSuccessThreshold:   1,
		CBOpenTimeout:        time.Hour,
	})
	token := createTestToken(t, secret)

	const endpoint = "/alt.knowledge_home.v1.KnowledgeHomeService/TrackHomeAction"
	call := func() int {
		req := httptest.NewRequest("POST", endpoint, bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := 0; i < 3; i++ {
		require.Equal(t, http.StatusInternalServerError, call(),
			"ClassTelemetry has no breaker: every request must still reach the backend")
	}

	degraded := telemetryHealthRecords(t, &logs, "telemetry_endpoint_degraded")
	require.Len(t, degraded, 1, "a run of telemetry failures must log once, not once per discarded write")
	assert.Equal(t, "WARN", degraded[0]["level"])
	assert.Equal(t, endpoint, degraded[0]["endpoint"])
	assert.Equal(t, float64(3), degraded[0]["consecutive_failures"])
	assert.Equal(t, float64(3), degraded[0]["total_failures"])

	stats := handler.GetCircuitBreakerClassStats()
	require.NotNil(t, stats.Telemetry, "telemetry outcomes must be countable, not just logged")
	assert.Equal(t, int64(3), stats.Telemetry.TotalFailures)
	assert.Equal(t, int64(0), stats.Telemetry.TotalSuccesses)

	// One more failure while already degraded must not log again.
	call()
	degraded = telemetryHealthRecords(t, &logs, "telemetry_endpoint_degraded")
	require.Len(t, degraded, 1)

	failing.Store(false)
	require.Equal(t, http.StatusOK, call())

	recovered := telemetryHealthRecords(t, &logs, "telemetry_endpoint_recovered")
	require.Len(t, recovered, 1, "the first success after a degraded run must be reported")
	assert.Equal(t, "INFO", recovered[0]["level"])
	assert.Equal(t, endpoint, recovered[0]["endpoint"])

	stats = handler.GetCircuitBreakerClassStats()
	assert.Equal(t, int64(1), stats.Telemetry.TotalSuccesses)
}

func telemetryHealthRecords(t *testing.T, buf *bytes.Buffer, msg string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, rec := range decodeLogRecords(t, buf) {
		if rec["msg"] == msg {
			out = append(out, rec)
		}
	}
	return out
}

// TestBFFHandler_UpstreamAttributed5xx_DoesNotChargeTheBreaker closes the half
// of ADR-000959 §1 that the 4xx rule left open.
//
// That decision made client errors neutral so one paywalled publisher could
// not open the class breaker. But a publisher that never answers at all — the
// commonest failure a feed reader meets — reaches the client as CodeUnavailable,
// which is a 5xx, so it kept charging the very budget the ADR meant to protect.
// The status code cannot distinguish "the site is dead" from "alt-backend is
// dead"; only the attribution header can.
func TestBFFHandler_UpstreamAttributed5xx_DoesNotChargeTheBreaker(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(FailureScopeHeader, FailureScopeHost)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCircuitBreaker:              true,
		CBFailureThreshold:                2,
		CBSuccessThreshold:                1,
		CBOpenTimeout:                     30 * time.Second,
		CBExternalContentFailureThreshold: 2,
		CBExternalContentOpenTimeout:      5 * time.Second,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("POST", "/alt.articles.v2.ArticleService/FetchArticleContent", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusServiceUnavailable, rec.Code,
			"the reader still learns this article is unavailable")
	}

	stats := handler.externalContentCB.Stats()
	assert.Equal(t, int64(0), stats.TotalFailures,
		"five dead publishers are not evidence that alt-backend is unhealthy")
	assert.Equal(t, int64(0), stats.TotalSuccesses,
		"nor may they reset the budget and mask a real outage")
	assert.Equal(t, resilience.StateClosed, handler.externalContentCB.State())
}

// TestBFFHandler_Unattributed5xx_StillChargesTheBreaker is the safety half.
// The header excuses a failure from the budget, so an unstamped 5xx has to
// keep counting — otherwise a backend that stopped stamping, or one broken in
// a way it cannot classify, would look healthy right up until it fell over.
func TestBFFHandler_Unattributed5xx_StillChargesTheBreaker(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCircuitBreaker:              true,
		CBFailureThreshold:                2,
		CBSuccessThreshold:                1,
		CBOpenTimeout:                     30 * time.Second,
		CBExternalContentFailureThreshold: 2,
		CBExternalContentOpenTimeout:      5 * time.Second,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/alt.articles.v2.ArticleService/FetchArticleContent", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	assert.Equal(t, resilience.StateOpen, handler.externalContentCB.State())
}

// TestBFFHandler_CircuitOpen_DeclaresGlobalFailureScope names the one condition
// that genuinely reaches every host, so the client can stop guessing.
//
// The frontend pauses all hosts on this and only this: a breaker rejection is
// a state of the gateway, and no other publisher can be routed around it. A
// per-article failure wearing the same CodeUnavailable must never trigger it.
func TestBFFHandler_CircuitOpen_DeclaresGlobalFailureScope(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableCircuitBreaker:              true,
		EnableErrorNormalization:          true,
		CBFailureThreshold:                2,
		CBSuccessThreshold:                1,
		CBOpenTimeout:                     30 * time.Second,
		CBExternalContentFailureThreshold: 2,
		CBExternalContentOpenTimeout:      5 * time.Second,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	var rec *httptest.ResponseRecorder
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/alt.articles.v2.ArticleService/FetchArticleContent", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, FailureScopeGlobal, rec.Header().Get(FailureScopeHeader))
	assert.Equal(t, "5", rec.Header().Get("Retry-After"))
}
