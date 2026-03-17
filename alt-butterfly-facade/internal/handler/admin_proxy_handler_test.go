package handler

import (
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

func TestAdminProxyHandler_ServeHTTP_RequiresAdminRole(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("backend should not be called for non-admin requests")
	}))
	defer backend.Close()

	backendClient := client.NewBackendClientWithTransport(
		backend.URL,
		30*time.Second,
		5*time.Minute,
		http.DefaultTransport,
	)

	handler := NewAdminProxyHandler(
		backendClient,
		[]byte("test-secret"),
		"auth-hub",
		"alt-backend",
		"service-secret",
		nil,
		30*time.Second,
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetProjectionHealth",
		strings.NewReader("{}"),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Alt-Backend-Token", createRoleToken(t, []byte("test-secret"), "user"))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestAdminProxyHandler_ServeHTTP_ForwardsServiceTokenForAdmin(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetProjectionHealth", r.URL.Path)
		assert.Equal(t, "service-secret", r.Header.Get("X-Service-Token"))
		assert.Empty(t, r.Header.Get("X-Alt-Backend-Token"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"activeVersion":1}`))
	}))
	defer backend.Close()

	backendClient := client.NewBackendClientWithTransport(
		backend.URL,
		30*time.Second,
		5*time.Minute,
		http.DefaultTransport,
	)

	handler := NewAdminProxyHandler(
		backendClient,
		[]byte("test-secret"),
		"auth-hub",
		"alt-backend",
		"service-secret",
		nil,
		30*time.Second,
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetProjectionHealth",
		strings.NewReader("{}"),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Alt-Backend-Token", createRoleToken(t, []byte("test-secret"), "admin"))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"activeVersion":1}`, recorder.Body.String())
}

func createRoleToken(t *testing.T, secret []byte, role string) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   uuid.New().String(),
		"email": "test@example.com",
		"role":  role,
		"sid":   "session-123",
		"iss":   "auth-hub",
		"aud":   []string{"alt-backend"},
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	})

	tokenStr, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	return tokenStr
}
