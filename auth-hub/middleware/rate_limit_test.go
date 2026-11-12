package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
)

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(10), 10)

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
	rl := NewRateLimiter(rate.Limit(1), 1)

	e := echo.New()
	e.Use(rl.Middleware())
	e.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec1 := httptest.NewRecorder()
	e.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)

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

	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec1 := httptest.NewRecorder()
	e.ServeHTTP(rec1, req1)

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

	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "1.2.3.4:1234"
	rec1 := httptest.NewRecorder()
	e.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)

	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "5.6.7.8:5678"
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)

	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.RemoteAddr = "1.2.3.4:1234"
	rec3 := httptest.NewRecorder()
	e.ServeHTTP(rec3, req3)
	assert.Equal(t, http.StatusTooManyRequests, rec3.Code)
}

func TestRateLimiter_TrustedProxyIgnoresSpoofedLeftmostXFF(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 1)

	e := echo.New()
	e.IPExtractor = NewTrustedProxyIPExtractor([]netip.Prefix{
		netip.MustParsePrefix("172.18.0.0/16"),
	})
	e.Use(rl.Middleware())
	e.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, c.RealIP())
	})

	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "172.18.0.5:43210"
	req1.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.42")
	rec1 := httptest.NewRecorder()
	e.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)
	assert.Equal(t, "198.51.100.42", rec1.Body.String())

	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "172.18.0.5:43210"
	req2.Header.Set("X-Forwarded-For", "203.0.113.2, 198.51.100.42")
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)

	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.RemoteAddr = "172.18.0.5:43210"
	req3.Header.Set("X-Forwarded-For", "203.0.113.9, 198.51.100.99, 172.18.0.10")
	rec3 := httptest.NewRecorder()
	e.ServeHTTP(rec3, req3)
	assert.Equal(t, http.StatusOK, rec3.Code)
	assert.Equal(t, "198.51.100.99", rec3.Body.String())
}

func TestRateLimiter_DirectUntrustedPeerIgnoresXFFEntirely(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 1)

	e := echo.New()
	e.IPExtractor = NewTrustedProxyIPExtractor([]netip.Prefix{
		netip.MustParsePrefix("172.18.0.5/32"),
	})
	e.Use(rl.Middleware())
	e.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, c.RealIP())
	})

	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "10.20.30.40:54321"
	req1.Header.Set("X-Forwarded-For", "198.51.100.1")
	rec1 := httptest.NewRecorder()
	e.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)
	assert.Equal(t, "10.20.30.40", rec1.Body.String())

	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "10.20.30.40:54321"
	req2.Header.Set("X-Forwarded-For", "198.51.100.2")
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
}

func TestTrustedProxyIPExtractor_FallbacksToSocketPeer(t *testing.T) {
	extractor := NewTrustedProxyIPExtractor([]netip.Prefix{
		netip.MustParsePrefix("172.18.0.0/16"),
	})

	t.Run("trusted peer without XFF uses socket peer IP", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "172.18.0.5:43210"
		assert.Equal(t, "172.18.0.5", extractor(req))
	})

	t.Run("trusted peer with only trusted CIDRs in XFF uses socket peer IP", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "172.18.0.5:43210"
		req.Header.Set("X-Forwarded-For", "172.18.0.9, 172.18.0.10")
		assert.Equal(t, "172.18.0.5", extractor(req))
	})

	t.Run("trusted peer with malformed XFF entry falls back to socket peer IP", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "172.18.0.5:43210"
		req.Header.Set("X-Forwarded-For", "198.51.100.1, garbage")
		assert.Equal(t, "172.18.0.5", extractor(req))
	})

	t.Run("unconfigured Echo router ignores XFF in RateLimiter middleware", func(t *testing.T) {
		rl := NewRateLimiter(rate.Limit(1), 1)
		e := echo.New()
		e.Use(rl.Middleware())
		e.GET("/test", func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})

		req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req1.RemoteAddr = "172.18.0.99:11111"
		req1.Header.Set("X-Forwarded-For", "203.0.113.10")
		rec1 := httptest.NewRecorder()
		e.ServeHTTP(rec1, req1)
		assert.Equal(t, http.StatusOK, rec1.Code)

		req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req2.RemoteAddr = "172.18.0.99:11111"
		req2.Header.Set("X-Forwarded-For", "203.0.113.11")
		rec2 := httptest.NewRecorder()
		e.ServeHTTP(rec2, req2)
		assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
	})
}

func TestTrustedProxyIPExtractor_NoneModeIgnoresXFFFromAnyPeer(t *testing.T) {
	extractor := NewTrustedProxyIPExtractor(nil)

	peers := []struct {
		remoteAddr string
		wantIP     string
	}{
		{remoteAddr: "127.0.0.1:12345", wantIP: "127.0.0.1"},
		{remoteAddr: "172.18.0.5:43210", wantIP: "172.18.0.5"},
		{remoteAddr: "10.0.0.5:8080", wantIP: "10.0.0.5"},
		{remoteAddr: "198.51.100.7:9000", wantIP: "198.51.100.7"},
		{remoteAddr: "[::1]:43210", wantIP: "::1"},
	}

	for _, tc := range peers {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = tc.remoteAddr
		req.Header.Set("X-Forwarded-For", "203.0.113.55, 198.51.100.42")
		assert.Equal(t, tc.wantIP, extractor(req), "peer %s", tc.remoteAddr)
	}
}

func TestTrustedProxyIPExtractor_EdgeCases(t *testing.T) {
	extractor := NewTrustedProxyIPExtractor([]netip.Prefix{
		netip.MustParsePrefix("172.18.0.0/16"),
		netip.MustParsePrefix("fd00::/8"),
	})

	t.Run("IPv4-mapped IPv6 peer is matched by IPv4 CIDR", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "[::ffff:172.18.0.5]:43210"
		req.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.42")
		assert.Equal(t, "198.51.100.42", extractor(req))
	})

	t.Run("multiple X-Forwarded-For header lines are walked right to left", func(t *testing.T) {
		req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req1.RemoteAddr = "172.18.0.5:43210"
		req1.Header.Add("X-Forwarded-For", "203.0.113.10, 198.51.100.20")
		req1.Header.Add("X-Forwarded-For", "198.51.100.30, 172.18.0.9")
		assert.Equal(t, "198.51.100.30", extractor(req1))

		req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req2.RemoteAddr = "172.18.0.5:43210"
		req2.Header.Add("X-Forwarded-For", "203.0.113.10, 198.51.100.20")
		req2.Header.Add("X-Forwarded-For", "172.18.0.8, 172.18.0.9")
		assert.Equal(t, "198.51.100.20", extractor(req2))
	})

	t.Run("extracts IPv6 client entry", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "172.18.0.5:43210"
		req.Header.Set("X-Forwarded-For", "2001:db8::1, 2001:db8::cafe, fd00::10")
		assert.Equal(t, "2001:db8::cafe", extractor(req))
	})

	t.Run("entry with port falls back to proxy socket peer address", func(t *testing.T) {
		for _, xff := range []string{"198.51.100.42:8080", "[2001:db8::cafe]:8443"} {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = "172.18.0.5:43210"
			req.Header.Set("X-Forwarded-For", xff)
			assert.Equal(t, "172.18.0.5", extractor(req), "xff %s", xff)
		}
	})
}
