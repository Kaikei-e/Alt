package rest

import (
	"testing"

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
// cross-tenant article dump on the browser-facing REST port; it now lives in
// cmd/datahub behind mutual TLS, on an Echo instance of its own.
func TestRegisterInternalRoutes_MountsOnItsOwnEcho(t *testing.T) {
	e := echo.New()
	RegisterInternalRoutes(e, &di.DataHubComponents{})

	paths := routePaths(e)
	for _, p := range []string{systemUserPath, recentArticlesPath} {
		if !paths[p] {
			t.Errorf("%s must be registered on the data-hub Echo instance", p)
		}
	}
}
