package rest

import (
	"alt/di"
	"alt/utils/logger"
	"net/http"

	"github.com/labstack/echo/v4"
)

func registerInternalRoutes(e *echo.Echo, container *di.ApplicationComponents) {
	// Internal routes group - restricted access recommended in production (e.g. via network policy)
	v1 := e.Group("/v1/internal")

	v1.GET("/system-user", func(c echo.Context) error {
		ctx := c.Request().Context()

		// Query the database directly via pool
		// We just need any valid user ID to associate system-generated/synced articles with
		// In a single-user system, getting the first user is sufficient
		var userID string
		err := container.AltDBRepository.GetPool().QueryRow(ctx, "SELECT id FROM users LIMIT 1").Scan(&userID)
		if err != nil {
			logger.Logger.Error("Failed to fetch system user", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to fetch system user",
			})
		}

		return c.JSON(http.StatusOK, map[string]string{
			"user_id": userID,
		})
	})
}
