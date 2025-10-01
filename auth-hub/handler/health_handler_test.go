package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHealthHandler(t *testing.T) {
	t.Run("creates handler successfully", func(t *testing.T) {
		handler := NewHealthHandler()

		assert.NotNil(t, handler)
	})
}

func TestHealthHandler_Handle(t *testing.T) {
	t.Run("returns 200 with healthy status", func(t *testing.T) {
		handler := NewHealthHandler()

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler.Handle(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

		// Parse response body
		var response map[string]string
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "healthy", response["status"])
		assert.Equal(t, "auth-hub", response["service"])
	})
}
