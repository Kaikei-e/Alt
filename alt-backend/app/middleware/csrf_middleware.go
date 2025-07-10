package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// CSRFTokenUsecase defines the interface for CSRF token operations
type CSRFTokenUsecase interface {
	GenerateToken(ctx context.Context) (string, error)
	ValidateToken(ctx context.Context, token string) (bool, error)
}

// CSRFMiddleware creates middleware for CSRF protection
func CSRFMiddleware(usecase CSRFTokenUsecase) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip CSRF protection for non-state-changing methods and exempt endpoints
			if !isCSRFProtectedEndpoint(c.Request().Method, c.Request().URL.Path) {
				return next(c)
			}

			// Get CSRF token from header
			csrfToken := c.Request().Header.Get("X-CSRF-Token")
			if csrfToken == "" {
				return echo.NewHTTPError(http.StatusForbidden, map[string]interface{}{
					"error":   "csrf_token_missing",
					"message": "CSRF token is required",
				})
			}

			// Validate CSRF token
			valid, err := usecase.ValidateToken(c.Request().Context(), csrfToken)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, map[string]interface{}{
					"error":   "csrf_validation_error",
					"message": "Failed to validate CSRF token",
				})
			}

			if !valid {
				return echo.NewHTTPError(http.StatusForbidden, map[string]interface{}{
					"error":   "csrf_token_invalid",
					"message": "Invalid CSRF token",
				})
			}

			// Token is valid, proceed with request
			return next(c)
		}
	}
}

// CSRFTokenHandler creates a handler for CSRF token generation endpoint
func CSRFTokenHandler(usecase CSRFTokenUsecase) echo.HandlerFunc {
	return func(c echo.Context) error {
		token, err := usecase.GenerateToken(c.Request().Context())
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, map[string]interface{}{
				"error":   "csrf_token_generation_error",
				"message": "Failed to generate CSRF token",
			})
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"csrf_token": token,
		})
	}
}

// isCSRFProtectedEndpoint determines if the endpoint requires CSRF protection
func isCSRFProtectedEndpoint(method, path string) bool {
	// Only protect POST, PUT, PATCH, DELETE methods
	if method == "GET" || method == "HEAD" || method == "OPTIONS" {
		return false
	}

	// Exempt specific endpoints
	exemptEndpoints := []string{
		"/v1/health",
		"/v1/csrf-token",
		"/security/csp-report",
	}

	for _, exempt := range exemptEndpoints {
		if strings.Contains(path, exempt) {
			return false
		}
	}

	// Protect all other state-changing endpoints
	protectedEndpoints := []string{
		"/v1/feeds/read",
		"/v1/feeds/search",
		"/v1/feeds/fetch/details",
		"/v1/feeds/tags",
		"/v1/rss-feed-link/register",
		"/v1/feeds/register/favorite",
	}

	for _, protected := range protectedEndpoints {
		if strings.Contains(path, protected) {
			return true
		}
	}

	// Default to protecting all POST/PUT/PATCH/DELETE endpoints not explicitly exempted
	return true
}
