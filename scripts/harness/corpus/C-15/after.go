package bootstrap

import (
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func registerSummarizeGroup(e *echo.Echo, handler echo.HandlerFunc) {
	api := e.Group("/api/v1")
	api.Use(middleware.BodyLimit(1 << 20))
	api.POST("/summarize", handler)
}
