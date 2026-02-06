package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/labstack/echo/v4"
)

const internalAuthHeader = "X-Internal-Auth"

// InternalAuth creates middleware that validates a shared secret for internal endpoints.
// Uses constant-time comparison to prevent timing attacks.
func InternalAuth(sharedSecret string) echo.MiddlewareFunc {
	secretBytes := []byte(sharedSecret)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			provided := []byte(c.Request().Header.Get(internalAuthHeader))
			if len(provided) == 0 {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing internal auth header")
			}
			if subtle.ConstantTimeCompare(provided, secretBytes) != 1 {
				return echo.NewHTTPError(http.StatusForbidden, "invalid internal auth")
			}
			return next(c)
		}
	}
}
