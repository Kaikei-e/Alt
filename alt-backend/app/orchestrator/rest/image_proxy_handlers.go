package rest

import (
	"alt/config"
	"alt/di"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// registerImageProxyRoutes registers the image proxy REST endpoint.
// This endpoint is unauthenticated — HMAC signature serves as the authorization token.
// It exists because browsers need a URL for <img src> that returns raw image bytes.
func registerImageProxyRoutes(v1 *echo.Group, container *di.ApplicationComponents, cfg *config.Config) {
	if !cfg.ImageProxy.Enabled {
		return
	}

	proxy := v1.Group("/images/proxy")
	proxy.GET("/:sig/:url", handleImageProxy(container))
}

func handleImageProxy(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		sig := c.Param("sig")
		encodedURL := c.Param("url")

		if sig == "" || encodedURL == "" {
			return c.String(http.StatusBadRequest, "missing parameters")
		}

		// 10s timeout to fit within nginx's 15s proxy_read_timeout
		ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
		defer cancel()

		result, err := container.ImageProxyUsecase.ProxyImage(ctx, sig, encodedURL)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "signature verification failed") {
				return c.String(http.StatusForbidden, "invalid signature")
			}
			if strings.Contains(errMsg, "domain not allowed") {
				return c.String(http.StatusForbidden, "domain not allowed")
			}
			if context.Cause(ctx) == context.Canceled {
				return c.NoContent(499)
			}
			if errors.Is(err, context.DeadlineExceeded) {
				slog.WarnContext(c.Request().Context(), "image proxy timeout", "error", err)
				return c.NoContent(http.StatusGatewayTimeout)
			}
			if strings.Contains(errMsg, "rate limit") || strings.Contains(errMsg, "would exceed context deadline") {
				return c.NoContent(http.StatusTooManyRequests)
			}
			slog.WarnContext(c.Request().Context(), "image proxy error", "error", err)
			return c.NoContent(http.StatusBadGateway)
		}

		// Check If-None-Match for 304
		ifNoneMatch := c.Request().Header.Get("If-None-Match")
		if ifNoneMatch != "" && ifNoneMatch == `"`+result.ETag+`"` {
			return c.NoContent(http.StatusNotModified)
		}

		// Set cache headers — 7 day browser cache, immutable
		c.Response().Header().Set("Content-Type", result.ContentType)
		c.Response().Header().Set("Cache-Control", "public, max-age=604800, immutable")
		c.Response().Header().Set("ETag", `"`+result.ETag+`"`)
		c.Response().Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
		c.Response().Header().Set("Vary", "Accept-Encoding")

		return c.Blob(http.StatusOK, result.ContentType, result.Data)
	}
}
