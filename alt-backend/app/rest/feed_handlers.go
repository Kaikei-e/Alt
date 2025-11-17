package rest

import (
	"alt/config"
	"alt/di"
	middleware_custom "alt/middleware"
	"alt/utils/errors"
	"alt/utils/logger"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/labstack/echo/v4"
)

func registerFeedRoutes(v1 *echo.Group, container *di.ApplicationComponents, cfg *config.Config) {
	// 認証ミドルウェアの初期化（ヘッダベースの認証）
	authMiddleware := middleware_custom.NewAuthMiddleware(logger.Logger)

	// TODO.md案A: privateグループ化で認証を適用
	// v1にまとめて適用する代わりに、feedsグループに認証ミドルウェアを適用
	feeds := v1.Group("/feeds", authMiddleware.RequireAuth())

	// Private endpoints (authentication required)
	feeds.GET("/fetch/single", handleFetchSingleFeed(container, cfg))
	feeds.GET("/fetch/list", handleFetchFeedsList(container, cfg))
	feeds.GET("/fetch/limit/:limit", handleFetchFeedsLimit(container, cfg))
	feeds.GET("/fetch/page/:page", handleFetchFeedsPage(container))

	// User-specific endpoints (authentication required) - 認証必須パス
	feeds.GET("/count/unreads", handleUnreadCount(container))
	feeds.GET("/fetch/cursor", handleFetchUnreadFeedsCursor(container))
	feeds.GET("/fetch/viewed/cursor", handleFetchReadFeedsCursor(container))
	feeds.GET("/fetch/favorites/cursor", handleFetchFavoriteFeedsCursor(container))
	feeds.POST("/read", handleMarkFeedAsRead(container))
	feeds.POST("/register/favorite", handleRegisterFavoriteFeed(container))

	// Authentication needed endpoints (for personalized results)
	feeds.POST("/search", handleSearchFeeds(container))
	feeds.POST("/fetch/details", handleFetchFeedDetails(container))
	feeds.GET("/stats", handleFeedStats(container, cfg))
	feeds.POST("/tags", handleFetchFeedTags(container))
	feeds.POST("/fetch/summary/provided", handleFetchInoreaderSummary(container))

	// Article summarization endpoint
	feeds.POST("/summarize", handleSummarizeFeed(container, cfg))

	// RSS feed registration (require auth) - 認証ミドルウェア付きでグループ作成
	rss := v1.Group("/rss-feed-link", authMiddleware.RequireAuth())
	rss.POST("/register", handleRegisterRSSFeed(container))
	rss.GET("/list", handleListRSSFeedLinks(container))
	rss.DELETE("/:id", handleDeleteRSSFeedLink(container))
}

func handleFetchSingleFeed(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Add caching headers
		c.Response().Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(cfg.Cache.FeedCacheExpiry.Seconds())))
		c.Response().Header().Set("ETag", `"single-feed"`)

		feed, err := container.FetchSingleFeedUsecase.Execute(c.Request().Context())
		if err != nil {
			return handleError(c, err, "fetch_single_feed")
		}
		return c.JSON(http.StatusOK, feed)
	}
}

func handleFetchFeedsList(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Add caching headers for feed list
		c.Response().Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(cfg.Cache.SearchCacheExpiry.Seconds())))
		c.Response().Header().Set("ETag", `"feeds-list"`)

		feeds, err := container.FetchFeedsListUsecase.Execute(c.Request().Context())
		if err != nil {
			return handleError(c, err, "fetch_feeds_list")
		}

		// Optimize response size
		optimizedFeeds := optimizeFeedsResponse(feeds)
		return c.JSON(http.StatusOK, optimizedFeeds)
	}
}

func handleFetchFeedsLimit(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		limit, err := strconv.Atoi(c.Param("limit"))
		if err != nil {
			return handleValidationError(c, "Invalid limit parameter", "limit", c.Param("limit"))
		}

		// Validate limit to prevent excessive resource usage
		if limit > 1000 {
			limit = 1000
		}

		// Add caching headers based on limit
		cacheAge := getCacheAgeForLimit(limit)
		c.Response().Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(cacheAge))
		c.Response().Header().Set("ETag", `"feeds-limit-`+strconv.Itoa(limit)+`"`)

		feeds, err := container.FetchFeedsListUsecase.ExecuteLimit(c.Request().Context(), limit)
		if err != nil {
			return handleError(c, err, "fetch_feeds_limit")
		}

		optimizedFeeds := optimizeFeedsResponse(feeds)
		return c.JSON(http.StatusOK, optimizedFeeds)
	}
}

func handleFetchFeedsPage(container *di.ApplicationComponents) echo.HandlerFunc {
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

		optimizedFeeds := optimizeFeedsResponse(feeds)
		return c.JSON(http.StatusOK, optimizedFeeds)
	}
}

func handleFetchUnreadFeedsCursor(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Parse query parameters
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
		if cursor == nil {
			c.Response().Header().Set("Cache-Control", "public, max-age=300") // 5 minutes for first page
		} else {
			c.Response().Header().Set("Cache-Control", "public, max-age=900") // 15 minutes for other pages
		}

		logger.Logger.Info("Fetching unread feeds with cursor", "cursor", cursor, "limit", limit)
		feeds, err := container.FetchUnreadFeedsListCursorUsecase.Execute(c.Request().Context(), cursor, limit)
		if err != nil {
			logger.Logger.Error("Error fetching feeds with cursor", "error", err, "cursor", cursor, "limit", limit)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch feeds with cursor"})
		}

		optimizedFeeds := optimizeFeedsResponse(feeds)

		// Include next cursor in response for pagination
		response := map[string]interface{}{
			"data": optimizedFeeds,
		}

		// Add next cursor if there are results
		if len(optimizedFeeds) > 0 {
			lastFeed := optimizedFeeds[len(optimizedFeeds)-1]
			// Parse the published time to use as next cursor
			if lastPublished, err := time.Parse(time.RFC3339, lastFeed.Published); err == nil {
				response["next_cursor"] = lastPublished.Format(time.RFC3339)
			}
		}

		return c.JSON(http.StatusOK, response)
	}
}

func handleFetchReadFeedsCursor(container *di.ApplicationComponents) echo.HandlerFunc {
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

		optimizedFeeds := optimizeFeedsResponse(feeds)

		// レスポンス構造 - 既存パターンと同じ
		response := map[string]interface{}{
			"data": optimizedFeeds,
		}

		// Add next cursor if there are results
		if len(optimizedFeeds) > 0 {
			lastFeed := optimizedFeeds[len(optimizedFeeds)-1]
			if lastPublished, err := time.Parse(time.RFC3339, lastFeed.Published); err == nil {
				response["next_cursor"] = lastPublished.Format(time.RFC3339)
			}
		}

		return c.JSON(http.StatusOK, response)
	}
}

func handleFetchFavoriteFeedsCursor(container *di.ApplicationComponents) echo.HandlerFunc {
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

		optimizedFeeds := optimizeFeedsResponse(feeds)
		response := map[string]interface{}{
			"data": optimizedFeeds,
		}

		if len(optimizedFeeds) > 0 {
			lastFeed := optimizedFeeds[len(optimizedFeeds)-1]
			if lastPublished, err := time.Parse(time.RFC3339, lastFeed.Published); err == nil {
				response["next_cursor"] = lastPublished.Format(time.RFC3339)
			}
		}

		return c.JSON(http.StatusOK, response)
	}
}

func handleMarkFeedAsRead(container *di.ApplicationComponents) echo.HandlerFunc {
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

func handleSearchFeeds(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		var payload FeedSearchPayload
		err := c.Bind(&payload)
		if err != nil {
			logger.Logger.Error("Error binding search payload", "error", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}

		if payload.Query == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Search query must not be empty"})
		}

		logger.Logger.Info("Executing feed search", "query", payload.Query)
		results, err := container.FeedSearchUsecase.Execute(c.Request().Context(), payload.Query)
		if err != nil {
			logger.Logger.Error("Error executing feed search", "error", err, "query", payload.Query)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		logger.Logger.Info("Feed search completed successfully", "query", payload.Query, "results_count", len(results))
		return c.JSON(http.StatusOK, results)
	}
}

func handleFetchFeedDetails(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		var payload FeedUrlPayload
		err := c.Bind(&payload)
		if err != nil {
			logger.Logger.Error("Error binding feed URL", "error", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}

		feedURLParsed, err := url.Parse(payload.FeedURL)
		if err != nil {
			logger.Logger.Error("Error parsing feed URL", "error", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}

		err = isAllowedURL(feedURLParsed)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}

		details, err := container.FeedsSummaryUsecase.Execute(c.Request().Context(), feedURLParsed)
		if err != nil {
			logger.Logger.Error("Error fetching feed details", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, details)
	}
}

func handleFeedStats(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Add caching headers for stats
		c.Response().Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(cfg.Cache.FeedCacheExpiry.Seconds())))
		c.Response().Header().Set("ETag", `"feeds-stats"`)

		// Fetch feed amount
		feedCount, err := container.FeedAmountUsecase.Execute(c.Request().Context())
		if err != nil {
			logger.Logger.Error("Error fetching feed amount", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch feed statistics"})
		}

		// Fetch summarized articles count
		summarizedCount, err := container.SummarizedArticlesCountUsecase.Execute(c.Request().Context())
		if err != nil {
			logger.Logger.Error("Error fetching summarized articles count", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch feed statistics"})
		}

		// Create response in expected format
		stats := FeedStatsSummary{
			FeedAmount:           feedAmount{Amount: feedCount},
			SummarizedFeedAmount: summarizedFeedAmount{Amount: summarizedCount},
		}

		logger.Logger.Info("Feed stats retrieved successfully",
			"feed_count", feedCount,
			"summarized_count", summarizedCount)

		return c.JSON(http.StatusOK, stats)
	}
}

func handleFetchFeedTags(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Parse request body
		var req FeedTagsPayload
		if err := c.Bind(&req); err != nil {
			logger.Logger.Error("Failed to bind request body", "error", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request format"})
		}

		// Parse and validate the article url
		parsedArticleURL, err := url.Parse(req.FeedURL)
		if err != nil {
			logger.Logger.Error("Invalid the url format", "error", err.Error(), "article_url", req.FeedURL)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid article url format"})
		}

		// Apply URL security validation (same as other endpoints)
		err = isAllowedURL(parsedArticleURL)
		if err != nil {
			logger.Logger.Error("Article URL not allowed", "error", err, "article_url", req.FeedURL)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Article URL not allowed for security reasons"})
		}

		// Set default limit if not provided
		limit := 20 // Default limit
		if req.Limit > 0 {
			limit = req.Limit
		}

		// Parse cursor if provided (same pattern as existing cursor endpoints)
		var cursor *time.Time
		if req.Cursor != "" {
			parsedCursor, err := time.Parse(time.RFC3339, req.Cursor)
			if err != nil {
				logger.Logger.Error("Invalid cursor parameter", "error", err, "cursor", req.Cursor)
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid cursor format. Use RFC3339 format"})
			}
			cursor = &parsedCursor
		}

		// Add caching headers (tags update infrequently)
		c.Response().Header().Set("Cache-Control", "public, max-age=3600") // 1 hour

		logger.Logger.Info("Fetching feed tags", "feed_url", req.FeedURL, "cursor", cursor, "limit", limit)
		tags, err := container.FetchFeedTagsUsecase.Execute(c.Request().Context(), req.FeedURL, cursor, limit)
		if err != nil {
			logger.Logger.Error("Error fetching feed tags", "error", err, "feed_url", req.FeedURL, "limit", limit)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch feed tags"})
		}

		// Convert domain tags to response format
		tagResponses := make([]FeedTagResponse, len(tags))
		for i, tag := range tags {
			tagResponses[i] = FeedTagResponse{
				ID:        tag.ID,
				Name:      tag.TagName,
				CreatedAt: tag.CreatedAt.Format(time.RFC3339),
			}
		}

		// Create response with next_cursor for pagination (same pattern as other cursor endpoints)
		response := map[string]interface{}{
			"feed_url": req.FeedURL,
			"tags":     tagResponses,
		}

		// Add next cursor if there are results
		if len(tags) > 0 {
			lastTag := tags[len(tags)-1]
			response["next_cursor"] = lastTag.CreatedAt.Format(time.RFC3339)
		}

		logger.Logger.Info("Feed tags retrieved successfully", "feed_url", req.FeedURL, "count", len(tags))
		return c.JSON(http.StatusOK, response)
	}
}

func handleUnreadCount(container *di.ApplicationComponents) echo.HandlerFunc {
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

func handleRegisterRSSFeed(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		var rssFeedLink RssFeedLink
		err := c.Bind(&rssFeedLink)
		if err != nil {
			return handleValidationError(c, "Invalid request format", "body", "malformed JSON")
		}

		if strings.TrimSpace(rssFeedLink.URL) == "" {
			return handleValidationError(c, "URL is required and cannot be empty", "url", rssFeedLink.URL)
		}

		// Parse and validate URL for SSRF protection
		parsedURL, err := url.Parse(rssFeedLink.URL)
		if err != nil {
			return handleValidationError(c, "Invalid URL format", "url", rssFeedLink.URL)
		}

		// Apply SSRF protection
		err = isAllowedURL(parsedURL)
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
			return handleError(c, err, "register_feed")
		}

		// Invalidate cache after registration
		c.Response().Header().Set("Cache-Control", "no-cache")
		return c.JSON(http.StatusOK, map[string]string{"message": "RSS feed link registered"})
	}
}

func handleRegisterFavoriteFeed(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		var payload RssFeedLink
		if err := c.Bind(&payload); err != nil {
			return handleValidationError(c, "Invalid request format", "body", "malformed JSON")
		}

		if strings.TrimSpace(payload.URL) == "" {
			return handleValidationError(c, "URL is required and cannot be empty", "url", payload.URL)
		}

		parsedURL, err := url.Parse(payload.URL)
		if err != nil {
			return handleValidationError(c, "Invalid URL format", "url", payload.URL)
		}

		if err = isAllowedURL(parsedURL); err != nil {
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
			return handleError(c, err, "register_favorite_feed")
		}

		c.Response().Header().Set("Cache-Control", "no-cache")
		return c.JSON(http.StatusOK, map[string]string{"message": "favorite feed registered"})
	}
}

func handleListRSSFeedLinks(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		links, err := container.ListFeedLinksUsecase.Execute(c.Request().Context())
		if err != nil {
			return handleError(c, err, "list_feed_links")
		}
		return c.JSON(http.StatusOK, links)
	}
}

func handleDeleteRSSFeedLink(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		idParam := c.Param("id")
		linkID, err := uuid.Parse(idParam)
		if err != nil {
			return handleValidationError(c, "Invalid feed link ID", "id", idParam)
		}

		if err := container.DeleteFeedLinkUsecase.Execute(c.Request().Context(), linkID); err != nil {
			return handleError(c, err, "delete_feed_link")
		}

		c.Response().Header().Set("Cache-Control", "no-cache")
		return c.JSON(http.StatusOK, map[string]string{"message": "Feed link deleted"})
	}
}

func handleFetchInoreaderSummary(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req FeedSummaryRequest
		if err := c.Bind(&req); err != nil {
			return handleValidationError(c, "Invalid request format", "body", "malformed JSON")
		}

		// Manual validation - check if feed_urls is provided and within limits
		if len(req.FeedURLs) == 0 {
			return handleValidationError(c, "feed_urls is required and cannot be empty", "feed_urls", req.FeedURLs)
		}
		if len(req.FeedURLs) > 50 {
			return handleValidationError(c, "Maximum 50 URLs allowed per request", "feed_urls", len(req.FeedURLs))
		}

		// SSRF protection for all URLs
		for _, feedURL := range req.FeedURLs {
			parsedURL, err := url.Parse(feedURL)
			if err != nil {
				return handleValidationError(c, "Invalid URL format", "feed_urls", feedURL)
			}

			if err := isAllowedURL(parsedURL); err != nil {
				securityErr := errors.NewValidationContextError(
					"URL not allowed for security reasons",
					"rest",
					"RESTHandler",
					"fetch_inoreader_summary",
					map[string]interface{}{
						"url":         feedURL,
						"reason":      err.Error(),
						"path":        c.Request().URL.Path,
						"method":      c.Request().Method,
						"remote_addr": c.Request().RemoteAddr,
						"request_id":  c.Response().Header().Get("X-Request-ID"),
					},
				)
				logger.Logger.Error("URL validation failed", "error", securityErr.Error(), "url", feedURL)
				return c.JSON(securityErr.HTTPStatusCode(), securityErr.ToHTTPResponse())
			}
		}

		// Execute usecase
		summaries, err := container.FetchInoreaderSummaryUsecase.Execute(c.Request().Context(), req.FeedURLs)
		if err != nil {
			return handleError(c, err, "fetch_inoreader_summary")
		}

		// Convert domain entities to response DTOs
		responses := make([]InoreaderSummaryResponse, 0, len(summaries))
		for _, summary := range summaries {
			authorStr := ""
			if summary.Author != nil {
				authorStr = *summary.Author
			}

			resp := InoreaderSummaryResponse{
				ArticleURL:  summary.ArticleURL,
				Title:       summary.Title,
				Author:      authorStr,
				Content:     summary.Content,
				ContentType: summary.ContentType,
				PublishedAt: summary.PublishedAt.Format(time.RFC3339),
				FetchedAt:   summary.FetchedAt.Format(time.RFC3339),
				InoreaderID: summary.InoreaderID,
			}
			responses = append(responses, resp)
		}

		// Build final response
		finalResponse := FeedSummaryProvidedResponse{
			MatchedArticles: responses,
			TotalMatched:    len(responses),
			RequestedCount:  len(req.FeedURLs),
		}

		// Set caching headers (15 minutes as per XPLAN11.md)
		c.Response().Header().Set("Cache-Control", "public, max-age=900")
		c.Response().Header().Set("Content-Type", "application/json")

		return c.JSON(http.StatusOK, finalResponse)
	}
}

// handleSummarizeFeed handles article summarization requests by proxying to pre-processor
func handleSummarizeFeed(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Parse request
		var req struct {
			FeedURL string `json:"feed_url" validate:"required"`
		}

		if err := c.Bind(&req); err != nil {
			logger.Logger.Error("Failed to bind summarize request", "error", err)
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid request format")
		}

		// Validate feed URL
		if req.FeedURL == "" {
			logger.Logger.Warn("Empty feed_url provided for summarization")
			return echo.NewHTTPError(http.StatusBadRequest, "feed_url is required")
		}

		// Validate URL format
		if _, err := url.Parse(req.FeedURL); err != nil {
			logger.Logger.Error("Invalid feed_url format", "error", err, "url", req.FeedURL)
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid feed_url format")
		}

		logger.Logger.Info("Processing summarization request", "feed_url", req.FeedURL)

		// Fetch article content (you might need to fetch from DB or URL)
		articleContent, articleID, articleTitle, err := fetchArticleContent(c.Request().Context(), req.FeedURL, container)
		if err != nil {
			logger.Logger.Error("Failed to fetch article content", "error", err, "url", req.FeedURL)
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch article content")
		}

		// Step 1: Try to fetch existing summary from database
		var summary string
		existingSummary, err := container.AltDBRepository.FetchArticleSummaryByArticleID(c.Request().Context(), articleID)
		if err == nil && existingSummary != nil && existingSummary.Summary != "" {
			logger.Logger.Info("Found existing summary in database", "article_id", articleID, "feed_url", req.FeedURL)
			summary = existingSummary.Summary
		} else {
			// Step 2: Generate new summary if not found in database
			logger.Logger.Info("No existing summary found, generating new summary", "article_id", articleID, "feed_url", req.FeedURL)

			summary, err = callPreProcessorSummarize(c.Request().Context(), articleContent, articleID, articleTitle, cfg.PreProcessor.URL)
			if err != nil {
				logger.Logger.Error("Failed to summarize article", "error", err, "url", req.FeedURL)
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to generate summary")
			}

			// Step 3: Save the generated summary to database
			if err := container.AltDBRepository.SaveArticleSummary(c.Request().Context(), articleID, articleTitle, summary); err != nil {
				logger.Logger.Error("Failed to save article summary to database", "error", err, "article_id", articleID, "feed_url", req.FeedURL)
				// Continue even if save fails - we still have the summary to return
			} else {
				logger.Logger.Info("Article summary saved to database", "article_id", articleID, "feed_url", req.FeedURL)
			}
		}

		logger.Logger.Info("Article summarized successfully", "feed_url", req.FeedURL, "from_cache", existingSummary != nil)

		// Step 4: Return response
		response := map[string]interface{}{
			"success":    true,
			"summary":    summary,
			"article_id": articleID,
			"feed_url":   req.FeedURL,
		}

		return c.JSON(http.StatusOK, response)
	}
}
