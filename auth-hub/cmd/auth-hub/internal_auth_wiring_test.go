package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"auth-hub/config"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

const (
	wrongInternalAuthValue = "this-is-an-invalid-internal-auth-secret-32-chars"
	internalAuthValue      = "this-is-a-distinct-internal-auth-secret-32-chars"
)

func separatedSecretsConfig() *config.Config {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return &config.Config{
		KratosURL:              "http://kratos:4433",
		Port:                   "8888",
		CacheTTL:               5 * time.Minute,
		CSRFSecret:             "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
		BackendTokenSigningKey: priv,
		InternalAuthSecret:     internalAuthValue,
	}
}

func serveInternal(t *testing.T, mw echo.MiddlewareFunc, header string) int {
	t.Helper()

	e := echo.New()
	e.Use(mw)
	e.GET("/internal/system-user", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/internal/system-user", nil)
	if header != "" {
		req.Header.Set("X-Internal-Auth", header)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec.Code
}

func TestWireInternalAuth_RejectsWrongSecret(t *testing.T) {
	mw := wireInternalAuth(separatedSecretsConfig())

	assert.Equal(t, http.StatusForbidden, serveInternal(t, mw, wrongInternalAuthValue))
}

func TestWireInternalAuth_AcceptsTheInternalAuthSecret(t *testing.T) {
	mw := wireInternalAuth(separatedSecretsConfig())

	assert.Equal(t, http.StatusOK, serveInternal(t, mw, internalAuthValue))
}
