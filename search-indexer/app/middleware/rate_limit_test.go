package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestRateLimit_AllowsBelowLimit(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(rate.Limit(10), 5)
	handler := rl.Middleware(newTestHandler())

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/search", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i, rec.Code)
		}
	}
}

func TestRateLimit_RejectsBurstOverCapacity(t *testing.T) {
	t.Parallel()
	// Very small bucket: burst=2, refill=1/s. Three immediate requests -> the
	// third is rejected with 429.
	rl := NewRateLimiter(rate.Limit(1), 2)
	handler := rl.Middleware(newTestHandler())

	statuses := make([]int, 3)
	for i := range statuses {
		req := httptest.NewRequest(http.MethodGet, "/v1/search", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		statuses[i] = rec.Code
	}

	got429 := 0
	for _, s := range statuses {
		if s == http.StatusTooManyRequests {
			got429++
		}
	}
	if got429 == 0 {
		t.Fatalf("expected at least one 429 once burst was exhausted, got %v", statuses)
	}
}

func TestRateLimit_SetsRetryAfterHeader(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(rate.Limit(1), 1)
	handler := rl.Middleware(newTestHandler())

	// Consume the token.
	r1 := httptest.NewRequest(http.MethodGet, "/v1/search", nil)
	handler.ServeHTTP(httptest.NewRecorder(), r1)

	// Immediate second call is rejected.
	r2 := httptest.NewRequest(http.MethodGet, "/v1/search", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r2)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After header missing on 429")
	}
}

// Make go vet happy about unused imports on non-test builds.
var _ = time.Second
