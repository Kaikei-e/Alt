package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlaintextHealthOnly_AllowsHealth(t *testing.T) {
	e := echo.New()
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "healthy"})
	})
	handler := PlaintextHealthOnly(e)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestPlaintextHealthOnly_RejectsSession(t *testing.T) {
	e := echo.New()
	e.GET("/session", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"ok": "true"})
	})
	handler := PlaintextHealthOnly(e)

	req := httptest.NewRequest(http.MethodGet, "/session", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)

	var resp map[string]string
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp["error"], "HTTPS")
}

func TestPlaintextHealthOnly_RejectsValidate(t *testing.T) {
	e := echo.New()
	handler := PlaintextHealthOnly(e)

	req := httptest.NewRequest(http.MethodGet, "/validate", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestPlaintextHealthOnly_RejectsCsrf(t *testing.T) {
	e := echo.New()
	handler := PlaintextHealthOnly(e)

	req := httptest.NewRequest(http.MethodPost, "/csrf", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestPlaintextHealthOnly_RejectsInternalRoutes(t *testing.T) {
	e := echo.New()
	handler := PlaintextHealthOnly(e)

	req := httptest.NewRequest(http.MethodGet, "/internal/system-user", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestPlaintextHealthOnly_RejectsSessionInvalidate(t *testing.T) {
	e := echo.New()
	handler := PlaintextHealthOnly(e)

	req := httptest.NewRequest(http.MethodPost, "/session/invalidate", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
