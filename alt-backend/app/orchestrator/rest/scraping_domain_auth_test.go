package rest

import (
	"alt/config"
	"alt/di"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/stretchr/testify/require"
)

// Finding [1]: /v1/admin/scraping-domains lets an unauthenticated caller
// rewrite scraping consent policy (ForceRespectRobots, AllowMLTraining) and
// trigger robots.txt refetches against arbitrary domains. Every other admin
// route group (registerDashboardRoutes) applies RequireAuth()+RequireAdmin();
// registerScrapingDomainRoutes must do the same.
//
// Recover() is installed so that an unauthenticated request which slips past
// a missing auth guard and hits the handler with a nil usecase produces a
// distinguishable 500 (panic recovered) rather than crashing the test binary,
// keeping the assertion meaningful either way.
func TestRegisterScrapingDomainRoutes_RequiresAuth(t *testing.T) {
	e := echo.New()
	e.Use(echomiddleware.Recover())
	v1 := e.Group("/v1")
	registerScrapingDomainRoutes(v1, &di.ApplicationComponents{}, &config.Config{})

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/scraping-domains", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code,
		"unauthenticated request to admin scraping-domains route must be rejected with 401, got %d: %s", rec.Code, rec.Body.String())
}
