package main

import (
	"alt/config"
	"alt/di"
	"alt/middleware"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

type stubCSRFUsecase struct{}

func (stubCSRFUsecase) GenerateToken(context.Context) (string, error)       { return "tok", nil }
func (stubCSRFUsecase) ValidateToken(context.Context, string) (bool, error) { return true, nil }

func TestNewRESTEcho_HTTPErrorHandlerResponses(t *testing.T) {
	e := newRESTEcho(&di.ApplicationComponents{}, &config.Config{}, false)
	e.POST("/v1/feeds/read", func(c *echo.Context) error {
		return c.NoContent(http.StatusOK)
	}, middleware.CSRFMiddleware(stubCSRFUsecase{}))
	e.POST("/v1/feeds/search", func(c *echo.Context) error {
		return c.NoContent(http.StatusOK)
	}, middleware.ValidationMiddleware())
	e.GET("/v1/boom", func(c *echo.Context) error {
		return errors.New("db down")
	})

	t.Run("CSRF missing token preserves v4 JSON body", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/feeds/read", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusForbidden, rec.Code)
		require.JSONEq(t, `{"error":"Forbidden","detail":{"error":"csrf_token_missing","message":"CSRF token is required"}}`, rec.Body.String())
		t.Logf("CSRF response body: %s", strings.TrimSpace(rec.Body.String()))
	})

	t.Run("Validation missing query preserves v4 JSON body", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/feeds/search", strings.NewReader(`{"other":"x"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.JSONEq(t, `{"error":"Bad Request","detail":{"error":"validation_failed","message":"Validation failed","details":[{"field":"query","message":"query field is required"}]}}`, rec.Body.String())
		t.Logf("Validation response body: %s", strings.TrimSpace(rec.Body.String()))
	})

	t.Run("echo.ErrNotFound returns 404 not 500", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/does-not-exist", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code)
		require.JSONEq(t, `{"error":"Not Found","detail":"Not Found"}`, rec.Body.String())
	})

	t.Run("echo.ErrMethodNotAllowed returns 405 not 500", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/feeds/read", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		require.JSONEq(t, `{"error":"Method Not Allowed","detail":"Method Not Allowed"}`, rec.Body.String())
	})

	t.Run("unclassified error returns 500 internal_error", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/boom", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusInternalServerError, rec.Code)
		require.JSONEq(t, `{"error":"internal_error"}`, rec.Body.String())
	})
}
