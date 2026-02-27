package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	assert.NotNil(t, handler.circuitBreaker)
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
	assert.Nil(t, handler.circuitBreaker)
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

	// Make requests until circuit opens
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetFeedStats", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("X-Alt-Backend-Token", token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// Circuit should be open now
	assert.Equal(t, resilience.StateOpen, handler.circuitBreaker.State())

	// Next request should fail immediately without hitting backend
	req := httptest.NewRequest("POST", "/alt.feeds.v2.FeedService/GetFeedStats", bytes.NewReader([]byte(`{}`)))
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
