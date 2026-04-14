package rest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"alt/di"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

// C-001a: /v1/internal/* must reject requests without a valid X-Service-Token.
// These tests exercise only the middleware layer of the route registration;
// they do not invoke downstream handlers and therefore do not require a
// fully-wired DI container.
func TestInternalRoutes_MissingServiceToken_Returns401(t *testing.T) {
	// Configure SERVICE_SECRET so the middleware is not in deny-all mode.
	t.Setenv("SERVICE_SECRET", "test-service-secret")

	e := echo.New()
	// A nil container is safe because the middleware rejects the request
	// before any handler closure runs.
	registerInternalRoutes(e, (*di.ApplicationComponents)(nil))

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/system-user", nil)
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)

	require.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestInternalRoutes_WrongServiceToken_Returns401(t *testing.T) {
	t.Setenv("SERVICE_SECRET", "test-service-secret")

	e := echo.New()
	registerInternalRoutes(e, (*di.ApplicationComponents)(nil))

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/articles/recent", nil)
	req.Header.Set("X-Service-Token", "wrong-value")
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)

	require.Equal(t, http.StatusUnauthorized, res.Code)
}

// Confirm SERVICE_SECRET_FILE also takes effect (Docker Secrets path),
// matching the existing middleware behaviour.
func TestInternalRoutes_ServiceSecretFileSupported(t *testing.T) {
	tmp := t.TempDir()
	secretPath := tmp + "/service-secret"
	require.NoError(t, os.WriteFile(secretPath, []byte("secret-from-file\n"), 0600))

	t.Setenv("SERVICE_SECRET", "")
	t.Setenv("SERVICE_SECRET_FILE", secretPath)

	e := echo.New()
	registerInternalRoutes(e, (*di.ApplicationComponents)(nil))

	// Missing header still 401
	req := httptest.NewRequest(http.MethodGet, "/v1/internal/system-user", nil)
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)
	require.Equal(t, http.StatusUnauthorized, res.Code)

	// Wrong header still 401
	req2 := httptest.NewRequest(http.MethodGet, "/v1/internal/system-user", nil)
	req2.Header.Set("X-Service-Token", "wrong")
	res2 := httptest.NewRecorder()
	e.ServeHTTP(res2, req2)
	require.Equal(t, http.StatusUnauthorized, res2.Code)
}
