package handler

import (
	"bytes"
	"encoding/json"
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
	assert.NotNil(t, handler.nonCriticalCB)
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
	assert.Nil(t, handler.nonCriticalCB)
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
	assert.Equal(t, resilience.StateOpen, handler.nonCriticalCB.State())

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

	assert.Equal(t, resilience.StateOpen, handler.nonCriticalCB.State())
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
