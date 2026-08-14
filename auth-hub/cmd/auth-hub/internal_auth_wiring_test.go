package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"auth-hub/config"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

const (
	jwtSigningKey     = "this-is-a-valid-backend-token-secret-32-chars-long"
	internalAuthValue = "this-is-a-distinct-internal-auth-secret-32-chars"
)

func separatedSecretsConfig() *config.Config {
	return &config.Config{
		KratosURL:          "http://kratos:4433",
		Port:               "8888",
		CacheTTL:           5 * time.Minute,
		CSRFSecret:         "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
		BackendTokenSecret: jwtSigningKey,
		InternalAuthSecret: internalAuthValue,
	}
}

// serveInternal exercises the middleware main wires onto the /internal group.
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

// The JWT signing key must not open /internal. alt-backend puts whatever this
// group accepts into a plaintext X-Internal-Auth header on every
// GetSystemUser call, so accepting the signing key here is what pushed it out
// of the process and into access logs and OTel span attributes.
func TestWireInternalAuth_RejectsTheJWTSigningKey(t *testing.T) {
	mw := wireInternalAuth(separatedSecretsConfig())

	assert.Equal(t, http.StatusForbidden, serveInternal(t, mw, jwtSigningKey))
}

func TestWireInternalAuth_AcceptsTheInternalAuthSecret(t *testing.T) {
	mw := wireInternalAuth(separatedSecretsConfig())

	assert.Equal(t, http.StatusOK, serveInternal(t, mw, internalAuthValue))
}

// Defence in depth for the choke point: config.Validate already rejects an
// identical pair, so reaching here with one means that invariant broke.
func TestWireInternalAuth_PanicsWhenSecretsAreIdentical(t *testing.T) {
	cfg := separatedSecretsConfig()
	cfg.InternalAuthSecret = cfg.BackendTokenSecret

	assert.Panics(t, func() {
		wireInternalAuth(cfg)
	})
}
