// ABOUTME: TDD tests for RemoteTokenRepository — asserts the X-Internal-Auth header
// ABOUTME: is sent so auth-token-manager's fail-closed /api/token check accepts the call.

package repository

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newRemoteTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestRemoteTokenRepository_GetCurrentToken_SendsInternalAuthHeader asserts that every
// request to /api/token carries the X-Internal-Auth header with the configured internal
// auth token, matching what auth-token-manager's fail-closed check requires.
func TestRemoteTokenRepository_GetCurrentToken_SendsInternalAuthHeader(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Internal-Auth")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"a","refresh_token":"r","expires_at":"2099-01-01T00:00:00Z","token_type":"bearer","scope":"read"}`))
	}))
	defer srv.Close()

	repo := NewRemoteTokenRepository(srv.URL, "expected-internal-token", newRemoteTestLogger())

	if _, err := repo.GetCurrentToken(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotHeader != "expected-internal-token" {
		t.Fatalf("expected X-Internal-Auth header %q, got %q", "expected-internal-token", gotHeader)
	}
}

// TestRemoteTokenRepository_GetCurrentToken_PropagatesUnauthorized asserts that a 401 from
// auth-token-manager (wrong/missing internal auth token) surfaces as an error rather than
// being swallowed, so a misconfigured secret is visible instead of silently starving the
// token source.
func TestRemoteTokenRepository_GetCurrentToken_PropagatesUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Unauthorized"}`))
	}))
	defer srv.Close()

	repo := NewRemoteTokenRepository(srv.URL, "wrong-token", newRemoteTestLogger())

	if _, err := repo.GetCurrentToken(context.Background()); err == nil {
		t.Fatalf("expected error on 401, got nil")
	}
}
