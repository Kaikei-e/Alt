package rest_feeds

import (
	"alt/di"
	"alt/utils/errors"
	"alt/utils/logger"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func RestHandleRegisterRSSFeed(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		var rssFeedLink RssFeedLink
		err := c.Bind(&rssFeedLink)
		if err != nil {
			return HandleValidationError(c, "Invalid request format", "body", "malformed JSON")
		}

		if strings.TrimSpace(rssFeedLink.URL) == "" {
			return HandleValidationError(c, "URL is required and cannot be empty", "url", rssFeedLink.URL)
		}

		// Parse and validate URL for SSRF protection
		parsedURL, err := url.Parse(rssFeedLink.URL)
		if err != nil {
			return HandleValidationError(c, "Invalid URL format", "url", rssFeedLink.URL)
		}

		// Apply SSRF protection
		err = IsAllowedURL(parsedURL)
		if err != nil {
			securityErr := errors.NewValidationContextError(
				"URL not allowed for security reasons",
				"rest",
				"RESTHandler",
				"register_feed",
				map[string]interface{}{
					"url":         rssFeedLink.URL,
					"reason":      err.Error(),
					"path":        c.Request().URL.Path,
					"method":      c.Request().Method,
					"remote_addr": c.Request().RemoteAddr,
					"request_id":  c.Response().Header().Get("X-Request-ID"),
				},
			)
			logger.Logger.Error("URL validation failed", "error", securityErr.Error(), "url", rssFeedLink.URL)
			return c.JSON(securityErr.HTTPStatusCode(), securityErr.ToHTTPResponse())
		}

		err = container.RegisterFeedsUsecase.Execute(c.Request().Context(), rssFeedLink.URL)
		if err != nil {
			return HandleError(c, err, "register_feed")
		}

		// Invalidate cache after registration
		c.Response().Header().Set("Cache-Control", "no-cache")
		return c.JSON(http.StatusOK, map[string]string{"message": "RSS feed link registered"})
	}
}

func RestHandleListRSSFeedLinks(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		links, err := container.ListFeedLinksUsecase.Execute(c.Request().Context())
		if err != nil {
			return HandleError(c, err, "list_feed_links")
		}
		return c.JSON(http.StatusOK, links)
	}
}

func RestHandleDeleteRSSFeedLink(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		idParam := c.Param("id")
		linkID, err := uuid.Parse(idParam)
		if err != nil {
			return HandleValidationError(c, "Invalid feed link ID", "id", idParam)
		}

		if err := container.DeleteFeedLinkUsecase.Execute(c.Request().Context(), linkID); err != nil {
			return HandleError(c, err, "delete_feed_link")
		}

		c.Response().Header().Set("Cache-Control", "no-cache")
		return c.JSON(http.StatusOK, map[string]string{"message": "Feed link deleted"})
	}
}
