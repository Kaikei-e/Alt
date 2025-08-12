package rest

import (
	"alt/config"
	"alt/di"
	middleware_custom "alt/middleware"
	"alt/utils/logger"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func RegisterRoutes(e *echo.Echo, container *di.ApplicationComponents, cfg *config.Config) {
	// 1. Request ID middleware first - すべてのリクエストにIDを付与
	e.Use(middleware_custom.RequestIDMiddleware())

	// 2. Recovery middleware early - パニックを早期に捕捉
	e.Use(middleware.Recover())

	// 3. Security headers - セキュリティ設定を早期に適用
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		HSTSMaxAge:            31536000,
		ContentSecurityPolicy: "default-src 'self'",
	}))

	// 4. CORS middleware - クロスオリジン制御
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"http://localhost:3000", "http://localhost:80", "https://curionoah.com"},
		AllowMethods: []string{echo.GET, echo.POST, echo.PUT, echo.DELETE, echo.OPTIONS},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, "Cache-Control", "Authorization", "X-Requested-With", "X-CSRF-Token"},
		MaxAge:       86400, // Cache preflight for 24 hours
	}))

	// 5. DOS protection - 悪意のあるリクエストを早期にブロック
	dosConfig := cfg.RateLimit.DOSProtection
	dosConfig.WhitelistedPaths = []string{"/v1/health", "/v1/sse/", "/security/csp-report"}
	e.Use(middleware_custom.DOSProtectionMiddleware(middleware_custom.ConvertConfigDOSProtection(dosConfig)))

	// 6. CSRF protection - 認証が必要な場合 Temporarily disabled for development
	// e.Use(middleware_custom.CSRFMiddleware(container.CSRFTokenUsecase))

	// 7. Request timeout - リクエスト処理時間の制限
	e.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{
		Timeout: cfg.Server.ReadTimeout,
		Skipper: func(c echo.Context) bool {
			return strings.Contains(c.Path(), "/sse/")
		},
	}))

	// 8. Validation middleware - リクエスト内容の検証
	e.Use(middleware_custom.ValidationMiddleware())

	// 9. Logging middleware - 処理内容をログに記録
	e.Use(middleware_custom.LoggingMiddleware(logger.Logger))

	// 10. Compression middleware last - レスポンス時の圧縮（最後に実行）
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level: 5, // Balanced compression level
		Skipper: func(c echo.Context) bool {
			// Skip compression for already compressed content and SSE endpoints
			return strings.Contains(c.Request().Header.Get("Accept-Encoding"), "br") ||
				strings.Contains(c.Path(), "/health") ||
				strings.Contains(c.Path(), "/sse/")
		},
	}))

	// Create route groups
	v1 := e.Group("/v1")

	// Register handlers by category
	registerSecurityRoutes(e, container)
	registerFeedRoutes(v1, container, cfg)
	registerArticleRoutes(v1, container)
	registerImageRoutes(v1, container)
	registerSSERoutes(v1, container, cfg)
}