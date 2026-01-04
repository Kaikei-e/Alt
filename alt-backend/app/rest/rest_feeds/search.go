package rest_feeds

import (
	"alt/di"
	"alt/utils/logger"
	"net/http"

	"github.com/labstack/echo/v4"
)

func RestHandleSearchFeeds(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		var payload FeedSearchPayload
		err := c.Bind(&payload)
		if err != nil {
			logger.Logger.Error("Error binding search payload", "error", err)
			return HandleValidationError(c, "Invalid request format", "body", nil)
		}

		if payload.Query == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Search query must not be empty"})
		}

		// Default values for pagination
		offset := 0
		if payload.Cursor != nil {
			offset = *payload.Cursor
			if offset < 0 {
				offset = 0
			}
		}

		limit := 20
		if payload.Limit != nil {
			limit = *payload.Limit
			if limit <= 0 {
				limit = 20
			}
			if limit > 100 {
				limit = 100
			}
		}

		// Check if pagination is requested
		if payload.Cursor != nil || payload.Limit != nil {
			// Use pagination-aware usecase
			logger.Logger.Info("Executing feed search with pagination",
				"query", payload.Query,
				"offset", offset,
				"limit", limit)
			results, hasMore, err := container.FeedSearchUsecase.ExecuteWithPagination(c.Request().Context(), payload.Query, offset, limit)
			if err != nil {
				logger.Logger.Error("Error executing feed search with pagination", "error", err, "query", payload.Query)
				return HandleError(c, err, "SearchFeedsWithPagination")
			}

			logger.Logger.Info("Feed search with pagination completed successfully",
				"query", payload.Query,
				"results_count", len(results),
				"has_more", hasMore)

			// Optimize response size for search results
			optimizedFeeds := OptimizeFeedsResponseForSearch(results)

			// Prepare cursor-based response
			var nextCursor *int
			if hasMore {
				nextOffset := offset + len(results)
				nextCursor = &nextOffset
			}

			response := map[string]interface{}{
				"data":        optimizedFeeds,
				"has_more":    hasMore,
				"next_cursor": nextCursor,
			}

			// Add cache headers for search results (short TTL since results may change)
			c.Response().Header().Set("Cache-Control", "private, max-age=30")

			return c.JSON(http.StatusOK, response)
		}

		// Fallback to non-paginated search for backward compatibility
		logger.Logger.Info("Executing feed search", "query", payload.Query)
		results, err := container.FeedSearchUsecase.Execute(c.Request().Context(), payload.Query)
		if err != nil {
			logger.Logger.Error("Error executing feed search", "error", err, "query", payload.Query)
			return HandleError(c, err, "SearchFeeds")
		}

		logger.Logger.Info("Feed search completed successfully", "query", payload.Query, "results_count", len(results))

		// Optimize response size for search results (200 chars for description)
		optimizedFeeds := OptimizeFeedsResponseForSearch(results)

		// Add cache headers for search results (short TTL since results may change)
		c.Response().Header().Set("Cache-Control", "private, max-age=30")

		return c.JSON(http.StatusOK, optimizedFeeds)
	}
}
