package handler

import (
	"io"
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

	handler := NewProxyHandler(backendClient, secret, "auth-hub", "alt-backend", nil)

	assert.NotNil(t, handler)
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

	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", nil)

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

	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", nil)

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

	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", nil)

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

	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", nil)

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

	handler := NewProxyHandler(backendClient, []byte("test-secret"), "auth-hub", "alt-backend", nil)

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
