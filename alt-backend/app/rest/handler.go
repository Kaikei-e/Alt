package rest

import (
	"net/http"
	"github.com/labstack/echo/v4"
)

func RegisterRoutes(e *echo.Echo) {
	v1 := e.Group("/v1")
	v1.GET("/health", func(c echo.Context) error {
		response := map[string]string{
			"status": "healthy",
		}
		return c.JSON(http.StatusOK, response)
	})
	
	v1.GET("/collectedFeeds", func(c echo.Context) error {
		return c.JSON(http.StatusOK, "Hello, World!")
	})
}

