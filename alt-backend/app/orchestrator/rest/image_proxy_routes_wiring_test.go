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

// Finding [6]: registerImageProxyRoutes must not register the /images/proxy
// route when ImageProxy.Enabled=true but Secret is empty (misconfiguration —
// e.g. a Docker secret file failed to load). With container.ImageProxyUsecase
// left nil by newImageModule in that case (see di/image_module.go), a
// request that reaches the handler panics on a nil pointer method call
// instead of a clean 404. The route registration guard must match the
// usecase-construction guard exactly.
func TestRegisterImageProxyRoutes_SkipsWhenEnabledButNoSecret(t *testing.T) {
	e := echo.New()
	e.Use(echomiddleware.Recover())
	v1 := e.Group("/v1")

	cfg := &config.Config{}
	cfg.ImageProxy.Enabled = true
	cfg.ImageProxy.Secret = "" // misconfigured: enabled but no secret

	registerImageProxyRoutes(v1, &di.ApplicationComponents{}, cfg)

	req := httptest.NewRequest(http.MethodGet, "/v1/images/proxy/sig/dGVzdA", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code,
		"route must not be registered when secret is empty, got %d: %s", rec.Code, rec.Body.String())
}
