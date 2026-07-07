// ABOUTME: TDD tests for RemoteTokenService — surfaces auth-token-manager 404 as a typed error
// ABOUTME: and exposes degraded state so the silent-failure 2026-05-05 incident becomes observable.

package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"pre-processor-sidecar/repository"
)

// fakeClock is a deterministic Clock for tests.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time { return c.now }
func (c *fakeClock) advance(d time.Duration) {
	c.now = c.now.Add(d)
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestRemoteTokenService_GetValidToken_Returns_ErrTokenUnavailable_On_404 asserts that
// when auth-token-manager returns 404 (the exact failure mode observed during the disk-full
// incident), GetValidToken returns the typed sentinel ErrTokenUnavailable rather than an
// opaque "auth-token-manager returned status: 404" string error.
func TestRemoteTokenService_GetValidToken_Returns_ErrTokenUnavailable_On_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"No token data found"}`))
	}))
	defer srv.Close()

	repo := repository.NewRemoteTokenRepository(srv.URL, "test-internal-token", newTestLogger())
	clock := &fakeClock{now: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)}
	svc := NewRemoteTokenServiceWithClock(repo, newTestLogger(), clock)

	_, err := svc.GetValidToken(context.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrTokenUnavailable) {
		t.Fatalf("expected ErrTokenUnavailable, got %v", err)
	}
}

// TestRemoteTokenService_IsDegraded_AfterThreeFailuresWithin60s asserts that after three
// consecutive 404 failures within a 60-second window, IsDegraded() returns true. This is
// the signal that lets /admin/health expose token-source-down without polling internals.
func TestRemoteTokenService_IsDegraded_AfterThreeFailuresWithin60s(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"No token data found"}`))
	}))
	defer srv.Close()

	repo := repository.NewRemoteTokenRepository(srv.URL, "test-internal-token", newTestLogger())
	clock := &fakeClock{now: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)}
	svc := NewRemoteTokenServiceWithClock(repo, newTestLogger(), clock)

	if svc.IsDegraded() {
		t.Fatalf("expected IsDegraded()=false before any calls")
	}

	for i := 0; i < 3; i++ {
		_, _ = svc.GetValidToken(context.Background())
		clock.advance(10 * time.Second) // 0s, 10s, 20s — all within 60s window
	}

	if !svc.IsDegraded() {
		t.Fatalf("expected IsDegraded()=true after 3 failures within 60s")
	}
}

// TestRemoteTokenService_IsDegraded_FalseWhenFailuresOutsideWindow asserts that failures
// older than the 60-second rolling window are evicted: 3 failures spread across 90s
// must not flip IsDegraded.
func TestRemoteTokenService_IsDegraded_FalseWhenFailuresOutsideWindow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	repo := repository.NewRemoteTokenRepository(srv.URL, "test-internal-token", newTestLogger())
	clock := &fakeClock{now: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)}
	svc := NewRemoteTokenServiceWithClock(repo, newTestLogger(), clock)

	_, _ = svc.GetValidToken(context.Background())
	clock.advance(45 * time.Second)
	_, _ = svc.GetValidToken(context.Background())
	clock.advance(50 * time.Second) // first failure now 95s old, evicted
	_, _ = svc.GetValidToken(context.Background())

	if svc.IsDegraded() {
		t.Fatalf("expected IsDegraded()=false when only 2 failures remain in the 60s window")
	}
}

// TestRemoteTokenService_IsDegraded_RecoversAfterSuccess asserts that a single successful
// 200 response after a degraded streak clears the failure window so the alarm is silenced
// on actual recovery (the operator-visible counterpart of "circuit breaker CLOSED").
func TestRemoteTokenService_IsDegraded_RecoversAfterSuccess(t *testing.T) {
	var failing atomic.Bool
	failing.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"a","refresh_token":"r","expires_at":"2099-01-01T00:00:00Z","token_type":"bearer","scope":"read"}`))
	}))
	defer srv.Close()

	repo := repository.NewRemoteTokenRepository(srv.URL, "test-internal-token", newTestLogger())
	clock := &fakeClock{now: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)}
	svc := NewRemoteTokenServiceWithClock(repo, newTestLogger(), clock)

	for i := 0; i < 3; i++ {
		_, _ = svc.GetValidToken(context.Background())
		clock.advance(5 * time.Second)
	}
	if !svc.IsDegraded() {
		t.Fatalf("expected IsDegraded()=true after 3 failures")
	}

	failing.Store(false)
	tok, err := svc.GetValidToken(context.Background())
	if err != nil {
		t.Fatalf("expected success after recovery, got err=%v", err)
	}
	if tok == nil || tok.AccessToken == "" {
		t.Fatalf("expected non-empty token after recovery, got %+v", tok)
	}
	if svc.IsDegraded() {
		t.Fatalf("expected IsDegraded()=false after a successful response")
	}
}

// TestRemoteTokenService_GetValidToken_Returns_ErrTokenUnavailable_On_EmptyAccessToken
// asserts that a 200 response with an empty access_token (the second flavor of "no token
// available") also surfaces as ErrTokenUnavailable. Covers the case where the repo writes
// the env file but the token field is blank, mirroring repository.ErrTokenNotFound.
func TestRemoteTokenService_GetValidToken_Returns_ErrTokenUnavailable_On_EmptyAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.Copy(w, strings.NewReader(`{"access_token":"","refresh_token":"","expires_at":"2099-01-01T00:00:00Z","token_type":"bearer"}`))
	}))
	defer srv.Close()

	repo := repository.NewRemoteTokenRepository(srv.URL, "test-internal-token", newTestLogger())
	clock := &fakeClock{now: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)}
	svc := NewRemoteTokenServiceWithClock(repo, newTestLogger(), clock)

	_, err := svc.GetValidToken(context.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrTokenUnavailable) {
		t.Fatalf("expected ErrTokenUnavailable, got %v", err)
	}
}
