package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func newServiceAuthForTest(secret string) *ServiceAuthMiddleware {
	return &ServiceAuthMiddleware{
		logger:        nil,
		serviceSecret: secret,
	}
}

func TestRequireServiceAuth_MissingToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", nil)
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

func TestRequireServiceAuth_WrongToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", nil)
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

func TestRequireServiceAuth_CorrectToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", nil)
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

func TestRequireServiceAuth_DifferingLengthToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", nil)
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

func TestRequireServiceAuth_SecretNotConfigured(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", nil)
	req.Header.Set("X-Service-Token", "anything")
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	mw := newServiceAuthForTest("")
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

func TestRequireServiceAuth_EmptyTokenHeader(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", nil)
	req.Header.Set("X-Service-Token", "   ")
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
