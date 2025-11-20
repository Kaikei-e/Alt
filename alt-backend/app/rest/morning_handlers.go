package rest

import (
	"alt/di"
	"alt/domain"
	middleware_custom "alt/middleware"
	"alt/utils/logger"
	"net/http"

	"github.com/labstack/echo/v4"
)

// registerMorningRoutes registers the morning letter routes.
func registerMorningRoutes(v1 *echo.Group, container *di.ApplicationComponents) {
	// 認証ミドルウェアの初期化
	authMiddleware := middleware_custom.NewAuthMiddleware(logger.Logger)

	// Morning letter endpoints (authentication required)
	morning := v1.Group("/morning-letter", authMiddleware.RequireAuth())
	morning.GET("/updates", handleMorningUpdates(container))
}

// handleMorningUpdates returns the overnight morning updates for the authenticated user.
func handleMorningUpdates(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		// Retrieve user from context
		user, err := domain.GetUserFromContext(ctx)
		if err != nil {
			logger.Logger.Error("user context not found", "error", err)
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		}
		// Call usecase
		updates, err := container.MorningUsecase.GetOvernightUpdates(ctx, user.UserID.String())
		if err != nil {
			return handleError(c, err, "morning_updates")
		}
		return c.JSON(http.StatusOK, updates)
	}
}
