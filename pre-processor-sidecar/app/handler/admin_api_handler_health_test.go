// ABOUTME: TDD tests for /admin/health — the observable signal that was missing during the
// ABOUTME: 2026-05-05 to 2026-05-08 silent Inoreader outage. Surfaces ingestion staleness,
// ABOUTME: circuit-breaker state, and token availability so external watchers can alert.

package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeIngestionHealth struct {
	last     time.Time
	cbState  string
	tokenAvl bool
}

func (f *fakeIngestionHealth) LastSuccessfulFetch() time.Time { return f.last }
func (f *fakeIngestionHealth) CircuitBreakerState() string    { return f.cbState }
func (f *fakeIngestionHealth) TokenAvailable() bool           { return f.tokenAvl }

type fakeHealthClock struct{ now time.Time }

func (c *fakeHealthClock) Now() time.Time { return c.now }

func newHealthTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// healthResponse mirrors the JSON shape we promise for /admin/health.
type healthResponse struct {
	Status                          string `json:"status"`
	TokenAvailable                  bool   `json:"token_available"`
	CircuitBreakerState             string `json:"circuit_breaker_state"`
	LastSuccessfulFetchAt           string `json:"last_successful_fetch_at"`
	SecondsSinceLastFetch           int64  `json:"seconds_since_last_fetch"`
	IngestionSilentThresholdSeconds int64  `json:"ingestion_silent_threshold_seconds"`
	IngestionSilent                 bool   `json:"ingestion_silent"`
}

// TestHandleHealth_Ok_When_FreshAndTokenAvailable: the happy path. Recent fetch + valid
// token + CB CLOSED ⇒ status="ok", ingestion_silent=false.
func TestHandleHealth_Ok_When_FreshAndTokenAvailable(t *testing.T) {
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	provider := &fakeIngestionHealth{
		last:     now.Add(-5 * time.Minute),
		cbState:  "CLOSED",
		tokenAvl: true,
	}
	h := NewHealthHandler(provider, &fakeHealthClock{now: now}, 1800, newHealthTestLogger())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/health", nil)
	h.HandleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("status=%q, want ok", resp.Status)
	}
	if !resp.TokenAvailable {
		t.Fatalf("token_available=false, want true")
	}
	if resp.CircuitBreakerState != "CLOSED" {
		t.Fatalf("circuit_breaker_state=%q, want CLOSED", resp.CircuitBreakerState)
	}
	if resp.IngestionSilent {
		t.Fatalf("ingestion_silent=true, want false")
	}
	if resp.SecondsSinceLastFetch != 300 {
		t.Fatalf("seconds_since_last_fetch=%d, want 300", resp.SecondsSinceLastFetch)
	}
	if resp.IngestionSilentThresholdSeconds != 1800 {
		t.Fatalf("threshold=%d, want 1800", resp.IngestionSilentThresholdSeconds)
	}
}

// TestHandleHealth_Degraded_When_StaleFetch: last_successful_fetch_at older than threshold
// ⇒ status="degraded", ingestion_silent=true. This is the signal that would have alerted
// during the 2026-05-05 silent outage.
func TestHandleHealth_Degraded_When_StaleFetch(t *testing.T) {
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	provider := &fakeIngestionHealth{
		last:     now.Add(-90 * time.Minute), // 5400s, threshold 1800s
		cbState:  "OPEN",
		tokenAvl: false,
	}
	h := NewHealthHandler(provider, &fakeHealthClock{now: now}, 1800, newHealthTestLogger())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/health", nil)
	h.HandleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (degraded is reported via body, not status code), got %d", rec.Code)
	}
	var resp healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Status != "degraded" {
		t.Fatalf("status=%q, want degraded", resp.Status)
	}
	if !resp.IngestionSilent {
		t.Fatalf("ingestion_silent=false, want true")
	}
	if resp.SecondsSinceLastFetch != 5400 {
		t.Fatalf("seconds_since_last_fetch=%d, want 5400", resp.SecondsSinceLastFetch)
	}
	if resp.TokenAvailable {
		t.Fatalf("token_available=true, want false")
	}
}

// TestHandleHealth_Degraded_When_TokenUnavailable: even on a fresh fetch, missing token
// ⇒ status="degraded". (The 2026-05-05 incident: token disappeared while last_sync was
// still recent on the very first tick after the disk-full event.)
func TestHandleHealth_Degraded_When_TokenUnavailable(t *testing.T) {
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	provider := &fakeIngestionHealth{
		last:     now.Add(-5 * time.Minute),
		cbState:  "OPEN",
		tokenAvl: false,
	}
	h := NewHealthHandler(provider, &fakeHealthClock{now: now}, 1800, newHealthTestLogger())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/health", nil)
	h.HandleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Status != "degraded" {
		t.Fatalf("status=%q, want degraded (token missing)", resp.Status)
	}
	if resp.IngestionSilent {
		t.Fatalf("ingestion_silent=true, want false (fresh fetch)")
	}
	if resp.TokenAvailable {
		t.Fatalf("token_available=true, want false")
	}
}

// TestHandleHealth_RejectsNonGet: only GET is accepted.
func TestHandleHealth_RejectsNonGet(t *testing.T) {
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	provider := &fakeIngestionHealth{last: now, cbState: "CLOSED", tokenAvl: true}
	h := NewHealthHandler(provider, &fakeHealthClock{now: now}, 1800, newHealthTestLogger())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/health", nil)
	h.HandleHealth(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
