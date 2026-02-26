package rest

import (
	"alt/config"
	"alt/di"
	"net/http"
	"strings"

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

		result, err := container.ImageProxyUsecase.ProxyImage(c.Request().Context(), sig, encodedURL)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "signature verification failed") {
				return c.String(http.StatusForbidden, "invalid signature")
			}
			if strings.Contains(errMsg, "domain not allowed") {
				return c.String(http.StatusForbidden, "domain not allowed")
			}
			return c.String(http.StatusBadGateway, "image proxy error")
		}

		// Check If-None-Match for 304
		ifNoneMatch := c.Request().Header.Get("If-None-Match")
		if ifNoneMatch != "" && ifNoneMatch == `"`+result.ETag+`"` {
			return c.NoContent(http.StatusNotModified)
		}

		// Set cache headers
		c.Response().Header().Set("Content-Type", result.ContentType)
		c.Response().Header().Set("Cache-Control", "public, max-age=43200, immutable")
		c.Response().Header().Set("ETag", `"`+result.ETag+`"`)
		c.Response().Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
		c.Response().Header().Set("Vary", "Accept-Encoding")

		return c.Blob(http.StatusOK, result.ContentType, result.Data)
	}
}
