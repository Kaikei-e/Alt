package bootstrap

import (
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

// Matches Echo v4 middleware.BodyLimit("1M") decimal megabyte (1,000,000 bytes).
const maxRequestBodyBytes = 1_000_000

func registerSummarizeGroup(e *echo.Echo, handler echo.HandlerFunc) {
	api := e.Group("/api/v1")
	api.Use(middleware.BodyLimit(maxRequestBodyBytes))
	api.POST("/summarize", handler)
}
