package rest_feeds

import (
	"alt/di"
	"alt/domain"
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
		ctx := c.Request().Context()
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
			logger.Logger.ErrorContext(ctx, "URL validation failed", "error", securityErr.Error(), "url", rssFeedLink.URL)
			return c.JSON(securityErr.HTTPStatusCode(), securityErr.ToHTTPResponse())
		}

		err = container.RegisterFeedsUsecase.Execute(ctx, rssFeedLink.URL)
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
		links, err := container.ListFeedLinksWithHealthUsecase.Execute(c.Request().Context())
		if err != nil {
			return HandleError(c, err, "list_feed_links")
		}

		type feedLinkResponse struct {
			ID                  string  `json:"id"`
			URL                 string  `json:"url"`
			HealthStatus        string  `json:"health_status"`
			ConsecutiveFailures int     `json:"consecutive_failures"`
			IsActive            bool    `json:"is_active"`
			LastFailureReason   *string `json:"last_failure_reason,omitempty"`
		}

		result := make([]feedLinkResponse, 0, len(links))
		for _, link := range links {
			resp := feedLinkResponse{
				ID:           link.ID.String(),
				URL:          link.URL,
				HealthStatus: string(link.GetHealthStatus()),
			}
			if link.Availability != nil {
				resp.ConsecutiveFailures = link.Availability.ConsecutiveFailures
				resp.IsActive = link.Availability.IsActive
				resp.LastFailureReason = link.Availability.LastFailureReason
			}
			result = append(result, resp)
		}
		return c.JSON(http.StatusOK, result)
	}
}

func RestHandleDeleteRSSFeedLink(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		idParam := c.Param("id")
		linkID, err := uuid.Parse(idParam)
		if err != nil {
			return HandleValidationError(c, "Invalid feed link ID", "id", idParam)
		}

		userCtx, err := domain.GetUserFromContext(ctx)
		if err != nil {
			return HandleError(c, err, "delete_feed_link")
		}

		if err := container.DeleteFeedLinkUsecase.Execute(ctx, userCtx.UserID, linkID); err != nil {
			return HandleError(c, err, "delete_feed_link")
		}

		c.Response().Header().Set("Cache-Control", "no-cache")
		return c.JSON(http.StatusOK, map[string]string{"message": "Feed unsubscribed"})
	}
}
