package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"auth-hub/internal/domain"
	"auth-hub/internal/infrastructure/cache"
	"auth-hub/internal/usecase"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionHandler_HandleInvalidate(t *testing.T) {
	e := echo.New()

	t.Run("successful authenticated invalidation with cookie", func(t *testing.T) {
		sessionCache := cache.NewSessionCache(60 * time.Second)
		sessionCache.Set("cookie-val-123", domain.CachedSession{
			UserID: "user-abc",
		})

		// Confirm entry exists
		_, found := sessionCache.Get("cookie-val-123")
		require.True(t, found)

		invalidateUC := usecase.NewInvalidateSession(sessionCache, slog.Default())
		h := NewSessionHandler(nil, invalidateUC)

		req := httptest.NewRequest(http.MethodPost, "/session/invalidate", nil)
		req.AddCookie(&http.Cookie{
			Name:  "ory_kratos_session",
			Value: "cookie-val-123",
		})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := h.HandleInvalidate(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]bool
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["ok"])

		// Confirm entry was removed from cache
		_, found = sessionCache.Get("cookie-val-123")
		assert.False(t, found, "cache entry must be deleted after invalidation")
	})

	t.Run("unauthenticated request missing cookie returns 401", func(t *testing.T) {
		sessionCache := cache.NewSessionCache(60 * time.Second)
		invalidateUC := usecase.NewInvalidateSession(sessionCache, slog.Default())
		h := NewSessionHandler(nil, invalidateUC)

		req := httptest.NewRequest(http.MethodPost, "/session/invalidate", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := h.HandleInvalidate(c)
		require.Error(t, err)

		he, ok := err.(*echo.HTTPError)
		require.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, he.Code)
		assert.Contains(t, he.Message, "session cookie not found")
	})
}

func TestInternalHandler_HandleInvalidateSession(t *testing.T) {
	e := echo.New()

	t.Run("successful internal invalidation with JSON body", func(t *testing.T) {
		sessionCache := cache.NewSessionCache(60 * time.Second)
		sessionCache.Set("internal-cookie-target", domain.CachedSession{
			UserID: "user-xyz",
		})

		_, found := sessionCache.Get("internal-cookie-target")
		require.True(t, found)

		invalidateUC := usecase.NewInvalidateSession(sessionCache, slog.Default())
		h := NewInternalHandler(nil, invalidateUC, nil)

		body := `{"cookie":"internal-cookie-target"}`
		req := httptest.NewRequest(http.MethodPost, "/internal/session/invalidate", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := h.HandleInvalidateSession(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		_, found = sessionCache.Get("internal-cookie-target")
		assert.False(t, found, "target session must be deleted from cache")
	})

	t.Run("empty body returns 400 Bad Request", func(t *testing.T) {
		sessionCache := cache.NewSessionCache(60 * time.Second)
		invalidateUC := usecase.NewInvalidateSession(sessionCache, slog.Default())
		h := NewInternalHandler(nil, invalidateUC, nil)

		body := `{}`
		req := httptest.NewRequest(http.MethodPost, "/internal/session/invalidate", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := h.HandleInvalidateSession(c)
		require.Error(t, err)

		he, ok := err.(*echo.HTTPError)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, he.Code)
	})
}
