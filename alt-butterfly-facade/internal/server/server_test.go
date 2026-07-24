package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	cfg := Config{
		BackendURL:       "http://localhost:9101",
		Secret:           []byte("test-secret"),
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}

	handler := NewServer(cfg, nil)

	assert.NotNil(t, handler)
}

func TestServer_HealthEndpoint(t *testing.T) {
	cfg := Config{
		BackendURL:       "http://localhost:9101",
		Secret:           []byte("test-secret"),
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}

	handler := NewServer(cfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))

	var resp HealthResponse
	err := json.Unmarshal(recorder.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "healthy", resp.Status)
	assert.Equal(t, "alt-butterfly-facade", resp.Service)
}

func TestServer_ProxyEndpoint_Unauthorized(t *testing.T) {
	cfg := Config{
		BackendURL:       "http://localhost:9101",
		Secret:           []byte("test-secret"),
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}

	handler := NewServer(cfg, nil)

	req := httptest.NewRequest(http.MethodPost, "/alt.feeds.v2.FeedService/GetFeedStats", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestServer_ProxyEndpoint_Success(t *testing.T) {
	// Create mock backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/alt.feeds.v2.FeedService/GetFeedStats", r.URL.Path)
		w.Header().Set("Content-Type", "application/proto")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	}))
	defer backend.Close()

	cfg := Config{
		BackendURL:       backend.URL,
		Secret:           []byte("test-secret"),
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}

	// Use HTTP/1.1 transport for testing
	handler := NewServerWithTransport(cfg, nil, http.DefaultTransport)

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

// TestServer_RESTRoute_NonAllowlistedPathIsStillForwarded documents the
// current (known-incomplete) state of ADR-000729 Phase 3: allowRESTPath is
// only used for warn-only telemetry today, not for routing enforcement. A
// path absent from restAllowlistPrefixes still reaches the upstream REST
// listener instead of being rejected with 404. If/when boundary enforcement
// lands, this test should be updated to assert http.StatusNotFound instead.
func TestServer_RESTRoute_NonAllowlistedPathIsStillForwarded(t *testing.T) {
	restBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/feeds/tags", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer restBackend.Close()

	cfg := Config{
		BackendURL:       "http://localhost:9101",
		BackendRESTURL:   restBackend.URL,
		Secret:           []byte("test-secret"),
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}

	handler := NewServerWithTransport(cfg, nil, http.DefaultTransport)

	// "/v1/feeds/tags" is not in restAllowlistPrefixes.
	req := httptest.NewRequest(http.MethodGet, "/v1/feeds/tags", nil)
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, []byte("test-secret")))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestServer_TTSRoute_Unauthorized(t *testing.T) {
	cfg := Config{
		BackendURL:       "http://localhost:9101",
		TTSConnectURL:    "http://localhost:9700",
		Secret:           []byte("test-secret"),
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}

	handler := NewServer(cfg, nil)

	req := httptest.NewRequest(http.MethodPost, "/alt.tts.v1.TTSService/Synthesize", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestServer_TTSRoute_Success(t *testing.T) {
	// Create mock TTS backend
	ttsBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/alt.tts.v1.TTSService/Synthesize", r.URL.Path)
		assert.Empty(t, r.Header.Get("X-Service-Token"), "auth is transport-layer now")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"audioWav":""}`))
	}))
	defer ttsBackend.Close()

	// Create mock alt-backend (should NOT receive TTS requests)
	altBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("TTS request should not reach alt-backend, got path: %s", r.URL.Path)
	}))
	defer altBackend.Close()

	cfg := Config{
		BackendURL:       altBackend.URL,
		TTSConnectURL:    ttsBackend.URL,
		Secret:           []byte("test-secret"),
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}

	handler := NewServerWithTransport(cfg, nil, http.DefaultTransport)

	req := httptest.NewRequest(
		http.MethodPost,
		"/alt.tts.v1.TTSService/Synthesize",
		strings.NewReader(`{"text":"test"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, []byte("test-secret")))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestServer_TTSRoute_NotRegistered_WhenURLEmpty(t *testing.T) {
	// Create mock backend that accepts all requests
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	cfg := Config{
		BackendURL:       backend.URL,
		TTSConnectURL:    "", // empty = TTS route not registered
		Secret:           []byte("test-secret"),
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}

	handler := NewServerWithTransport(cfg, nil, http.DefaultTransport)

	// TTS request should fall through to catch-all (alt-backend)
	req := httptest.NewRequest(
		http.MethodPost,
		"/alt.tts.v1.TTSService/Synthesize",
		strings.NewReader(`{"text":"test"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, []byte("test-secret")))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	// Falls through to alt-backend catch-all
	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestServer_AcolyteRoute_Unauthorized(t *testing.T) {
	cfg := Config{
		BackendURL:        "http://localhost:9101",
		AcolyteConnectURL: "http://localhost:8090",
		Secret:            []byte("test-secret"),
		Issuer:            "auth-hub",
		Audience:          "alt-backend",
		RequestTimeout:    30 * time.Second,
		StreamingTimeout:  5 * time.Minute,
	}

	handler := NewServer(cfg, nil)

	req := httptest.NewRequest(http.MethodPost, "/alt.acolyte.v1.AcolyteService/HealthCheck", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestServer_AcolyteRoute_Success(t *testing.T) {
	acolyteBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/alt.acolyte.v1.AcolyteService/HealthCheck", r.URL.Path)
		assert.Empty(t, r.Header.Get("X-Service-Token"), "auth is transport-layer now")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer acolyteBackend.Close()

	altBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Acolyte request should not reach alt-backend, got path: %s", r.URL.Path)
	}))
	defer altBackend.Close()

	cfg := Config{
		BackendURL:        altBackend.URL,
		AcolyteConnectURL: acolyteBackend.URL,
		Secret:            []byte("test-secret"),
		Issuer:            "auth-hub",
		Audience:          "alt-backend",
		RequestTimeout:    30 * time.Second,
		StreamingTimeout:  5 * time.Minute,
	}

	handler := NewServerWithTransport(cfg, nil, http.DefaultTransport)

	req := httptest.NewRequest(
		http.MethodPost,
		"/alt.acolyte.v1.AcolyteService/HealthCheck",
		strings.NewReader(`{}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, []byte("test-secret")))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"status":"ok"`)
}

func TestServer_AcolyteRoute_NotRegistered_WhenURLEmpty(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	cfg := Config{
		BackendURL:        backend.URL,
		AcolyteConnectURL: "",
		Secret:            []byte("test-secret"),
		Issuer:            "auth-hub",
		Audience:          "alt-backend",
		RequestTimeout:    30 * time.Second,
		StreamingTimeout:  5 * time.Minute,
	}

	handler := NewServerWithTransport(cfg, nil, http.DefaultTransport)

	req := httptest.NewRequest(
		http.MethodPost,
		"/alt.acolyte.v1.AcolyteService/HealthCheck",
		strings.NewReader(`{}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, []byte("test-secret")))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	// Falls through to alt-backend catch-all
	assert.Equal(t, http.StatusOK, recorder.Code)
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
