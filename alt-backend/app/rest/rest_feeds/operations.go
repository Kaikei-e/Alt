package rest_feeds

import (
	"alt/di"
	"alt/utils/errors"
	"alt/utils/logger"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

func RestHandleMarkFeedAsRead(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		var readStatus ReadStatus
		err := c.Bind(&readStatus)
		if err != nil {
			logger.Logger.Error("Error binding read status", "error", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		feedURL, err := url.Parse(readStatus.FeedURL)
		if err != nil {
			logger.Logger.Error("Error parsing feed URL", "error", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		err = container.FeedsReadingStatusUsecase.Execute(c.Request().Context(), *feedURL)
		if err != nil {
			logger.Logger.Error("Error updating feed read status", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		logger.Logger.Info("Feed read status updated", "feedURL", feedURL)

		// Invalidate cache after update
		c.Response().Header().Set("Cache-Control", "no-cache")
		return c.JSON(http.StatusOK, map[string]string{"message": "Feed read status updated"})
	}
}

func RestHandleRegisterFavoriteFeed(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		var payload RssFeedLink
		if err := c.Bind(&payload); err != nil {
			return HandleValidationError(c, "Invalid request format", "body", "malformed JSON")
		}

		if strings.TrimSpace(payload.URL) == "" {
			return HandleValidationError(c, "URL is required and cannot be empty", "url", payload.URL)
		}

		parsedURL, err := url.Parse(payload.URL)
		if err != nil {
			return HandleValidationError(c, "Invalid URL format", "url", payload.URL)
		}

		if err = IsAllowedURL(parsedURL); err != nil {
			securityErr := errors.NewValidationContextError(
				"URL not allowed for security reasons",
				"rest",
				"RESTHandler",
				"register_favorite_feed",
				map[string]interface{}{
					"url":         payload.URL,
					"reason":      err.Error(),
					"path":        c.Request().URL.Path,
					"method":      c.Request().Method,
					"remote_addr": c.Request().RemoteAddr,
					"request_id":  c.Response().Header().Get("X-Request-ID"),
				},
			)
			logger.Logger.Error("URL validation failed", "error", securityErr.Error(), "url", payload.URL)
			return c.JSON(securityErr.HTTPStatusCode(), securityErr.ToHTTPResponse())
		}

		if err = container.RegisterFavoriteFeedUsecase.Execute(c.Request().Context(), payload.URL); err != nil {
			return HandleError(c, err, "register_favorite_feed")
		}

		c.Response().Header().Set("Cache-Control", "no-cache")
		return c.JSON(http.StatusOK, map[string]string{"message": "favorite feed registered"})
	}
}

func RestHandleUnreadCount(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		sinceStr := c.QueryParam("since")
		var since time.Time
		var err error
		if sinceStr != "" {
			since, err = time.Parse(time.RFC3339, sinceStr)
			if err != nil {
				logger.Logger.Error("Invalid since parameter", "error", err)
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid since parameter"})
			}
		} else {
			now := time.Now().UTC()
			since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		}

		count, err := container.TodayUnreadArticlesCountUsecase.Execute(c.Request().Context(), since)
		if err != nil {
			logger.Logger.Error("Error fetching unread count", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch unread count"})
		}

		return c.JSON(http.StatusOK, map[string]int{"count": count})
	}
}
