package rest

import (
	"alt/config"
	"alt/di"
	"alt/domain"
	middleware_custom "alt/middleware"
	"alt/utils/logger"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v4"
)

func registerImageRoutes(v1 *echo.Group, container *di.ApplicationComponents, cfg *config.Config) {
	// 認証ミドルウェアの初期化
	authMiddleware := middleware_custom.NewAuthMiddleware(logger.Logger, cfg.Auth.SharedSecret, cfg)

	// 画像取得も認証必須
	images := v1.Group("/images", authMiddleware.RequireAuth())
	images.POST("/fetch", handleImageFetch(container))
}

func handleImageFetch(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req ImageFetchRequest
		if err := c.Bind(&req); err != nil {
			return HandleValidationError(c, "Invalid request format", "body", "malformed JSON")
		}

		// Basic validation
		if req.URL == "" {
			return HandleValidationError(c, "URL is required", "url", req.URL)
		}

		// Parse and validate URL
		parsedURL, err := url.Parse(req.URL)
		if err != nil {
			return HandleValidationError(c, "Invalid URL format", "url", req.URL)
		}

		// Apply SSRF protection
		if err := IsAllowedURL(parsedURL); err != nil {
			return HandleValidationError(c, fmt.Sprintf("URL not allowed: %v", err), "url", req.URL)
		}

		// Convert options from schema to domain
		var options *domain.ImageFetchOptions
		if req.Options != nil {
			options = &domain.ImageFetchOptions{
				MaxSize: req.Options.MaxSize,
				Timeout: time.Duration(req.Options.Timeout) * time.Second,
			}
		}

		// Execute usecase
		result, err := container.ImageFetchUsecase.Execute(c.Request().Context(), req.URL, options)
		if err != nil {
			return HandleError(c, err, "image_fetch")
		}

		// Return the image data directly with proper content type and COEP compliance
		c.Response().Header().Set("Content-Type", result.ContentType)
		c.Response().Header().Set("Cache-Control", "public, max-age=3600") // Cache for 1 hour
		c.Response().Header().Set("X-Image-Source", "alt-backend-proxy")   // For debugging

		// COEP (Cross-Origin Embedder Policy) compliance headers
		c.Response().Header().Set("Cross-Origin-Resource-Policy", "cross-origin") // Allow cross-origin usage
		c.Response().Header().Set("Access-Control-Allow-Origin", "*")             // CORS support
		c.Response().Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		c.Response().Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		return c.Blob(http.StatusOK, result.ContentType, result.Data)
	}
}
