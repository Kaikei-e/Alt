package rest_feeds

import (
	"alt/di"
	"alt/usecase/fetch_random_subscription_usecase"
	"alt/utils/logger"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
)

// RandomFeedResponse represents the response for the random subscription endpoint.
type RandomFeedResponse struct {
	Feed *RandomFeedData `json:"feed"`
}

// RandomFeedData represents the feed data in the response.
type RandomFeedData struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// RestHandleFetchRandomSubscription returns a random feed from the user's subscriptions.
func RestHandleFetchRandomSubscription(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()

		logger.Logger.InfoContext(ctx, "fetching random subscription")

		feed, err := container.FetchRandomSubscriptionUsecase.Execute(ctx)
		if err != nil {
			if errors.Is(err, fetch_random_subscription_usecase.ErrNoSubscriptions) {
				logger.Logger.WarnContext(ctx, "no subscriptions found")
				return c.JSON(http.StatusNotFound, map[string]string{
					"error": "no subscriptions found",
				})
			}
			logger.Logger.ErrorContext(ctx, "failed to fetch random subscription", "error", err)
			return HandleError(c, err, "fetch_random_subscription")
		}

		response := RandomFeedResponse{
			Feed: &RandomFeedData{
				ID:          feed.ID.String(),
				URL:         feed.Link, // Link is the website URL
				Title:       feed.Title,
				Description: feed.Description,
			},
		}

		// No caching for random results
		c.Response().Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")

		return c.JSON(http.StatusOK, response)
	}
}
