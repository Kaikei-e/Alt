package rest

import (
	"alt/config"
	"alt/di"
	middleware_custom "alt/middleware"
	"alt/orchestrator/rest/rest_feeds"
	"alt/utils/logger"
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// ctx bounds the lifetime of any background goroutines route registration
// starts (e.g. the DoS-protection limiter-cleanup ticker); pass the server's
// process lifetime context here, not a per-request one.
func RegisterRoutes(ctx context.Context, e *echo.Echo, container *di.ApplicationComponents, cfg *config.Config) {
	trustedProxies := config.TrustedProxyCIDRsFromEnv()
	e.IPExtractor = middleware_custom.NewTrustedProxyIPExtractor(trustedProxies)

	// 1. Request ID middleware first - すべてのリクエストにIDを付与
	e.Use(middleware_custom.RequestIDMiddleware())

	// 2. Recovery middleware early - パニックを早期に捕捉
	e.Use(middleware.Recover())

	e.Use(middleware.BodyLimit("2M"))

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
	dosMiddlewareCfg := middleware_custom.ConvertConfigDOSProtection(dosConfig)
	dosMiddlewareCfg.TrustedProxies = trustedProxies
	e.Use(middleware_custom.DOSProtectionMiddleware(ctx, dosMiddlewareCfg))

	// 6. CSRF protection for state-changing operations
	e.Use(middleware_custom.CSRFMiddleware(container.CSRFTokenUsecase))

	// 7. Request timeout - リクエスト処理時間の制限
	e.Use(streamingBodyReadTimeoutMiddleware(cfg.Server.ReadTimeout))
	e.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{
		Timeout: cfg.Server.ReadTimeout,
		Skipper: isStreamingRoute,
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
	registerMorningRoutes(v1, container, cfg.Auth)
	registerArticleRoutes(v1, container, cfg.Auth)
	fetchArticleRoutes(v1, container, cfg.Auth)
	registerImageRoutes(v1, container, cfg.Auth)
	registerImageProxyRoutes(v1, container, cfg)
	// SSE feed stats (/v1/sse/feeds/stats) は H-001 で削除し、Connect-RPC
	// `StreamFeedStats` (port 9101) に一本化された。
	// /v1/recap/articles も Connect-RPC
	// `services.datahub.v1.DataHubService/ListRecapArticles` に移行済。
	registerScrapingDomainRoutes(v1, container, cfg.Auth)
	registerDashboardRoutes(v1, container, cfg.Auth)
	RegisterAugurRoutes(e, v1, container, cfg.Auth)
	// /v1/internal/* is deliberately absent here — and no longer exists at all.
	// It moved to cmd/datahub with the split, and ADR-000954 D6 then folded its
	// two routes into services.datahub.v1.DataHubService, so there is no handler
	// left for the browser-facing router to regain by someone adding one
	// call.
}

func isStreamingRoute(c echo.Context) bool {
	path := c.Path()
	if path == "" {
		path = c.Request().URL.Path
	}
	return path == "/v1/feeds/summarize/stream" || strings.HasPrefix(path, "/sse/")
}

func streamingBodyReadTimeoutMiddleware(readTimeout time.Duration) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if readTimeout <= 0 || !isStreamingRoute(c) {
				return next(c)
			}
			req := c.Request()
			if req.Body == nil || req.Body == http.NoBody {
				return next(c)
			}

			deadline := time.Now().Add(readTimeout)
			ctrl := http.NewResponseController(c.Response().Writer)
			_ = ctrl.SetReadDeadline(deadline)

			wrapped := &deadlineBodyReader{
				rc:       req.Body,
				ctrl:     ctrl,
				deadline: deadline,
			}
			req.Body = wrapped

			err := next(c)
			if wrapped.timedOut.Load() && !c.Response().Committed {
				return echo.NewHTTPError(http.StatusRequestTimeout, "request body read timed out")
			}
			return err
		}
	}
}

type deadlineBodyReader struct {
	rc       io.ReadCloser
	ctrl     *http.ResponseController
	deadline time.Time
	timedOut atomic.Bool
}

type bodyReadResult struct {
	data []byte
	err  error
}

func (r *deadlineBodyReader) Read(p []byte) (int, error) {
	rem := time.Until(r.deadline)
	if rem <= 0 {
		r.timedOut.Store(true)
		return 0, context.DeadlineExceeded
	}

	ch := make(chan bodyReadResult, 1)
	buf := make([]byte, len(p))
	go func() {
		n, err := r.rc.Read(buf)
		ch <- bodyReadResult{data: buf[:n], err: err}
	}()

	timer := time.NewTimer(rem)
	defer timer.Stop()

	select {
	case res := <-ch:
		n := copy(p, res.data)
		if res.err == io.EOF {
			_ = r.ctrl.SetReadDeadline(time.Time{})
		}
		return n, res.err
	case <-timer.C:
		r.timedOut.Store(true)
		return 0, context.DeadlineExceeded
	}
}

func (r *deadlineBodyReader) Close() error {
	_ = r.ctrl.SetReadDeadline(time.Time{})
	return r.rc.Close()
}
