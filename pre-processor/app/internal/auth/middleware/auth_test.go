package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"pre-processor/internal/auth"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAuthServer(t *testing.T, expectedUser auth.UserContext) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/internal/validate" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer valid-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(expectedUser)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAuthMiddleware_RequireAuth(t *testing.T) {
	expectedUser := auth.UserContext{
		UserID:    uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		TenantID:  uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		Email:     "reader@example.com",
		Role:      "user",
		SessionID: "sess-123",
	}
	authSrv := newTestAuthServer(t, expectedUser)
	client := auth.NewClient(auth.Config{
		AuthServiceURL: authSrv.URL,
		ServiceName:    "pre-processor",
	})
	mw := NewAuthMiddleware(client)

	t.Run("rejects missing Authorization header with 401", func(t *testing.T) {
		e := echo.New()
		nextCalled := false
		e.GET("/protected", func(c *echo.Context) error {
			nextCalled = true
			return c.NoContent(http.StatusOK)
		}, mw.RequireAuth())

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/protected", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.False(t, nextCalled)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("rejects invalid token with 401", func(t *testing.T) {
		e := echo.New()
		nextCalled := false
		e.GET("/protected", func(c *echo.Context) error {
			nextCalled = true
			return c.NoContent(http.StatusOK)
		}, mw.RequireAuth())

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer bad-token")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.False(t, nextCalled)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("accepts valid token and attaches user context", func(t *testing.T) {
		e := echo.New()
		var gotUser *auth.UserContext
		e.GET("/protected", func(c *echo.Context) error {
			gotUser, _ = c.Request().Context().Value(UserContextKey).(*auth.UserContext)
			return c.NoContent(http.StatusOK)
		}, mw.RequireAuth())

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.NotNil(t, gotUser)
		assert.Equal(t, expectedUser.UserID, gotUser.UserID)
		assert.Equal(t, expectedUser.Email, gotUser.Email)
	})
}

func TestAuthMiddleware_OptionalAuth(t *testing.T) {
	expectedUser := auth.UserContext{
		UserID:    uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		TenantID:  uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		Email:     "reader@example.com",
		Role:      "user",
		SessionID: "sess-123",
	}
	authSrv := newTestAuthServer(t, expectedUser)
	client := auth.NewClient(auth.Config{
		AuthServiceURL: authSrv.URL,
		ServiceName:    "pre-processor",
	})
	mw := NewAuthMiddleware(client)

	t.Run("continues without user when Authorization header is absent", func(t *testing.T) {
		e := echo.New()
		nextCalled := false
		var gotUser any
		e.GET("/optional", func(c *echo.Context) error {
			nextCalled = true
			gotUser = c.Request().Context().Value(UserContextKey)
			return c.NoContent(http.StatusOK)
		}, mw.OptionalAuth())

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/optional", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.True(t, nextCalled)
		assert.Nil(t, gotUser)
	})

	t.Run("attaches user when valid token is provided", func(t *testing.T) {
		e := echo.New()
		var gotUser *auth.UserContext
		e.GET("/optional", func(c *echo.Context) error {
			gotUser, _ = c.Request().Context().Value(UserContextKey).(*auth.UserContext)
			return c.NoContent(http.StatusOK)
		}, mw.OptionalAuth())

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/optional", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.NotNil(t, gotUser)
		assert.Equal(t, expectedUser.UserID, gotUser.UserID)
	})
}
