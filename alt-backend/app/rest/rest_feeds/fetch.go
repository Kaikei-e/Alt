package rest_feeds

import (
	"alt/config"
	"alt/di"
	"alt/utils/logger"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

func RestHandleFetchSingleFeed(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Add caching headers
		c.Response().Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(cfg.Cache.FeedCacheExpiry.Seconds())))
		c.Response().Header().Set("ETag", `"single-feed"`)

		feed, err := container.FetchSingleFeedUsecase.Execute(c.Request().Context())
		if err != nil {
			return HandleError(c, err, "fetch_single_feed")
		}
		return c.JSON(http.StatusOK, feed)
	}
}

func RestHandleFetchFeedsList(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Add caching headers for feed list
		c.Response().Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(cfg.Cache.SearchCacheExpiry.Seconds())))
		c.Response().Header().Set("ETag", `"feeds-list"`)

		feeds, err := container.FetchFeedsListUsecase.Execute(c.Request().Context())
		if err != nil {
			return HandleError(c, err, "fetch_feeds_list")
		}

		// Optimize response size
		optimizedFeeds := OptimizeFeedsResponse(feeds)
		return c.JSON(http.StatusOK, optimizedFeeds)
	}
}

func RestHandleFetchFeedsLimit(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		limit, err := strconv.Atoi(c.Param("limit"))
		if err != nil {
			return HandleValidationError(c, "Invalid limit parameter", "limit", c.Param("limit"))
		}

		// Validate limit to prevent excessive resource usage
		if limit > 1000 {
			limit = 1000
		}

		// Add caching headers based on limit
		cacheAge := GetCacheAgeForLimit(limit)
		c.Response().Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(cacheAge))
		c.Response().Header().Set("ETag", `"feeds-limit-`+strconv.Itoa(limit)+`"`)

		feeds, err := container.FetchFeedsListUsecase.ExecuteLimit(c.Request().Context(), limit)
		if err != nil {
			return HandleError(c, err, "fetch_feeds_limit")
		}

		optimizedFeeds := OptimizeFeedsResponse(feeds)
		return c.JSON(http.StatusOK, optimizedFeeds)
	}
}

func RestHandleFetchFeedsPage(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		page, err := strconv.Atoi(c.Param("page"))
		if err != nil {
			logger.Logger.Error("Invalid page parameter", "error", err, "page", c.Param("page"))
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid page parameter"})
		}

		// Validate page parameter
		if page < 0 {
			logger.Logger.Error("Negative page parameter", "page", page)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Page parameter must be non-negative"})
		}

		// Add caching headers for paginated results
		c.Response().Header().Set("Cache-Control", "public, max-age=600") // 10 minutes
		c.Response().Header().Set("ETag", `"feeds-page-`+strconv.Itoa(page)+`"`)

		feeds, err := container.FetchFeedsListUsecase.ExecutePage(c.Request().Context(), page)
		if err != nil {
			logger.Logger.Error("Error fetching feeds page", "error", err, "page", page)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch feeds page"})
		}

		optimizedFeeds := OptimizeFeedsResponse(feeds)
		return c.JSON(http.StatusOK, optimizedFeeds)
	}
}

func RestHandleFetchUnreadFeedsCursor(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Parse query parameters
		limitStr := c.QueryParam("limit")
		cursorStr := c.QueryParam("cursor")
		view := c.QueryParam("view") // "swipe" mode for optimized single-card response

		// Log incoming request parameters for debugging
		logger.Logger.Info(
			"received unread feeds cursor request",
			"cursor_param", cursorStr,
			"limit_param", limitStr,
			"view", view,
			"request_id", c.Response().Header().Get("X-Request-ID"),
		)

		// Default limit - use 1 for swipe view, 20 otherwise
		limit := 20
		if view == "swipe" {
			limit = 1
		}
		if limitStr != "" {
			parsedLimit, err := strconv.Atoi(limitStr)
			if err != nil {
				logger.Logger.Error("Invalid limit parameter", "error", err, "limit", limitStr)
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid limit parameter"})
			}
			if parsedLimit > 0 && parsedLimit <= 100 {
				limit = parsedLimit
			} else if parsedLimit > 100 {
				limit = 100
			} else {
				logger.Logger.Error("Invalid limit value", "limit", parsedLimit)
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Limit must be between 1 and 100"})
			}
		}

		// Parse cursor if provided
		var cursor *time.Time
		if cursorStr != "" {
			parsedCursor, err := time.Parse(time.RFC3339, cursorStr)
			if err != nil {
				logger.Logger.Error("Invalid cursor parameter", "error", err, "cursor", cursorStr)
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid cursor format. Use RFC3339 format"})
			}
			cursor = &parsedCursor
		}

		// Add caching headers for cursor-based pagination
		// Use private cache for swipe view (user-specific), public for others
		if view == "swipe" {
			c.Response().Header().Set("Cache-Control", "private, max-age=30") // 30s for swipe view
		} else if cursor == nil {
			c.Response().Header().Set("Cache-Control", "public, max-age=300") // 5 minutes for first page
		} else {
			c.Response().Header().Set("Cache-Control", "public, max-age=900") // 15 minutes for other pages
		}

		logger.Logger.Info("Fetching unread feeds with cursor", "cursor", cursor, "cursor_str", cursorStr, "limit", limit)
		feeds, hasMore, err := container.FetchUnreadFeedsListCursorUsecase.Execute(c.Request().Context(), cursor, limit)
		if err != nil {
			logger.Logger.Error("Error fetching feeds with cursor", "error", err, "cursor", cursor, "limit", limit)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch feeds with cursor"})
		}

		optimizedFeeds := OptimizeFeedsResponse(feeds)

		// Include pagination metadata
		response := map[string]interface{}{
			"data":        optimizedFeeds,
			"has_more":    hasMore,
			"next_cursor": nil,
		}

		var nextCursor string
		if hasMore {
			if derivedCursor, ok := DeriveNextCursorFromFeeds(feeds); ok {
				nextCursor = derivedCursor
				response["next_cursor"] = derivedCursor
			} else {
				logger.Logger.Warn(
					"unable to derive next cursor despite has_more flag",
					"count",
					len(optimizedFeeds),
				)
			}
		}

		logger.Logger.Info(
			"responding to unread feeds cursor",
			"cursor", cursor,
			"limit", limit,
			"count", len(optimizedFeeds),
			"has_more", hasMore,
			"next_cursor", nextCursor,
		)

		return c.JSON(http.StatusOK, response)
	}
}

func RestHandleFetchReadFeedsCursor(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Parse query parameters - 既存パターンと同じ
		limitStr := c.QueryParam("limit")
		cursorStr := c.QueryParam("cursor")

		// Default limit
		limit := 20
		if limitStr != "" {
			parsedLimit, err := strconv.Atoi(limitStr)
			if err != nil {
				logger.Logger.Error("Invalid limit parameter", "error", err, "limit", limitStr)
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid limit parameter"})
			}
			if parsedLimit > 0 && parsedLimit <= 100 {
				limit = parsedLimit
			} else if parsedLimit > 100 {
				limit = 100
			} else {
				logger.Logger.Error("Invalid limit value", "limit", parsedLimit)
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Limit must be between 1 and 100"})
			}
		}

		// Parse cursor if provided - 既存パターンと同じ
		var cursor *time.Time
		if cursorStr != "" {
			parsedCursor, err := time.Parse(time.RFC3339, cursorStr)
			if err != nil {
				logger.Logger.Error("Invalid cursor parameter", "error", err, "cursor", cursorStr)
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid cursor format. Use RFC3339 format"})
			}
			cursor = &parsedCursor
		}

		// キャッシング戦略 - 既読記事の特性を考慮
		if cursor == nil {
			c.Response().Header().Set("Cache-Control", "public, max-age=900") // 15分（初回）
		} else {
			c.Response().Header().Set("Cache-Control", "public, max-age=3600") // 60分（他ページ）
		}

		logger.Logger.Info("Fetching read feeds with cursor", "cursor", cursor, "limit", limit)
		feeds, err := container.FetchReadFeedsListCursorUsecase.Execute(c.Request().Context(), cursor, limit)
		if err != nil {
			logger.Logger.Error("Error fetching read feeds with cursor", "error", err, "cursor", cursor, "limit", limit)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch read feeds with cursor"})
		}

		optimizedFeeds := OptimizeFeedsResponse(feeds)

		// レスポンス構造 - 既存パターンと同じ
		response := map[string]interface{}{
			"data": optimizedFeeds,
		}

		// Add next cursor if there are results
		if len(feeds) > 0 {
			if derivedCursor, ok := DeriveNextCursorFromFeeds(feeds); ok {
				response["next_cursor"] = derivedCursor
			}
		}

		return c.JSON(http.StatusOK, response)
	}
}

func RestHandleFetchFavoriteFeedsCursor(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		limitStr := c.QueryParam("limit")
		cursorStr := c.QueryParam("cursor")

		limit := 20
		if limitStr != "" {
			parsedLimit, err := strconv.Atoi(limitStr)
			if err != nil {
				logger.Logger.Error("Invalid limit parameter", "error", err, "limit", limitStr)
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid limit parameter"})
			}
			if parsedLimit > 0 && parsedLimit <= 100 {
				limit = parsedLimit
			} else if parsedLimit > 100 {
				limit = 100
			} else {
				logger.Logger.Error("Invalid limit value", "limit", parsedLimit)
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Limit must be between 1 and 100"})
			}
		}

		var cursor *time.Time
		if cursorStr != "" {
			parsedCursor, err := time.Parse(time.RFC3339, cursorStr)
			if err != nil {
				logger.Logger.Error("Invalid cursor parameter", "error", err, "cursor", cursorStr)
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid cursor format. Use RFC3339 format"})
			}
			cursor = &parsedCursor
		}

		if cursor == nil {
			c.Response().Header().Set("Cache-Control", "public, max-age=900")
		} else {
			c.Response().Header().Set("Cache-Control", "public, max-age=3600")
		}

		logger.Logger.Info("Fetching favorite feeds with cursor", "cursor", cursor, "limit", limit)
		feeds, err := container.FetchFavoriteFeedsListCursorUsecase.Execute(c.Request().Context(), cursor, limit)
		if err != nil {
			logger.Logger.Error("Error fetching favorite feeds with cursor", "error", err, "cursor", cursor, "limit", limit)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch favorite feeds with cursor"})
		}

		optimizedFeeds := OptimizeFeedsResponse(feeds)
		response := map[string]interface{}{
			"data": optimizedFeeds,
		}

		if len(feeds) > 0 {
			if derivedCursor, ok := DeriveNextCursorFromFeeds(feeds); ok {
				response["next_cursor"] = derivedCursor
			}
		}

		return c.JSON(http.StatusOK, response)
	}
}
