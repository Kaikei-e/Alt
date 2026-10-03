package middleware

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// RequireMTLS enforces that a request arrived over a TLS connection with
// at least one verified client certificate. It guards the internal group
// against being exposed on the frontend server-only TLS listener or plaintext.
func RequireMTLS() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			tlsState := c.Request().TLS
			if tlsState == nil || len(tlsState.VerifiedChains) == 0 || len(tlsState.VerifiedChains[0]) == 0 {
				return echo.NewHTTPError(http.StatusForbidden, "mTLS required")
			}
			return next(c)
		}
	}
}

// RequireMTLSPeer enforces that a request arrived over a TLS connection and the client certificate's
// Common Name is in the allowed list.
func RequireMTLSPeer(allowedCNs []string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			tlsState := c.Request().TLS
			if tlsState == nil || len(tlsState.VerifiedChains) == 0 || len(tlsState.VerifiedChains[0]) == 0 {
				return echo.NewHTTPError(http.StatusForbidden, "mTLS required")
			}

			leaf := tlsState.VerifiedChains[0][0]
			cn := leaf.Subject.CommonName
			allowed := false
			for _, allowedCN := range allowedCNs {
				if cn == allowedCN {
					allowed = true
					break
				}
			}
			if !allowed {
				return echo.NewHTTPError(http.StatusForbidden, "unauthorized peer")
			}
			return next(c)
		}
	}
}
