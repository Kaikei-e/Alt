package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func createValidAdminToken(t *testing.T, secret []byte) string {
	t.Helper()

	claims := jwt.MapClaims{
		"sub":   uuid.New().String(),
		"email": "admin@example.com",
		"role":  "admin",
		"sid":   "session-admin",
		"iss":   "auth-hub",
		"aud":   []string{"alt-backend"},
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	}

	tokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		t.Fatalf("failed to create admin test token: %v", err)
	}
	return tokenStr
}

// The BFF holds a legitimate mTLS identity toward alt-backend. Forwarding an
// arbitrary caller-supplied service path therefore turns it into a confused
// deputy: a self-registered user with a valid session could reach the
// service-to-service RPC surface through the product's own /api/v2 proxy.
// Those paths must die at the BFF regardless of how good the caller's token is.
func TestServer_RejectsServiceToServiceConnectPaths(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("service-to-service path must not reach alt-backend, got %s", r.URL.Path)
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
	h := NewServerWithTransport(cfg, nil, http.DefaultTransport)

	paths := []string{
		"/services.backend.v1.BackendInternalService/CreateArticle",
		"/services.backend.v1.BackendInternalService/ListArticlesWithTags",
		"/services.sovereign.v1.KnowledgeSovereignService/ListKnowledgeEvents",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alt-Backend-Token", createValidToken(t, []byte("test-secret")))

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusNotFound, rec.Code,
				"a valid user token must not open the service-to-service surface")
		})
	}
}

// User-facing Connect-RPC still flows through the catch-all.
func TestServer_ForwardsUserFacingConnectPaths(t *testing.T) {
	reached := false
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
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
	h := NewServerWithTransport(cfg, nil, http.DefaultTransport)

	req := httptest.NewRequest(http.MethodPost, "/alt.feeds.v2.FeedService/GetFeedStats", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, []byte("test-secret")))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, reached, "user-facing Connect-RPC must still reach alt-backend")
}

// The admin RPCs moved to alt-backend's internal listener. The BFF keeps its
// admin-role check at the edge and forwards to that listener, so a missing
// BackendInternalURL must not silently fall back to the public port.
func TestServer_AdminProxyTargetsInternalListener(t *testing.T) {
	internalReached := false
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		internalReached = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer internal.Close()

	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("admin RPC must not go to the public listener, got %s", r.URL.Path)
	}))
	defer public.Close()

	cfg := Config{
		BackendURL:         public.URL,
		BackendInternalURL: internal.URL,
		Secret:             []byte("test-secret"),
		Issuer:             "auth-hub",
		Audience:           "alt-backend",
		RequestTimeout:     30 * time.Second,
		StreamingTimeout:   5 * time.Minute,
	}
	h := NewServerWithTransport(cfg, nil, http.DefaultTransport)

	req := httptest.NewRequest(http.MethodPost,
		"/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetProjectionHealth",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Alt-Backend-Token", createValidAdminToken(t, []byte("test-secret")))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, internalReached, "admin RPC must reach the internal listener")
}
