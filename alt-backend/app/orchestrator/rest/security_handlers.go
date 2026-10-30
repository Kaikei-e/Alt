package rest

import (
	"alt/di"
	middleware_custom "alt/middleware"
	"alt/utils/logger"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

const maxCSPReportBodyBytes = 16 * 1024

func registerSecurityRoutes(e *echo.Echo, container *di.ApplicationComponents) {
	v1 := e.Group("/v1")

	// CSRF token generation endpoint
	v1.GET("/csrf-token", middleware_custom.CSRFTokenHandler(container.CSRFTokenUsecase))

	// Health check is liveness only. Dependency reachability lives on the
	// ops listener at /health/deep — this public route used to hardcode
	// `"database":"connected"` while cmd/backend has no database at all.
	v1.GET("/health", restHandleLiveness)

	// CSP report endpoint
	e.POST("/security/csp-report", handleCSPReport)
}

func handleCSPReport(c echo.Context) error {
	ctx := c.Request().Context()
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maxCSPReportBodyBytes)
	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "CSP report body too large")
		}
		logger.Logger.ErrorContext(ctx, "Failed to read request body", "error", err)
		return echo.NewHTTPError(http.StatusBadRequest, "Failed to read request body")
	}

	// Try to parse as JSON
	var report map[string]interface{}
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &report); err != nil {
			logger.Logger.WarnContext(ctx, "CSP Report - Invalid JSON", "body_len", len(bodyBytes), "error", err)
			// Return 204 even for invalid JSON to prevent browser retries
			return c.NoContent(http.StatusNoContent)
		}
	}

	// Log CSP violation report
	logger.Logger.WarnContext(ctx, "CSP Violation Report",
		"timestamp", time.Now().Format(time.RFC3339),
		"report", report,
		"user_agent", c.Request().UserAgent(),
		"ip", c.RealIP(),
	)

	// Return 204 No Content for CSP reports
	return c.NoContent(http.StatusNoContent)
}

func restHandleLiveness(c echo.Context) error {
	c.Response().Header().Set("Cache-Control", "public, max-age=30")
	return c.JSON(http.StatusOK, map[string]string{
		"status": "healthy",
	})
}
