package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

// newServiceAuthForTest constructs a ServiceAuthMiddleware with a fixed secret
// without touching environment variables, so tests can run in any order.
func newServiceAuthForTest(secret string) *ServiceAuthMiddleware {
	return &ServiceAuthMiddleware{
		logger:        nil,
		serviceSecret: secret,
	}
}

// C-001: missing X-Service-Token must return 401.
func TestRequireServiceAuth_MissingToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/internal/system-user", nil)
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	mw := newServiceAuthForTest("secret-value")
	called := false
	h := mw.RequireServiceAuth()(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})

	err := h(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	require.Equal(t, http.StatusUnauthorized, httpErr.Code)
	require.False(t, called)
}

// C-001: wrong X-Service-Token must return 401 without leaking info.
func TestRequireServiceAuth_WrongToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/internal/system-user", nil)
	req.Header.Set("X-Service-Token", "wrong-value")
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	mw := newServiceAuthForTest("secret-value")
	called := false
	h := mw.RequireServiceAuth()(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})

	err := h(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	require.Equal(t, http.StatusUnauthorized, httpErr.Code)
	require.False(t, called)
}

// C-001: correct X-Service-Token must pass through.
func TestRequireServiceAuth_CorrectToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/internal/system-user", nil)
	req.Header.Set("X-Service-Token", "secret-value")
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	mw := newServiceAuthForTest("secret-value")
	called := false
	h := mw.RequireServiceAuth()(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})
	require.NoError(t, h(c))
	require.True(t, called)
}

// H-002: tokens of differing length must still be compared in constant time,
// i.e. the middleware must not short-circuit on length before comparing content.
// We assert that a differing-length token is rejected (functional correctness).
// True timing-safety is verified by reading the source (crypto/subtle).
func TestRequireServiceAuth_DifferingLengthToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/internal/system-user", nil)
	req.Header.Set("X-Service-Token", "secret-value-extended")
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	mw := newServiceAuthForTest("secret-value")
	h := mw.RequireServiceAuth()(func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	err := h(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	require.Equal(t, http.StatusUnauthorized, httpErr.Code)
}

// C-001: when SERVICE_SECRET is not configured, the middleware must deny-all
// (500) rather than accepting any token.
func TestRequireServiceAuth_SecretNotConfigured(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/internal/system-user", nil)
	req.Header.Set("X-Service-Token", "anything")
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	mw := newServiceAuthForTest("") // empty secret
	called := false
	h := mw.RequireServiceAuth()(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})

	err := h(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	require.Equal(t, http.StatusInternalServerError, httpErr.Code)
	require.False(t, called)
}
