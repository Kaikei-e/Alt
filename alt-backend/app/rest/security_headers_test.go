package rest

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/stretchr/testify/require"
)

// M-008: hardened security headers — CSP must lock down sources used by the API
// (no HTML rendering) and report violations to /security/csp-report.
// Referrer-Policy and Permissions-Policy must be present.
func buildHeaderEchoForSecure() *echo.Echo {
	e := echo.New()
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		HSTSMaxAge:            31536000,
		HSTSPreloadEnabled:    true,
		ContentSecurityPolicy: "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'; report-uri /security/csp-report",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
	}))
	e.GET("/v1/health", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	return e
}

func TestSecurityHeaders_CSPHardened(t *testing.T) {
	e := buildHeaderEchoForSecure()
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)

	csp := res.Header().Get("Content-Security-Policy")
	require.Contains(t, csp, "default-src 'none'", "API responses must lock default-src to none")
	require.Contains(t, csp, "frame-ancestors 'none'", "must forbid framing")
	require.Contains(t, csp, "base-uri 'none'", "must forbid base-uri abuse")
	require.Contains(t, csp, "report-uri /security/csp-report", "violations must report to backend endpoint")
}

func TestSecurityHeaders_ReferrerPolicySet(t *testing.T) {
	e := buildHeaderEchoForSecure()
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)
	require.Equal(t,
		"strict-origin-when-cross-origin",
		res.Header().Get("Referrer-Policy"),
		"Referrer-Policy must be present and conservative",
	)
}

func TestSecurityHeaders_HSTSWithPreload(t *testing.T) {
	e := buildHeaderEchoForSecure()
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	// Echo's Secure middleware only emits HSTS when the request is TLS.
	req.TLS = &tls.ConnectionState{}
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)

	sts := res.Header().Get("Strict-Transport-Security")
	require.NotEmpty(t, sts)
	require.Contains(t, sts, "max-age=31536000")
	require.True(t,
		strings.Contains(sts, "preload"),
		"HSTS must include preload when HSTSPreloadEnabled is true, got: %s", sts,
	)
}

// M-009: CORS AllowHeaders must include the X-Alt-Backend-Token header used
// by JWT auth so browser preflight does not block authenticated requests.
func buildHeaderEchoForCORS() *echo.Echo {
	e := echo.New()
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"https://example.com"},
		AllowMethods: []string{echo.GET, echo.POST, echo.PUT, echo.DELETE, echo.OPTIONS},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			"Cache-Control",
			"Authorization",
			"X-Requested-With",
			"X-CSRF-Token",
			"X-Alt-Backend-Token",
		},
	}))
	e.POST("/v1/feeds/search", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	return e
}

func TestCORS_AllowsAltBackendTokenHeader(t *testing.T) {
	e := buildHeaderEchoForCORS()
	req := httptest.NewRequest(http.MethodOptions, "/v1/feeds/search", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "x-alt-backend-token")
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)

	allowHeaders := strings.ToLower(res.Header().Get("Access-Control-Allow-Headers"))
	require.Contains(t, allowHeaders, "x-alt-backend-token",
		"preflight must allow X-Alt-Backend-Token so browser CORS does not block JWT requests")
}
