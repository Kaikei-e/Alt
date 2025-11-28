package middleware_test

import (
	"alt/config"
	"alt/middleware"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestAuthMiddleware_Bypass(t *testing.T) {
	// Setup
	e := echo.New()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{
		Auth: config.AuthConfig{
			BackendTokenSecret:   "",
			BackendTokenIssuer:   "auth-hub",
			BackendTokenAudience: "alt-backend",
		},
	}
	authMiddleware := middleware.NewAuthMiddleware(logger, "test-secret", cfg)

	// Define a protected route
	e.GET("/protected", func(c echo.Context) error {
		return c.String(http.StatusOK, "Secret Data")
	}, authMiddleware.RequireAuth())

	// Case 1: No headers - should fail
	t.Run("Missing Headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	// Case 2: Arbitrary headers WITHOUT secret - should fail (FIXED)
	t.Run("Arbitrary Headers Without Secret", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		// Set arbitrary valid UUIDs
		req.Header.Set("X-Alt-User-Id", "123e4567-e89b-12d3-a456-426614174000")
		req.Header.Set("X-Alt-Tenant-Id", "123e4567-e89b-12d3-a456-426614174000")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		// This should now be 401 Unauthorized
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "Fix confirmed: Arbitrary headers without secret rejected")
	})

	// Case 3: Arbitrary headers WITH secret - should succeed
	t.Run("Arbitrary Headers With Secret", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		// Set arbitrary valid UUIDs
		req.Header.Set("X-Alt-User-Id", "123e4567-e89b-12d3-a456-426614174000")
		req.Header.Set("X-Alt-Tenant-Id", "123e4567-e89b-12d3-a456-426614174000")
		req.Header.Set("X-Alt-Shared-Secret", "test-secret")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code, "Valid secret allowed access")
	})
}
