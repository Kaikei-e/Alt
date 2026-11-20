package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"auth-hub/config"
	appmiddleware "auth-hub/middleware"

	echootel "github.com/labstack/echo-otel/v5"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"golang.org/x/time/rate"
)

// wireInternalAuth is the single fail-closed choke point for /internal auth
// wiring. It must never return a no-op middleware: an empty secret is a
// startup bug (config.Validate() should have already rejected it), not a
// signal to skip auth (CLAUDE.md Rule 8).
func TestWireInternalAuth_PanicsOnEmptySecret(t *testing.T) {
	assert.Panics(t, func() {
		wireInternalAuth(&config.Config{})
	})
}

func TestWireInternalAuth_FailsClosedWithoutHeader(t *testing.T) {
	mw := wireInternalAuth(separatedSecretsConfig())

	e := echo.New()
	e.Use(mw)
	e.GET("/internal/system-user", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/internal/system-user", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestWireInternalAuth_AllowsValidSecret(t *testing.T) {
	cfg := separatedSecretsConfig()
	mw := wireInternalAuth(cfg)

	e := echo.New()
	e.Use(mw)
	e.GET("/internal/system-user", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/internal/system-user", nil)
	req.Header.Set("X-Internal-Auth", cfg.InternalAuthSecret)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestBuildPlaintextHandler_DefaultRejectsBusinessRoutes(t *testing.T) {
	e := echo.New()
	e.GET("/health", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	e.GET("/session", func(c *echo.Context) error {
		return c.String(http.StatusOK, "session-data")
	})
	e.GET("/validate", func(c *echo.Context) error {
		return c.String(http.StatusOK, "validate-data")
	})
	e.POST("/csrf", func(c *echo.Context) error {
		return c.String(http.StatusOK, "csrf-token")
	})
	e.GET("/internal/system-user", func(c *echo.Context) error {
		return c.String(http.StatusOK, "system-user-id")
	})

	// Default: devPlaintextAllowed is false
	handler := buildPlaintextHandler(e, false)

	// Health check MUST succeed on plaintext (bootstrap availability)
	reqHealth := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil)
	recHealth := httptest.NewRecorder()
	handler.ServeHTTP(recHealth, reqHealth)
	assert.Equal(t, http.StatusOK, recHealth.Code)

	// Business routes MUST be rejected on plaintext (fail-closed A04)
	businessEndpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/session"},
		{http.MethodGet, "/validate"},
		{http.MethodPost, "/csrf"},
		{http.MethodGet, "/internal/system-user"},
	}

	for _, ep := range businessEndpoints {
		req := httptest.NewRequestWithContext(t.Context(), ep.method, ep.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code, "endpoint %s must be forbidden on plaintext listener", ep.path)
		assert.Contains(t, rec.Body.String(), "business endpoints require HTTPS")
	}
}

func TestBuildPlaintextHandler_ExplicitDevPermitsRoutes(t *testing.T) {
	e := echo.New()
	e.GET("/session", func(c *echo.Context) error {
		return c.String(http.StatusOK, "dev-session-data")
	})

	// Explicit dev opt-in allows business routes
	handler := buildPlaintextHandler(e, true)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/session", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "dev-session-data", rec.Body.String())
}

func TestConfigureTrustedProxies_WiresExtractorAndLogsAtStartup(t *testing.T) {
	assert.Panics(t, func() {
		configureTrustedProxies(context.Background(), echo.New(), &config.Config{}, slog.Default())
	})
	assert.Panics(t, func() {
		configureTrustedProxies(context.Background(), echo.New(), &config.Config{
			TrustedProxiesDisabled: true,
			TrustedProxyCIDRs:      []netip.Prefix{netip.MustParsePrefix("172.18.0.5/32")},
		}, slog.Default())
	})

	var logBuf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logBuf, nil))
	e := echo.New()
	cidrs := []netip.Prefix{
		netip.MustParsePrefix("172.18.0.5/32"),
	}

	configureTrustedProxies(context.Background(), e, &config.Config{TrustedProxyCIDRs: cidrs}, log)

	assert.Contains(t, logBuf.String(), "trusted_proxy_cidrs_configured")
	assert.Contains(t, logBuf.String(), "172.18.0.5/32")
	assert.NotContains(t, logBuf.String(), "trusted_proxy_cidrs_disabled")

	e.GET("/ip", func(c *echo.Context) error {
		return c.String(http.StatusOK, c.RealIP())
	})

	trustedReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ip", nil)
	trustedReq.RemoteAddr = "172.18.0.5:44444"
	trustedReq.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.77")
	trustedRec := httptest.NewRecorder()
	e.ServeHTTP(trustedRec, trustedReq)
	assert.Equal(t, "198.51.100.77", trustedRec.Body.String())

	untrustedReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ip", nil)
	untrustedReq.RemoteAddr = "172.18.0.99:44444"
	untrustedReq.Header.Set("X-Forwarded-For", "198.51.100.77")
	untrustedRec := httptest.NewRecorder()
	e.ServeHTTP(untrustedRec, untrustedReq)
	assert.Equal(t, "172.18.0.99", untrustedRec.Body.String())
}

func TestConfigureTrustedProxies_NoneLogsDisabledAndIgnoresXFF(t *testing.T) {
	var logBuf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logBuf, nil))
	e := echo.New()

	configureTrustedProxies(context.Background(), e, &config.Config{TrustedProxiesDisabled: true}, log)

	assert.Contains(t, logBuf.String(), "trusted_proxy_cidrs_disabled")
	assert.NotContains(t, logBuf.String(), "trusted_proxy_cidrs_configured")

	e.GET("/ip", func(c *echo.Context) error {
		return c.String(http.StatusOK, c.RealIP())
	})

	for _, peer := range []struct {
		remoteAddr string
		wantIP     string
	}{
		{remoteAddr: "127.0.0.1:44444", wantIP: "127.0.0.1"},
		{remoteAddr: "172.18.0.5:44444", wantIP: "172.18.0.5"},
		{remoteAddr: "198.51.100.7:44444", wantIP: "198.51.100.7"},
	} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ip", nil)
		req.RemoteAddr = peer.remoteAddr
		req.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.77")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, peer.wantIP, rec.Body.String())
	}
}

func TestEchoMiddlewareChain_ClientIPResolutionAndRateLimit(t *testing.T) {
	var logBuf bytes.Buffer
	appLog := slog.New(slog.NewJSONHandler(&logBuf, nil))
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	e := echo.New()
	e.Logger = appLog
	configureTrustedProxies(context.Background(), e, &config.Config{
		TrustedProxyCIDRs: []netip.Prefix{netip.MustParsePrefix("172.18.0.0/16")},
	}, appLog)

	e.Use(appmiddleware.SecurityHeaders())
	e.Use(echootel.NewMiddlewareWithConfig(echootel.Config{
		ServerName:     "auth-hub",
		Skipper:        appmiddleware.OTelSkipper,
		TracerProvider: tp,
		Propagators:    propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}),
	}))
	e.Use(appmiddleware.OTelStatusMiddleware())
	e.Use(newRequestLoggerMiddleware())
	e.Use(middleware.Recover())

	rl := appmiddleware.NewRateLimiter(rate.Limit(1), 1)
	e.GET("/validate", func(c *echo.Context) error {
		return c.String(http.StatusOK, c.RealIP())
	}, rl.Middleware())

	// Untrusted peer carrying both X-Forwarded-For and X-Real-IP resolves to socket peer.
	untrusted1 := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/validate", nil)
	untrusted1.RemoteAddr = "198.51.100.10:12345"
	untrusted1.Header.Set("X-Forwarded-For", "203.0.113.50")
	untrusted1.Header.Set("X-Real-IP", "203.0.113.51")
	recU1 := httptest.NewRecorder()
	e.ServeHTTP(recU1, untrusted1)
	require.Equal(t, http.StatusOK, recU1.Code)
	assert.Equal(t, "198.51.100.10", recU1.Body.String())

	// Second request from same untrusted peer with different spoofed headers hits the same bucket.
	untrusted2 := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/validate", nil)
	untrusted2.RemoteAddr = "198.51.100.10:12345"
	untrusted2.Header.Set("X-Forwarded-For", "203.0.113.60")
	untrusted2.Header.Set("X-Real-IP", "203.0.113.61")
	recU2 := httptest.NewRecorder()
	e.ServeHTTP(recU2, untrusted2)
	assert.Equal(t, http.StatusTooManyRequests, recU2.Code)

	// Trusted proxy CIDR carrying X-Forwarded-For (and X-Real-IP) resolves to the forwarded client.
	trusted1 := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/validate", nil)
	trusted1.RemoteAddr = "172.18.0.5:43210"
	trusted1.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.77")
	trusted1.Header.Set("X-Real-IP", "203.0.113.99")
	recT1 := httptest.NewRecorder()
	e.ServeHTTP(recT1, trusted1)
	require.Equal(t, http.StatusOK, recT1.Code)
	assert.Equal(t, "198.51.100.77", recT1.Body.String())

	// Second request for same forwarded client through trusted proxy hits the client's rate limit.
	trusted2 := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/validate", nil)
	trusted2.RemoteAddr = "172.18.0.5:43210"
	trusted2.Header.Set("X-Forwarded-For", "203.0.113.2, 198.51.100.77")
	recT2 := httptest.NewRecorder()
	e.ServeHTTP(recT2, trusted2)
	assert.Equal(t, http.StatusTooManyRequests, recT2.Code)
}

func TestRequestLoggerMiddleware_LogsSameKeysAndSkipsHealth(t *testing.T) {
	var logBuf bytes.Buffer
	appLog := slog.New(slog.NewJSONHandler(&logBuf, nil))

	e := echo.New()
	e.Logger = appLog
	e.Use(newRequestLoggerMiddleware())
	e.Use(middleware.Recover())

	e.GET("/health", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "healthy"})
	})
	e.GET("/session", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	e.GET("/fail", func(_ *echo.Context) error {
		return echo.NewHTTPError(http.StatusUnauthorized, "session cookie not found")
	})

	// /health must be skipped
	recHealth := httptest.NewRecorder()
	e.ServeHTTP(recHealth, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil))
	require.Equal(t, http.StatusOK, recHealth.Code)
	assert.Empty(t, strings.TrimSpace(logBuf.String()))

	// /session logs request completed with exact keys
	recOK := httptest.NewRecorder()
	e.ServeHTTP(recOK, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/session", nil))
	require.Equal(t, http.StatusOK, recOK.Code)

	lines := strings.Split(strings.TrimSpace(logBuf.String()), "\n")
	require.Len(t, lines, 1)
	var okEntry map[string]any
	okDec := json.NewDecoder(strings.NewReader(lines[0]))
	okDec.UseNumber()
	require.NoError(t, okDec.Decode(&okEntry))
	assert.Equal(t, "request completed", okEntry["msg"])
	assert.Equal(t, http.MethodGet, okEntry["method"])
	assert.Equal(t, "/session", okEntry["uri"])
	assert.Equal(t, json.Number("200"), okEntry["status"])
	assert.Contains(t, okEntry, "latency_ms")
	assert.NotContains(t, okEntry, "error")

	// /fail logs request failed with error key and preserves HTTP error status/body
	logBuf.Reset()
	recFail := httptest.NewRecorder()
	e.ServeHTTP(recFail, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/fail", nil))
	require.Equal(t, http.StatusUnauthorized, recFail.Code)
	assert.JSONEq(t, `{"message":"session cookie not found"}`, recFail.Body.String())

	failLines := strings.Split(strings.TrimSpace(logBuf.String()), "\n")
	require.Len(t, failLines, 1)
	var failEntry map[string]any
	failDec := json.NewDecoder(strings.NewReader(failLines[0]))
	failDec.UseNumber()
	require.NoError(t, failDec.Decode(&failEntry))
	assert.Equal(t, "request failed", failEntry["msg"])
	assert.Equal(t, http.MethodGet, failEntry["method"])
	assert.Equal(t, "/fail", failEntry["uri"])
	assert.Equal(t, json.Number("401"), failEntry["status"])
	assert.Contains(t, failEntry, "latency_ms")
	assert.Contains(t, failEntry["error"], "session cookie not found")
}

func TestOTelMiddlewareChain_RecordsErrorSpanAndSkipsHealth(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	e := echo.New()
	e.Logger = slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	e.Use(echootel.NewMiddlewareWithConfig(echootel.Config{
		ServerName:     "auth-hub",
		Skipper:        appmiddleware.OTelSkipper,
		TracerProvider: tp,
		Propagators:    propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}),
	}))
	e.Use(appmiddleware.OTelStatusMiddleware())
	e.Use(newRequestLoggerMiddleware())

	e.GET("/health", func(c *echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	e.GET("/validate", func(_ *echo.Context) error {
		return echo.NewHTTPError(http.StatusBadGateway, "identity provider unavailable")
	})

	recHealth := httptest.NewRecorder()
	e.ServeHTTP(recHealth, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil))
	require.Equal(t, http.StatusOK, recHealth.Code)
	assert.Empty(t, spanRecorder.Ended())

	recErr := httptest.NewRecorder()
	e.ServeHTTP(recErr, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/validate", nil))
	require.Equal(t, http.StatusBadGateway, recErr.Code)
	assert.JSONEq(t, `{"message":"identity provider unavailable"}`, recErr.Body.String())

	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status().Code)

	var gotStatus int64
	for _, attr := range spans[0].Attributes() {
		if string(attr.Key) == "http.response.status_code" {
			gotStatus = attr.Value.AsInt64()
		}
	}
	assert.Equal(t, int64(http.StatusBadGateway), gotStatus)
}
