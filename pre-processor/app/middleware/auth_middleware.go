// ABOUTME: This file handles authentication middleware for the pre-processor service
// ABOUTME: It integrates with the shared authentication library for service-to-service auth

package middleware

import (
	"fmt"
	"os"
	"time"

	"log/slog"

	"github.com/labstack/echo/v4"

	"pre-processor/internal/auth"
	"pre-processor/internal/auth/middleware"
)

func SetupAuth(e *echo.Echo, logger *slog.Logger) error {
	config := auth.Config{
		AuthServiceURL: getEnv("AUTH_SERVICE_URL", "http://auth-service:8080"),
		ServiceName:    "pre-processor",
		ServiceSecret:  getEnv("SERVICE_SECRET", ""),
		TokenTTL:       time.Hour,
	}

	if config.ServiceSecret == "" {
		logger.Error("SERVICE_SECRET environment variable is required")
		return fmt.Errorf("SERVICE_SECRET environment variable is required")
	}

	authClient := auth.NewClient(config)
	authMiddleware := middleware.NewAuthMiddleware(authClient)

	// 認証が必要なエンドポイント
	private := e.Group("/api/v1")
	private.Use(authMiddleware.RequireAuth())

	// ユーザー固有の処理エンドポイント
	private.POST("/sync", handleSyncUserSubscriptions)
	private.GET("/articles", handleGetUserArticles)
	private.POST("/articles/fetch", handleFetchUserArticles)

	// サービス間通信用エンドポイント
	internal := e.Group("/internal")
	internal.Use(authMiddleware.RequireServiceAuth())

	internal.POST("/process", handleProcessArticles)
	internal.GET("/health", handleHealthCheck)

	logger.Info("authentication middleware configured",
		"auth_service_url", config.AuthServiceURL,
		"service_name", config.ServiceName)

	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Placeholder handlers - these will be implemented with proper business logic
func handleSyncUserSubscriptions(c echo.Context) error {
	// TODO: Implement user subscription sync
	return c.JSON(200, map[string]string{"status": "not implemented"})
}

func handleGetUserArticles(c echo.Context) error {
	// TODO: Implement user articles retrieval
	return c.JSON(200, map[string]string{"status": "not implemented"})
}

func handleFetchUserArticles(c echo.Context) error {
	// TODO: Implement user articles fetching
	return c.JSON(200, map[string]string{"status": "not implemented"})
}

func handleProcessArticles(c echo.Context) error {
	// TODO: Implement article processing
	return c.JSON(200, map[string]string{"status": "not implemented"})
}

func handleHealthCheck(c echo.Context) error {
	// TODO: Implement health check
	return c.JSON(200, map[string]string{"status": "healthy"})
}
