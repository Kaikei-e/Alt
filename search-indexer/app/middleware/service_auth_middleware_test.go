package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"search-indexer/logger"
)

func TestMain(m *testing.M) {
	logger.Init()
	os.Exit(m.Run())
}

func newTestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	})
}

func TestRequireServiceAuth_RejectsMissingHeader(t *testing.T) {
	t.Parallel()

	m := NewServiceAuthMiddleware("correct-secret")
	handler := m.RequireServiceAuth(newTestHandler())

	req := httptest.NewRequest(http.MethodGet, "/v1/search?q=test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: got status %d, want 401", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "correct-secret") {
		t.Fatal("response body leaks the configured secret")
	}
}

func TestRequireServiceAuth_RejectsWrongToken(t *testing.T) {
	t.Parallel()

	m := NewServiceAuthMiddleware("correct-secret")
	handler := m.RequireServiceAuth(newTestHandler())

	req := httptest.NewRequest(http.MethodGet, "/v1/search", nil)
	req.Header.Set("X-Service-Token", "wrong-secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: got status %d, want 401", rec.Code)
	}
}

func TestRequireServiceAuth_RejectsShorterToken(t *testing.T) {
	t.Parallel()

	// Constant-time comparison must still reject length-mismatched tokens.
	m := NewServiceAuthMiddleware("correct-secret")
	handler := m.RequireServiceAuth(newTestHandler())

	req := httptest.NewRequest(http.MethodGet, "/v1/search", nil)
	req.Header.Set("X-Service-Token", "c")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("short token: got status %d, want 401", rec.Code)
	}
}

func TestRequireServiceAuth_AcceptsCorrectToken(t *testing.T) {
	t.Parallel()

	m := NewServiceAuthMiddleware("correct-secret")
	handler := m.RequireServiceAuth(newTestHandler())

	req := httptest.NewRequest(http.MethodGet, "/v1/search?q=test", nil)
	req.Header.Set("X-Service-Token", "correct-secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("valid token: got status %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "ok" {
		t.Fatalf("valid token: body = %q, want %q", got, "ok")
	}
}

func TestRequireServiceAuth_EmptyServerSecretFailsClosed(t *testing.T) {
	t.Parallel()

	// If SERVICE_TOKEN is not configured, every request must fail (fail-closed),
	// even when the client presents what would otherwise be a valid empty token.
	m := NewServiceAuthMiddleware("")
	handler := m.RequireServiceAuth(newTestHandler())

	cases := []string{"", "any-value"}
	for _, tok := range cases {
		req := httptest.NewRequest(http.MethodGet, "/v1/search", nil)
		if tok != "" {
			req.Header.Set("X-Service-Token", tok)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			t.Fatalf("unconfigured secret accepted token=%q", tok)
		}
	}
}
