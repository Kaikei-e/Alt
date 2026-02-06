package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// HealthHandler handles health check requests.
type HealthHandler struct{}

// NewHealthHandler creates a new health handler.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Handle processes the /health endpoint.
func (h *HealthHandler) Handle(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"status": "healthy",
	})
}
