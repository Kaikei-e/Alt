package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
)

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(10), 10) // 10 req/s, burst 10

	e := echo.New()
	e.Use(rl.Middleware())
	e.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRateLimiter_RejectsOverLimit(t *testing.T) {
	// 1 req/s, burst 1: second request should be rejected
	rl := NewRateLimiter(rate.Limit(1), 1)

	e := echo.New()
	e.Use(rl.Middleware())
	e.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	// First request: allowed
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec1 := httptest.NewRecorder()
	e.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)

	// Second request (immediate): rejected
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
}

func TestRateLimiter_RetryAfterHeader(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 1)

	e := echo.New()
	e.Use(rl.Middleware())
	e.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	// Exhaust the burst
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec1 := httptest.NewRecorder()
	e.ServeHTTP(rec1, req1)

	// Second request triggers rate limit
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)

	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
	assert.NotEmpty(t, rec2.Header().Get("Retry-After"))
}

func TestRateLimiter_DifferentIPsGetSeparateLimits(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 1)

	e := echo.New()
	e.Use(rl.Middleware())
	e.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	// First IP first request: allowed
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "1.2.3.4:1234"
	rec1 := httptest.NewRecorder()
	e.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)

	// Second IP first request: allowed (separate limiter)
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "5.6.7.8:5678"
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)

	// First IP second request: rejected
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.RemoteAddr = "1.2.3.4:1234"
	rec3 := httptest.NewRecorder()
	e.ServeHTTP(rec3, req3)
	assert.Equal(t, http.StatusTooManyRequests, rec3.Code)
}

func TestRateLimiter_IgnoresSpoofedXForwardedFor(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 1)

	e := echo.New()
	e.Use(rl.Middleware())
	e.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	// First request through proxy 172.18.0.5 where client 198.51.100.42 spoofs leftmost XFF: allowed
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "172.18.0.5:43210"
	req1.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.42")
	rec1 := httptest.NewRecorder()
	e.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)

	// Second request from same client 198.51.100.42 rotating leftmost spoofed XFF: must be rate-limited (429)
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "172.18.0.5:43210"
	req2.Header.Set("X-Forwarded-For", "203.0.113.2, 198.51.100.42")
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
}
