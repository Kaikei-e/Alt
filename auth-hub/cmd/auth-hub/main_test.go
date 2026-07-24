package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

// wireInternalAuth is the single fail-closed choke point for /internal auth
// wiring. It must never return a no-op middleware: an empty secret is a
// startup bug (config.Validate() should have already rejected it), not a
// signal to skip auth (CLAUDE.md Rule 8).
func TestWireInternalAuth_PanicsOnEmptySecret(t *testing.T) {
	assert.Panics(t, func() {
		wireInternalAuth("")
	})
}

func TestWireInternalAuth_FailsClosedWithoutHeader(t *testing.T) {
	mw := wireInternalAuth("this-is-a-valid-backend-token-secret-32-chars-long")

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
	secret := "this-is-a-valid-backend-token-secret-32-chars-long"
	mw := wireInternalAuth(secret)

	e := echo.New()
	e.Use(mw)
	e.GET("/internal/system-user", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/internal/system-user", nil)
	req.Header.Set("X-Internal-Auth", secret)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}
