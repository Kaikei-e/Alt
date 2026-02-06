package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestSecurityHeaders_SetsAllHeaders(t *testing.T) {
	e := echo.New()
	e.Use(SecurityHeaders())
	e.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "max-age=63072000; includeSubDomains; preload", rec.Header().Get("Strict-Transport-Security"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "default-src 'none'; frame-ancestors 'none'", rec.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "strict-origin-when-cross-origin", rec.Header().Get("Referrer-Policy"))
	assert.Equal(t, "camera=(), microphone=(), geolocation=()", rec.Header().Get("Permissions-Policy"))
	assert.Equal(t, "no-store, no-cache, must-revalidate, private", rec.Header().Get("Cache-Control"))
}

func TestSecurityHeaders_DoesNotBlockRequest(t *testing.T) {
	e := echo.New()
	e.Use(SecurityHeaders())
	e.POST("/test", func(c echo.Context) error {
		return c.String(http.StatusCreated, "created")
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "created", rec.Body.String())
}
