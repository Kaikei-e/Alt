package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"alt/config"
	"alt/di"

	"github.com/labstack/echo/v4"
)

const (
	systemUserPath     = "/v1/internal/system-user"
	recentArticlesPath = "/v1/internal/articles/recent"
)

func routePaths(e *echo.Echo) map[string]bool {
	paths := make(map[string]bool)
	for _, r := range e.Routes() {
		paths[r.Path] = true
	}
	return paths
}

// The /v1/internal group inherits no auth middleware. Registering it on the
// public Echo instance published the system-user identity and an unbounded
// cross-tenant article dump on the browser-facing REST port.
func TestRegisterInternalRoutes_MountsOnItsOwnEcho(t *testing.T) {
	e := echo.New()
	RegisterInternalRoutes(e, &di.ApplicationComponents{})

	paths := routePaths(e)
	for _, p := range []string{systemUserPath, recentArticlesPath} {
		if !paths[p] {
			t.Errorf("%s must be registered on the internal Echo instance", p)
		}
	}
}

// The pin the other direction. ADR-000717 closed these two endpoints with a
// RequireServiceAuth wrap, ADR-000743 removed the wrap, and nothing failed —
// the router built by RegisterRoutes is what the published REST port serves,
// so this asserts against that router rather than against a hand-built one.
func TestRegisterRoutes_DoesNotExposeInternalRoutes(t *testing.T) {
	e := echo.New()
	RegisterRoutes(context.Background(), e, &di.ApplicationComponents{}, &config.Config{})

	paths := routePaths(e)
	for _, p := range []string{systemUserPath, recentArticlesPath} {
		if paths[p] {
			t.Errorf("%s must not be registered on the browser-facing REST router", p)
		}
	}

	for _, p := range []string{systemUserPath, recentArticlesPath + "?limit=0"} {
		res := httptest.NewRecorder()
		e.ServeHTTP(res, httptest.NewRequest(http.MethodGet, p, nil))
		if res.Code != http.StatusNotFound {
			t.Errorf("GET %s on the browser-facing REST router = %d, want 404", p, res.Code)
		}
	}
}
