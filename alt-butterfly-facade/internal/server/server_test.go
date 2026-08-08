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

	"alt-butterfly-facade/internal/handler"
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

// The plaintext REST surface is now closed: a /v1/* path that is not on the
// architectural allowlist dies at the BFF instead of reaching alt-backend's
// Echo listener (ADR-000729 Phase 3).
func TestServer_RESTRoute_NonAllowlistedPathIsRejected(t *testing.T) {
	restBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("non-allowlisted REST path must not reach alt-backend, got %s", r.URL.Path)
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

	paths := []string{
		"/v1/feeds/tags",
		"/v1/feeds/fetch/cursor",
		"/v1/feeds/x/y/tags",
		"/v1/whatever",
		"/v1/",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("X-Alt-Backend-Token", createValidToken(t, []byte("test-secret")))

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusNotFound, recorder.Code)
		})
	}
}

// /v1/bff/stats and /v1/aggregate are BFF-owned endpoints, not proxies, so
// they are deliberately absent from the REST allowlist. They survive because
// their exact mux patterns outrank the "/v1/" subtree — a fence worth keeping
// now that falling through to "/v1/" means 404 instead of a proxied request.
func TestServer_BFFOwnedV1Routes_OutrankRESTAllowlist(t *testing.T) {
	restBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("BFF-owned route must not be proxied, got %s", r.URL.Path)
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

	assert.False(t, allowRESTPath("/v1/bff/stats"), "guard: this test is only meaningful while the path is off the allowlist")
	assert.False(t, allowRESTPath("/v1/aggregate"), "guard: this test is only meaningful while the path is off the allowlist")

	statsReq := httptest.NewRequest(http.MethodGet, "/v1/bff/stats", nil)
	statsReq.Header.Set("X-Alt-Backend-Token", createValidAdminToken(t, []byte("test-secret")))
	statsRec := httptest.NewRecorder()
	handler.ServeHTTP(statsRec, statsReq)
	assert.Equal(t, http.StatusOK, statsRec.Code)

	// The aggregation handler rejects an unauthenticated caller itself; the
	// point here is only that it, not the 404, answers.
	aggReq := httptest.NewRequest(http.MethodPost, "/v1/aggregate", strings.NewReader(`{}`))
	aggRec := httptest.NewRecorder()
	handler.ServeHTTP(aggRec, aggReq)
	assert.NotEqual(t, http.StatusNotFound, aggRec.Code)
}

// Every allowlisted REST path — including the segment patterns that carry a
// resource id — still reaches the upstream Echo listener untouched.
func TestServer_RESTRoute_AllowlistedPathsAreForwarded(t *testing.T) {
	var seen string
	restBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
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

	paths := []string{
		"/v1/images/fetch",
		"/v1/dashboard/metrics",
		"/v1/dashboard/recap_jobs",
		"/v1/admin/scraping-domains",
		"/v1/csrf-token",
		"/v1/health",
		"/v1/rss-feed-link/list",
		"/v1/rss-feed-link/register",
		"/v1/rss-feed-link/random",
		"/v1/rss-feed-link/0f8fad5b-d9cb-469f-a165-70867728950e",
		"/v1/rss-feed-link/export/opml",
		"/v1/rss-feed-link/import/opml",
		"/v1/feeds/read",
		"/v1/feeds/stats/trends",
		"/v1/feeds/0f8fad5b-d9cb-469f-a165-70867728950e/tags",
		"/v1/articles/by-tag",
		"/v1/articles/123/tags",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			seen = ""
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("X-Alt-Backend-Token", createValidToken(t, []byte("test-secret")))

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, path, seen, "path must be forwarded unmodified")
		})
	}
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

// TestServer_BFFStats_KeepsItsShapeAndReportsRejections pins /v1/bff/stats as
// a contract: it is consumed elsewhere, so the per-class keys must survive.
// total_rejections is the additive part — the volume of requests an open
// breaker refused, which the transition log deliberately never emits a line
// per occurrence of.
func TestServer_BFFStats_KeepsItsShapeAndReportsRejections(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer backend.Close()

	secret := []byte("test-secret")
	cfg := Config{
		BackendURL:       backend.URL,
		Secret:           secret,
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
		BFFConfig: handler.BFFConfig{
			EnableCircuitBreaker: true,
			CBFailureThreshold:   2,
			CBSuccessThreshold:   1,
			CBOpenTimeout:        time.Hour,

			CBExternalContentFailureThreshold: 20,
			CBExternalContentOpenTimeout:      5 * time.Second,
		},
	}
	srv := NewServerWithTransport(cfg, nil, http.DefaultTransport)

	const endpoint = "/alt.feeds.v2.FeedService/GetUnreadFeeds"
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{}`))
		req.Header.Set("X-Alt-Backend-Token", createValidAdminToken(t, secret))
		srv.ServeHTTP(httptest.NewRecorder(), req)
	}

	statsReq := httptest.NewRequest(http.MethodGet, "/v1/bff/stats", nil)
	statsReq.Header.Set("X-Alt-Backend-Token", createValidAdminToken(t, secret))
	statsRec := httptest.NewRecorder()
	srv.ServeHTTP(statsRec, statsReq)
	require.Equal(t, http.StatusOK, statsRec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(statsRec.Body.Bytes(), &body))

	cb, ok := body["circuit_breaker"].(map[string]any)
	require.True(t, ok, "circuit_breaker must stay a top-level object")
	for _, key := range []string{"state", "total_successes", "total_failures", "total_rejections"} {
		assert.Contains(t, cb, key)
	}
	for _, class := range []string{"mutation", "projection", "non_critical", "external_content"} {
		slice, ok := cb[class].(map[string]any)
		require.True(t, ok, "per-class stats key %q must stay present", class)
		for _, key := range []string{"state", "total_successes", "total_failures", "total_rejections"} {
			assert.Contains(t, slice, key, "class %q", class)
		}
	}

	projection := cb["projection"].(map[string]any)
	assert.Equal(t, "OPEN", projection["state"])
	assert.Equal(t, float64(2), projection["total_failures"])
	assert.Equal(t, float64(2), projection["total_rejections"],
		"requests refused by the open breaker must be countable without reading the log")
	assert.Equal(t, float64(2), cb["total_rejections"], "the rollup sums every class")
}

// TestServer_BFFStats_TelemetryOutcomesAreCountable pins the counter half of
// the ClassTelemetry observability fix end to end, through the real HTTP
// stats endpoint rather than the handler's Go API: a fire-and-forget write's
// failures must be visible in /v1/bff/stats even though the endpoint has no
// breaker and so no state/rejections to report.
func TestServer_BFFStats_TelemetryOutcomesAreCountable(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer backend.Close()

	secret := []byte("test-secret")
	cfg := Config{
		BackendURL:       backend.URL,
		Secret:           secret,
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
		BFFConfig: handler.BFFConfig{
			EnableCircuitBreaker: true,
			CBFailureThreshold:   2,
			CBSuccessThreshold:   1,
			CBOpenTimeout:        time.Hour,
		},
	}
	srv := NewServerWithTransport(cfg, nil, http.DefaultTransport)

	const endpoint = "/alt.knowledge_home.v1.KnowledgeHomeService/TrackHomeAction"
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{}`))
		req.Header.Set("X-Alt-Backend-Token", createValidAdminToken(t, secret))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		require.Equal(t, http.StatusInternalServerError, rec.Code,
			"ClassTelemetry has no breaker: every request must reach the backend, none short-circuited")
	}

	statsReq := httptest.NewRequest(http.MethodGet, "/v1/bff/stats", nil)
	statsReq.Header.Set("X-Alt-Backend-Token", createValidAdminToken(t, secret))
	statsRec := httptest.NewRecorder()
	srv.ServeHTTP(statsRec, statsReq)
	require.Equal(t, http.StatusOK, statsRec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(statsRec.Body.Bytes(), &body))

	cb, ok := body["circuit_breaker"].(map[string]any)
	require.True(t, ok)
	telemetry, ok := cb["telemetry"].(map[string]any)
	require.True(t, ok, "telemetry must be visible in /v1/bff/stats, not silently dropped")
	assert.Equal(t, float64(3), telemetry["total_failures"])
	assert.Equal(t, float64(0), telemetry["total_successes"])
}
