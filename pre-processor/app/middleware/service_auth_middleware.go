package middleware

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
)

const serviceTokenHeader = "X-Service-Token"

type ServiceAuthMiddleware struct {
	logger        *slog.Logger
	serviceSecret string
}

func NewServiceAuthMiddleware(logger *slog.Logger) *ServiceAuthMiddleware {
	secret := os.Getenv("SERVICE_SECRET")
	if secretFile := os.Getenv("SERVICE_SECRET_FILE"); secretFile != "" {
		content, err := os.ReadFile(secretFile) // #nosec G304 -- path is env-configured Docker Secrets mount
		if err == nil {
			secret = strings.TrimSpace(string(content))
		} else if logger != nil {
			logger.Error("failed to read SERVICE_SECRET_FILE", "error", err)
		}
	}

	if secret == "" && logger != nil {
		logger.Warn("SERVICE_SECRET not set, service auth will deny all requests")
	}

	return &ServiceAuthMiddleware{
		logger:        logger,
		serviceSecret: secret,
	}
}

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

			expected := []byte(m.serviceSecret)
			provided := []byte(token)
			valid := len(expected) == len(provided) &&
				subtle.ConstantTimeCompare(expected, provided) == 1
			if !valid {
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

			c.Set("service.authenticated", true)
			return next(c)
		}
	}
}
