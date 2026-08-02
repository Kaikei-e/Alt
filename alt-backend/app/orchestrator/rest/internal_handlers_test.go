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

// ADR-000717 closed these two endpoints with a RequireServiceAuth wrap,
// ADR-000743 removed the wrap, and nothing failed — so this asserts against
// the router RegisterRoutes actually builds rather than a hand-made one.
//
// The handlers themselves are gone — ADR-000954 D6 folded them into
// services.datahub.v1.DataHubService and Wave 2-C deleted the REST pair. The
// assertion stays because "the published REST port serves no /v1/internal
// route" is the property that matters, and it has to keep holding whether the
// route is unregistered, moved to another binary, or retired outright.
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
