package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestInternalAuth_ValidSecret(t *testing.T) {
	secret := "my-shared-secret-for-internal-endpoints"
	e := echo.New()
	e.Use(InternalAuth(secret))
	e.GET("/internal/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/internal/test", nil)
	req.Header.Set("X-Internal-Auth", secret)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestInternalAuth_MissingHeader(t *testing.T) {
	secret := "my-shared-secret-for-internal-endpoints"
	e := echo.New()
	e.Use(InternalAuth(secret))
	e.GET("/internal/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/internal/test", nil)
	// No auth header
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestInternalAuth_InvalidSecret(t *testing.T) {
	secret := "my-shared-secret-for-internal-endpoints"
	e := echo.New()
	e.Use(InternalAuth(secret))
	e.GET("/internal/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/internal/test", nil)
	req.Header.Set("X-Internal-Auth", "wrong-secret")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
