package handler

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"alt-butterfly-facade/internal/client"
)

func TestNewProxyHandler(t *testing.T) {
	backendClient := client.NewBackendClientWithTransport(
		"http://localhost:9101",
		30*time.Second,
		5*time.Minute,
		http.DefaultTransport,
	)
	secret := []byte("test-secret")

	handler := NewProxyHandler(backendClient, secret, "auth-hub", "alt-backend", nil, 30*time.Second, 5*time.Minute)

	assert.NotNil(t, handler)
}

func TestProxyHandler_ApplyConnectTimeout_WithHeader(t *testing.T) {
	backendClient := client.NewBackendClientWithTransport(
		"http://localhost:9101",
		30*time.Second,
		5*time.Minute,
		http.DefaultTransport,
	)
	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", nil, 30*time.Second, 5*time.Minute)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Connect-Timeout-Ms", "120000")

	newReq, cancel := handler.applyConnectTimeout(req)
	defer cancel()

	deadline, ok := newReq.Context().Deadline()
	assert.True(t, ok)
	// Should be ~120s from now
	remaining := time.Until(deadline)
	assert.Greater(t, remaining, 119*time.Second)
	assert.LessOrEqual(t, remaining, 120*time.Second)
}

func TestProxyHandler_ApplyConnectTimeout_WithoutHeader(t *testing.T) {
	backendClient := client.NewBackendClientWithTransport(
		"http://localhost:9101",
		30*time.Second,
		5*time.Minute,
		http.DefaultTransport,
	)
	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", nil, 30*time.Second, 5*time.Minute)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)

	newReq, cancel := handler.applyConnectTimeout(req)
	defer cancel()

	deadline, ok := newReq.Context().Deadline()
	assert.True(t, ok)
	// Should be ~30s (default) from now
	remaining := time.Until(deadline)
	assert.Greater(t, remaining, 29*time.Second)
	assert.LessOrEqual(t, remaining, 30*time.Second)
}

func TestProxyHandler_ApplyConnectTimeout_CappedAt5Minutes(t *testing.T) {
	backendClient := client.NewBackendClientWithTransport(
		"http://localhost:9101",
		30*time.Second,
		5*time.Minute,
		http.DefaultTransport,
	)
	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", nil, 30*time.Second, 5*time.Minute)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Connect-Timeout-Ms", "999999999") // ~277 hours

	newReq, cancel := handler.applyConnectTimeout(req)
	defer cancel()

	deadline, ok := newReq.Context().Deadline()
	assert.True(t, ok)
	// Should be capped at 5 minutes
	remaining := time.Until(deadline)
	assert.Greater(t, remaining, 4*time.Minute+59*time.Second)
	assert.LessOrEqual(t, remaining, 5*time.Minute)
}

func TestProxyHandler_ApplyConnectTimeout_StreamingUsesStreamingTimeout(t *testing.T) {
	backendClient := client.NewBackendClientWithTransport(
		"http://localhost:9101",
		30*time.Second,
		40*time.Minute,
		http.DefaultTransport,
	)
	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", nil, 30*time.Second, 40*time.Minute)

	req := httptest.NewRequest(http.MethodPost, "/alt.augur.v2.AugurService/StreamChat", nil)

	newReq, cancel := handler.applyConnectTimeout(req)
	defer cancel()

	deadline, ok := newReq.Context().Deadline()
	assert.True(t, ok)
	// Streaming requests should get streaming timeout (~40 min), not default (~30s)
	remaining := time.Until(deadline)
	assert.Greater(t, remaining, 39*time.Minute+59*time.Second)
	assert.LessOrEqual(t, remaining, 40*time.Minute)
}

func TestProxyHandler_ApplyConnectTimeout_StreamingNotCappedAtUnaryMax(t *testing.T) {
	backendClient := client.NewBackendClientWithTransport(
		"http://localhost:9101",
		30*time.Second,
		40*time.Minute,
		http.DefaultTransport,
	)
	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", nil, 30*time.Second, 40*time.Minute)

	// Streaming request with Connect-Timeout-Ms = 10 minutes (exceeds unary 5 min cap)
	req := httptest.NewRequest(http.MethodPost, "/alt.knowledge_home.v1.KnowledgeHomeService/StreamKnowledgeHomeUpdates", nil)
	req.Header.Set("Connect-Timeout-Ms", "600000") // 10 min

	newReq, cancel := handler.applyConnectTimeout(req)
	defer cancel()

	deadline, ok := newReq.Context().Deadline()
	assert.True(t, ok)
	// Should be ~10 min, NOT capped at 5 min (unary cap)
	remaining := time.Until(deadline)
	assert.Greater(t, remaining, 9*time.Minute+59*time.Second)
	assert.LessOrEqual(t, remaining, 10*time.Minute)
}

func TestProxyHandler_ApplyConnectTimeout_StreamingCappedAtStreamingMax(t *testing.T) {
	backendClient := client.NewBackendClientWithTransport(
		"http://localhost:9101",
		30*time.Second,
		40*time.Minute,
		http.DefaultTransport,
	)
	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", nil, 30*time.Second, 40*time.Minute)

	// Streaming request with absurdly large Connect-Timeout-Ms
	req := httptest.NewRequest(http.MethodPost, "/alt.feeds.v2.FeedService/StreamFeedStats", nil)
	req.Header.Set("Connect-Timeout-Ms", "999999999") // ~277 hours

	newReq, cancel := handler.applyConnectTimeout(req)
	defer cancel()

	deadline, ok := newReq.Context().Deadline()
	assert.True(t, ok)
	// Should be capped at streaming max (40 min), not unary max (5 min)
	remaining := time.Until(deadline)
	assert.Greater(t, remaining, 39*time.Minute+59*time.Second)
	assert.LessOrEqual(t, remaining, 40*time.Minute)
}

func TestProxyHandler_ApplyConnectTimeout_LoopStreamUsesStreamingTimeout(t *testing.T) {
	// Regression guard: StreamKnowledgeLoopUpdates must be recognized as a
	// streaming procedure so it picks up the longer streaming timeout. Previously
	// this path was treated as unary, which cancelled the backend stream after
	// the 30s default timeout and caused a reconnect storm.
	backendClient := client.NewBackendClientWithTransport(
		"http://localhost:9101",
		30*time.Second,
		40*time.Minute,
		http.DefaultTransport,
	)
	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", nil, 30*time.Second, 40*time.Minute)

	req := httptest.NewRequest(http.MethodPost, "/alt.knowledge.loop.v1.KnowledgeLoopService/StreamKnowledgeLoopUpdates", nil)

	newReq, cancel := handler.applyConnectTimeout(req)
	defer cancel()

	deadline, ok := newReq.Context().Deadline()
	assert.True(t, ok)
	remaining := time.Until(deadline)
	assert.Greater(t, remaining, 39*time.Minute+59*time.Second)
	assert.LessOrEqual(t, remaining, 40*time.Minute)
}

func TestIsStreamingProcedure_KnowledgeLoopStream(t *testing.T) {
	assert.True(
		t,
		isStreamingProcedure("/alt.knowledge.loop.v1.KnowledgeLoopService/StreamKnowledgeLoopUpdates"),
		"StreamKnowledgeLoopUpdates must be on the streaming allowlist so the BFF keeps buffering off and uses the streaming timeout",
	)
}

func TestProxyHandler_ApplyConnectTimeout_UnaryUsesDefaultTimeout(t *testing.T) {
	backendClient := client.NewBackendClientWithTransport(
		"http://localhost:9101",
		30*time.Second,
		5*time.Minute,
		http.DefaultTransport,
	)
	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", nil, 30*time.Second, 5*time.Minute)

	req := httptest.NewRequest(http.MethodPost, "/alt.feeds.v2.FeedService/GetFeedStats", nil)

	newReq, cancel := handler.applyConnectTimeout(req)
	defer cancel()

	deadline, ok := newReq.Context().Deadline()
	assert.True(t, ok)
	// Unary requests should get default timeout (~30s)
	remaining := time.Until(deadline)
	assert.Greater(t, remaining, 29*time.Second)
	assert.LessOrEqual(t, remaining, 30*time.Second)
}

func TestProxyHandler_ServeHTTP_Success(t *testing.T) {
	// Create mock backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/alt.feeds.v2.FeedService/GetFeedStats", r.URL.Path)
		assert.NotEmpty(t, r.Header.Get("X-Alt-Backend-Token"))

		w.Header().Set("Content-Type", "application/proto")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	}))
	defer backend.Close()

	backendClient := client.NewBackendClientWithTransport(
		backend.URL,
		30*time.Second,
		5*time.Minute,
		http.DefaultTransport,
	)

	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", nil, 30*time.Second, 5*time.Minute)

	// Create request with valid token
	req := httptest.NewRequest(
		http.MethodPost,
		"/alt.feeds.v2.FeedService/GetFeedStats",
		strings.NewReader("request"),
	)
	req.Header.Set("Content-Type", "application/proto")
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, []byte("test-secret")))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "response", recorder.Body.String())
}

func TestProxyHandler_ServeHTTP_MissingToken(t *testing.T) {
	backendClient := client.NewBackendClientWithTransport(
		"http://localhost:9101",
		30*time.Second,
		5*time.Minute,
		http.DefaultTransport,
	)

	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", nil, 30*time.Second, 5*time.Minute)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestProxyHandler_ServeHTTP_InvalidToken(t *testing.T) {
	backendClient := client.NewBackendClientWithTransport(
		"http://localhost:9101",
		30*time.Second,
		5*time.Minute,
		http.DefaultTransport,
	)

	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", nil, 30*time.Second, 5*time.Minute)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("X-Alt-Backend-Token", "invalid-token")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestProxyHandler_ServeHTTP_BackendError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer backend.Close()

	backendClient := client.NewBackendClientWithTransport(
		backend.URL,
		30*time.Second,
		5*time.Minute,
		http.DefaultTransport,
	)

	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", nil, 30*time.Second, 5*time.Minute)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, []byte("test-secret")))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
}

func TestProxyHandler_ServeHTTP_StreamingRequest(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/alt.feeds.v2.FeedService/StreamFeedStats", r.URL.Path)

		w.Header().Set("Content-Type", "application/connect+proto")
		w.WriteHeader(http.StatusOK)

		flusher, _ := w.(http.Flusher)
		for i := 0; i < 3; i++ {
			w.Write([]byte("chunk"))
			flusher.Flush()
		}
	}))
	defer backend.Close()

	backendClient := client.NewBackendClientWithTransport(
		backend.URL,
		30*time.Second,
		5*time.Minute,
		http.DefaultTransport,
	)

	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", nil, 30*time.Second, 5*time.Minute)

	req := httptest.NewRequest(
		http.MethodPost,
		"/alt.feeds.v2.FeedService/StreamFeedStats",
		nil,
	)
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, []byte("test-secret")))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	body, _ := io.ReadAll(recorder.Body)
	assert.Contains(t, string(body), "chunk")
}

// TestProxyHandler_ServeHTTP_LogsAccess gives ops a breadcrumb per proxy hop.
// Without this the BFF chain (SvelteKit → butterfly-facade → alt-backend) has
// an observability gap that hides whether a Connect-RPC request reached the
// facade at all. See plan db-dwir7y3s Phase 1-C.
func TestProxyHandler_ServeHTTP_LogsAccess(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}))
	defer backend.Close()

	backendClient := client.NewBackendClientWithTransport(
		backend.URL,
		30*time.Second,
		5*time.Minute,
		http.DefaultTransport,
	)

	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", logger, 30*time.Second, 5*time.Minute)

	req := httptest.NewRequest(
		http.MethodPost,
		"/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop",
		strings.NewReader(`{}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, []byte("test-secret")))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	out := buf.String()
	assert.Contains(t, out, `"msg":"bff.proxy_request"`, "expected inbound access log")
	assert.Contains(t, out, `"path":"/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop"`)
	assert.Contains(t, out, `"streaming":false`)
	assert.Contains(t, out, `"msg":"bff.proxy_response"`, "expected outbound response log")
	assert.Contains(t, out, `"status":200`)
}

// createValidToken creates a valid JWT for testing
func createValidToken(t *testing.T, secret []byte) string {
	t.Helper()

	claims := jwt.MapClaims{
		"sub":   uuid.New().String(),
		"email": "test@example.com",
		"role":  "user",
		"sid":   "session-123",
		"iss":   "auth-hub",
		"aud":   []string{"alt-backend"},
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("failed to create test token: %v", err)
	}
	return tokenStr
}
