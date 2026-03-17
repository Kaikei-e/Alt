package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"alt-butterfly-facade/internal/client"
)

const testRESTSecret = "test-rest-secret"
const testRESTIssuer = "auth-hub"
const testRESTAudience = "alt-backend"

func createTestJWT(t *testing.T) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": "00000000-0000-0000-0000-000000000001",
		"iss": testRESTIssuer,
		"aud": []string{testRESTAudience},
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
		"email": "test@example.com",
		"role":  "user",
		"sid":   "test-session",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testRESTSecret))
	if err != nil {
		t.Fatalf("failed to sign JWT: %v", err)
	}
	return signed
}

func newTestRESTProxyHandler(t *testing.T, backendURL string) *RESTProxyHandler {
	t.Helper()
	logger := slog.Default()
	backendClient := client.NewBackendClientWithTransport(
		backendURL,
		30*time.Second,
		30*time.Second,
		http.DefaultTransport,
	)
	return NewRESTProxyHandler(
		backendClient,
		[]byte(testRESTSecret),
		testRESTIssuer,
		testRESTAudience,
		logger,
		30*time.Second,
	)
}

func TestRESTProxyHandler_MissingToken(t *testing.T) {
	handler := newTestRESTProxyHandler(t, "http://localhost:1")

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRESTProxyHandler_InvalidToken(t *testing.T) {
	handler := newTestRESTProxyHandler(t, "http://localhost:1")

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	req.Header.Set("X-Alt-Backend-Token", "invalid-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRESTProxyHandler_GET_Forward(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/health" {
			t.Errorf("expected /v1/health, got %s", r.URL.Path)
		}
		// Verify token is forwarded
		if r.Header.Get("X-Alt-Backend-Token") == "" {
			t.Error("expected X-Alt-Backend-Token header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer backend.Close()

	handler := newTestRESTProxyHandler(t, backend.URL)
	token := createTestJWT(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json, got %s", ct)
	}
}

func TestRESTProxyHandler_POST_Forward(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"key":"value"}` {
			t.Errorf("unexpected body: %s", string(body))
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json Content-Type, got %s", ct)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"123"}`))
	}))
	defer backend.Close()

	handler := newTestRESTProxyHandler(t, backend.URL)
	token := createTestJWT(t)

	body := strings.NewReader(`{"key":"value"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/feeds", body)
	req.Header.Set("X-Alt-Backend-Token", token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}
}

func TestRESTProxyHandler_PUT_Forward(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	handler := newTestRESTProxyHandler(t, backend.URL)
	token := createTestJWT(t)

	req := httptest.NewRequest(http.MethodPut, "/v1/feeds/1", strings.NewReader(`{"name":"updated"}`))
	req.Header.Set("X-Alt-Backend-Token", token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRESTProxyHandler_DELETE_Forward(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/v1/feeds/123" {
			t.Errorf("expected /v1/feeds/123, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()

	handler := newTestRESTProxyHandler(t, backend.URL)
	token := createTestJWT(t)

	req := httptest.NewRequest(http.MethodDelete, "/v1/feeds/123", nil)
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestRESTProxyHandler_HeaderForwarding(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers are forwarded
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("expected Accept header forwarded")
		}
		if r.Header.Get("X-Alt-Backend-Token") == "" {
			t.Error("expected X-Alt-Backend-Token header")
		}
		// Response headers
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "req-123")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer backend.Close()

	handler := newTestRESTProxyHandler(t, backend.URL)
	token := createTestJWT(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
	req.Header.Set("X-Alt-Backend-Token", token)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type forwarded, got %s", ct)
	}
}

func TestRESTProxyHandler_QueryParams(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "window=24h&limit=10" {
			t.Errorf("expected query params forwarded, got %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer backend.Close()

	handler := newTestRESTProxyHandler(t, backend.URL)
	token := createTestJWT(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard/overview?window=24h&limit=10", nil)
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRESTProxyHandler_MultipartFormData(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("expected multipart/form-data, got %s", r.Header.Get("Content-Type"))
		}
		// Parse the multipart form
		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("failed to get form file: %v", err)
		}
		defer file.Close()
		if header.Filename != "feeds.opml" {
			t.Errorf("expected filename feeds.opml, got %s", header.Filename)
		}
		content, _ := io.ReadAll(file)
		if !strings.Contains(string(content), "<opml") {
			t.Errorf("expected OPML content, got %s", string(content))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"imported":5}`))
	}))
	defer backend.Close()

	handler := newTestRESTProxyHandler(t, backend.URL)
	token := createTestJWT(t)

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "feeds.opml")
	if err != nil {
		t.Fatal(err)
	}
	part.Write([]byte(`<opml version="2.0"><head><title>Test</title></head><body></body></opml>`))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/rss-feed-link/import/opml", &buf)
	req.Header.Set("X-Alt-Backend-Token", token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRESTProxyHandler_BackendError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer backend.Close()

	handler := newTestRESTProxyHandler(t, backend.URL)
	token := createTestJWT(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
	req.Header.Set("X-Alt-Backend-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Should forward backend error as-is
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}
