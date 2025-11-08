package middleware

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
)

const (
	serviceTokenHeader = "X-Service-Token"
)

// ServiceAuthMiddleware validates service-to-service authentication tokens.
// This middleware is intended for internal microservice communication only.
type ServiceAuthMiddleware struct {
	logger        *slog.Logger
	serviceSecret string
}

// NewServiceAuthMiddleware constructs a ServiceAuthMiddleware instance.
// The service secret is loaded from the SERVICE_SECRET environment variable.
func NewServiceAuthMiddleware(logger *slog.Logger) *ServiceAuthMiddleware {
	secret := os.Getenv("SERVICE_SECRET")
	if secret == "" {
		if logger != nil {
			logger.Warn("SERVICE_SECRET not set, service auth will deny all requests")
		}
	}

	return &ServiceAuthMiddleware{
		logger:        logger,
		serviceSecret: secret,
	}
}

// RequireServiceAuth ensures that the X-Service-Token header is present and valid.
// This middleware should be used for internal service-to-service endpoints only.
func (m *ServiceAuthMiddleware) RequireServiceAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token := strings.TrimSpace(c.Request().Header.Get(serviceTokenHeader))

			if token == "" {
				if m.logger != nil {
					m.logger.Warn("service auth failed: missing token header",
						"path", c.Request().URL.Path,
						"remote_addr", c.RealIP(),
					)
				}
				return echo.NewHTTPError(http.StatusUnauthorized, map[string]string{
					"error":  "Unauthorized",
					"detail": "missing authentication headers",
				})
			}

			if m.serviceSecret == "" {
				if m.logger != nil {
					m.logger.Error("service auth failed: SERVICE_SECRET not configured")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, map[string]string{
					"error":  "Internal Server Error",
					"detail": "service authentication not properly configured",
				})
			}

			if token != m.serviceSecret {
				if m.logger != nil {
					m.logger.Warn("service auth failed: invalid token",
						"path", c.Request().URL.Path,
						"remote_addr", c.RealIP(),
					)
				}
				return echo.NewHTTPError(http.StatusUnauthorized, map[string]string{
					"error":  "Unauthorized",
					"detail": "invalid authentication token",
				})
			}

			if m.logger != nil {
				m.logger.Debug("service auth successful",
					"path", c.Request().URL.Path,
					"remote_addr", c.RealIP(),
				)
			}

			// Set a flag to indicate this is an authenticated service request
			c.Set("service.authenticated", true)

			return next(c)
		}
	}
}
