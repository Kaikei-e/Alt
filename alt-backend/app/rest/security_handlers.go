package rest

import (
	"alt/di"
	middleware_custom "alt/middleware"
	"alt/utils/logger"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

func registerSecurityRoutes(e *echo.Echo, container *di.ApplicationComponents) {
	v1 := e.Group("/v1")

	// CSRF token generation endpoint
	v1.GET("/csrf-token", middleware_custom.CSRFTokenHandler(container.CSRFTokenUsecase))

	// Health check with database connectivity test
	v1.GET("/health", func(c echo.Context) error {
		// Set cache headers for health check
		c.Response().Header().Set("Cache-Control", "public, max-age=30")

		response := map[string]string{
			"status": "healthy",
		}

		response["database"] = "connected"
		return c.JSON(http.StatusOK, response)
	})

	// CSP report endpoint
	e.POST("/security/csp-report", func(c echo.Context) error {
		// Read raw body for debugging
		body, err := io.ReadAll(c.Request().Body)
		if err != nil {
			logger.Logger.Error("Failed to read request body", "error", err)
			return echo.NewHTTPError(http.StatusBadRequest, "Failed to read request body")
		}

		// Log raw body for debugging
		logger.Logger.Info("CSP Report Raw Body", "body", string(body))

		// Try to parse as JSON
		var report map[string]interface{}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &report); err != nil {
				logger.Logger.Warn("CSP Report - Invalid JSON", "body", string(body), "error", err)
				// Return 204 even for invalid JSON to prevent browser retries
				return c.NoContent(http.StatusNoContent)
			}
		}

		// Log CSP violation report
		logger.Logger.Warn("CSP Violation Report",
			"timestamp", time.Now().Format(time.RFC3339),
			"report", report,
			"user_agent", c.Request().UserAgent(),
			"ip", c.RealIP(),
		)

		// Return 204 No Content for CSP reports
		return c.NoContent(http.StatusNoContent)
	})
}
