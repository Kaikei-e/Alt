package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"search-indexer/config"
	appOtel "search-indexer/utils/otel"
)

func TestHTTPServer_DeepHealthFailsWhenMeilisearchDown(t *testing.T) {
	srv := newHTTPServer(nil, nil, appOtel.Config{}, config.RateLimitConfig{RequestsPerSecond: 100, Burst: 100},
		func(context.Context) error { return context.DeadlineExceeded })

	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/deep", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("deep = %d, want 503", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "fail" {
		t.Errorf("status = %v", body["status"])
	}

	rec = httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("cheap /health = %d, want 200", rec.Code)
	}
}

func TestHTTPServer_DeepHealthPassesWhenMeilisearchUp(t *testing.T) {
	srv := newHTTPServer(nil, nil, appOtel.Config{}, config.RateLimitConfig{RequestsPerSecond: 100, Burst: 100},
		func(context.Context) error { return nil })

	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/deep", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("deep = %d, want 200", rec.Code)
	}
}

func TestHTTPServer_DeepHealthNeverReturns429(t *testing.T) {
	srv := newHTTPServer(nil, nil, appOtel.Config{}, config.RateLimitConfig{RequestsPerSecond: 1, Burst: 1},
		func(context.Context) error { return nil })

	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/deep", nil))
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("GET /health/deep returned 429 on call %d; deep must not share the search token bucket", i+1)
		}
		if rec.Code != http.StatusOK && rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("deep = %d, want 200 or 503 envelope", rec.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("deep body is not JSON: %v (%q)", err, rec.Body.String())
		}
		status, _ := body["status"].(string)
		switch status {
		case "pass", "warn", "fail":
		default:
			t.Fatalf("status = %v, want pass|warn|fail envelope", body["status"])
		}
	}
}
