package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"auth-hub/config"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
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
	e.GET("/internal/system-user", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/internal/system-user", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestWireInternalAuth_AllowsValidSecret(t *testing.T) {
	cfg := separatedSecretsConfig()
	mw := wireInternalAuth(cfg)

	e := echo.New()
	e.Use(mw)
	e.GET("/internal/system-user", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/internal/system-user", nil)
	req.Header.Set("X-Internal-Auth", cfg.InternalAuthSecret)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestBuildPlaintextHandler_DefaultRejectsBusinessRoutes(t *testing.T) {
	e := echo.New()
	e.GET("/health", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	e.GET("/session", func(c echo.Context) error {
		return c.String(http.StatusOK, "session-data")
	})
	e.GET("/validate", func(c echo.Context) error {
		return c.String(http.StatusOK, "validate-data")
	})
	e.POST("/csrf", func(c echo.Context) error {
		return c.String(http.StatusOK, "csrf-token")
	})
	e.GET("/internal/system-user", func(c echo.Context) error {
		return c.String(http.StatusOK, "system-user-id")
	})

	// Default: devPlaintextAllowed is false
	handler := buildPlaintextHandler(e, false)

	// Health check MUST succeed on plaintext (bootstrap availability)
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
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
		req := httptest.NewRequest(ep.method, ep.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code, "endpoint %s must be forbidden on plaintext listener", ep.path)
		assert.Contains(t, rec.Body.String(), "business endpoints require HTTPS")
	}
}

func TestBuildPlaintextHandler_ExplicitDevPermitsRoutes(t *testing.T) {
	e := echo.New()
	e.GET("/session", func(c echo.Context) error {
		return c.String(http.StatusOK, "dev-session-data")
	})

	// Explicit dev opt-in allows business routes
	handler := buildPlaintextHandler(e, true)

	req := httptest.NewRequest(http.MethodGet, "/session", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "dev-session-data", rec.Body.String())
}
