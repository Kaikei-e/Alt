package rest

import (
	"alt/config"
	"alt/di"
	middleware_custom "alt/middleware"
	"alt/rest/rest_feeds"
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

	// 2.5. Body size limit - メモリ枯渇 DoS 対策 (H-005)。2MB は OPML import
	// の 1MB per-file 上限 + multipart オーバーヘッドを許容する値。streaming
	// 系は stream 読み込みに干渉する懸念があるため Skipper で除外する。
	e.Use(middleware.BodyLimitWithConfig(middleware.BodyLimitConfig{
		Skipper: func(c echo.Context) bool {
			return strings.Contains(c.Path(), "/sse/") || strings.Contains(c.Path(), "/stream")
		},
		Limit: "2M",
	}))

	// 3. Security headers - セキュリティ設定を早期に適用 (M-008)
	// alt-backend は HTML を返さない API なので CSP は最小許可。frame-ancestors,
	// base-uri, form-action を明示し、違反は /security/csp-report で受ける。
	// Referrer-Policy / Permissions-Policy で外部参照と機能アクセスを制限。
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		HSTSMaxAge:            31536000,
		HSTSPreloadEnabled:    true,
		ContentSecurityPolicy: "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'; report-uri /security/csp-report",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
	}))

	// 4. CORS middleware - クロスオリジン制御
	// X-Alt-Backend-Token は JWT 認証ヘッダなので preflight で許可する (M-009)。
	// Permissions-Policy はエッジ層で配信するため Echo Secure ではなく nginx 側で
	// 設定する想定。
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: cfg.Server.CORSAllowedOrigins,
		AllowMethods: []string{echo.GET, echo.POST, echo.PUT, echo.DELETE, echo.OPTIONS},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			"Cache-Control",
			"Authorization",
			"X-Requested-With",
			"X-CSRF-Token",
			"X-Alt-Backend-Token",
		},
		MaxAge: 86400, // Cache preflight for 24 hours
	}))

	// 5. DOS protection - 悪意のあるリクエストを早期にブロック
	// /v1/sse/ は H-001 で削除済みのため WhitelistedPaths から外した。残る
	// /v1/feeds/summarize/stream は Connect-RPC streaming に未移行の summarize
	// 経路で、レート制限から除外する必要がある (M-005 で完全一致比較に修正)。
	// trustForwardedHeaders=true は alt-backend が常に nginx の背後に置かれて
	// X-Real-IP / X-Forwarded-For をエッジ層で設定し直すデプロイ前提 (M-007)。
	dosConfig := cfg.RateLimit.DOSProtection
	dosConfig.WhitelistedPaths = []string{"/v1/health", "/v1/feeds/summarize/stream", "/security/csp-report", "/v1/images/proxy/"}
	e.Use(middleware_custom.DOSProtectionMiddlewareWithTrust(
		middleware_custom.ConvertConfigDOSProtection(dosConfig),
		true,
	))

	// 6. CSRF protection for state-changing operations
	e.Use(middleware_custom.CSRFMiddleware(container.CSRFTokenUsecase))

	// 7. Request timeout - リクエスト処理時間の制限
	e.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{
		Timeout: cfg.Server.ReadTimeout,
		Skipper: func(c echo.Context) bool {
			// Skip timeout for SSE and streaming endpoints
			return strings.Contains(c.Path(), "/sse/") || strings.Contains(c.Path(), "/stream")
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
			// Skip compression for already compressed content, SSE endpoints,
			// and /metrics. Prometheus 3.x reports the gzip-framed response as
			// "expected a valid start token, got \x1f" for this endpoint, and
			// the scrape payload is small enough that compression savings are
			// immaterial compared to parsing robustness.
			return strings.Contains(c.Request().Header.Get("Accept-Encoding"), "br") ||
				strings.Contains(c.Path(), "/health") ||
				strings.Contains(c.Path(), "/sse/") ||
				c.Path() == "/metrics"
		},
	}))

	// Create route groups with path probe middleware for debugging
	v1 := e.Group("/v1", middleware_custom.PathProbe)

	// Register handlers by category
	registerSecurityRoutes(e, container)
	rest_feeds.RegisterFeedRoutes(v1, container, cfg)
	// Register morning updates route
	registerMorningRoutes(v1, container, cfg)
	registerArticleRoutes(v1, container, cfg)
	fetchArticleRoutes(v1, container, cfg)
	registerImageRoutes(v1, container, cfg)
	registerImageProxyRoutes(v1, container, cfg)
	// SSE feed stats (/v1/sse/feeds/stats) は H-001 で削除し、Connect-RPC
	// `StreamFeedStats` (port 9101) に一本化された。
	registerRecapRoutes(v1, container, cfg)
	registerScrapingDomainRoutes(v1, container, cfg)
	registerDashboardRoutes(v1, container, cfg)
	RegisterAugurRoutes(e, v1, container)
	registerInternalRoutes(e, container)
}
