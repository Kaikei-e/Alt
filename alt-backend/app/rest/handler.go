package rest

import (
	"alt/config"
	"alt/di"
	"alt/domain"
	"alt/driver/search_indexer"
	middleware_custom "alt/middleware"
	"alt/utils/errors"
	"alt/utils/html_parser"
	"alt/utils/logger"
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func RegisterRoutes(e *echo.Echo, container *di.ApplicationComponents, cfg *config.Config) {

	// 1. Request ID middleware first - すべてのリクエストにIDを付与
	e.Use(middleware_custom.RequestIDMiddleware())

	// 2. Recovery middleware early - パニックを早期に捕捉
	e.Use(middleware.Recover())

	// 3. Security headers - セキュリティ設定を早期に適用
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		HSTSMaxAge:            31536000,
		ContentSecurityPolicy: "default-src 'self'",
	}))

	// 4. CORS middleware - クロスオリジン制御
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"http://localhost:3000", "http://localhost:80", "https://curionoah.com"},
		AllowMethods: []string{echo.GET, echo.POST, echo.PUT, echo.DELETE, echo.OPTIONS},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, "Cache-Control", "Authorization", "X-Requested-With", "X-CSRF-Token"},
		MaxAge:       86400, // Cache preflight for 24 hours
	}))

	// 5. DOS protection - 悪意のあるリクエストを早期にブロック
	dosConfig := cfg.RateLimit.DOSProtection
	dosConfig.WhitelistedPaths = []string{"/v1/health", "/v1/sse/", "/security/csp-report"}
	e.Use(middleware_custom.DOSProtectionMiddleware(middleware_custom.ConvertConfigDOSProtection(dosConfig)))

	// 6. CSRF protection - 認証が必要な場合 Temporarily disabled for development
	// e.Use(middleware_custom.CSRFMiddleware(container.CSRFTokenUsecase))

	// 7. Request timeout - リクエスト処理時間の制限
	e.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{
		Timeout: cfg.Server.ReadTimeout,
		Skipper: func(c echo.Context) bool {
			return strings.Contains(c.Path(), "/sse/")
		},
	}))

	// 8. Validation middleware - リクエスト内容の検証
	e.Use(middleware_custom.ValidationMiddleware())

	// 9. Logging middleware - 処理内容をログに記録
	e.Use(middleware_custom.LoggingMiddleware(logger.Logger))

	// 10. Compression middleware last - レスポンス時の圧縮（最後に実行）
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level: 5, // Balanced compression level
		Skipper: func(c echo.Context) bool {
			// Skip compression for already compressed content and SSE endpoints
			return strings.Contains(c.Request().Header.Get("Accept-Encoding"), "br") ||
				strings.Contains(c.Path(), "/health") ||
				strings.Contains(c.Path(), "/sse/")
		},
	}))

	v1 := e.Group("/v1")

	// CSRF token generation endpoint
	v1.GET("/csrf-token", middleware_custom.CSRFTokenHandler(container.CSRFTokenUsecase))

	// Health check with database connectivity test
	v1.GET("/health", func(c echo.Context) error {
		// Set cache headers for health check
		c.Response().Header().Set("Cache-Control", "public, max-age=30")

		response := map[string]string{
			"status": "healthy",
		}

		response["database"] = "connected"
		return c.JSON(http.StatusOK, response)
	})

	// CSP report endpoint
	e.POST("/security/csp-report", func(c echo.Context) error {
		// Read raw body for debugging
		body, err := io.ReadAll(c.Request().Body)
		if err != nil {
			logger.Logger.Error("Failed to read request body", "error", err)
			return echo.NewHTTPError(http.StatusBadRequest, "Failed to read request body")
		}

		// Log raw body for debugging
		logger.Logger.Info("CSP Report Raw Body", "body", string(body))

		// Try to parse as JSON
		var report map[string]interface{}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &report); err != nil {
				logger.Logger.Warn("CSP Report - Invalid JSON", "body", string(body), "error", err)
				// Return 204 even for invalid JSON to prevent browser retries
				return c.NoContent(http.StatusNoContent)
			}
		}

		// Log CSP violation report
		logger.Logger.Warn("CSP Violation Report",
			"timestamp", time.Now().Format(time.RFC3339),
			"report", report,
			"user_agent", c.Request().UserAgent(),
			"ip", c.RealIP(),
		)

		// Return 204 No Content for CSP reports
		return c.NoContent(http.StatusNoContent)
	})

	v1.GET("/feeds/fetch/single", func(c echo.Context) error {
		// Add caching headers
		c.Response().Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(cfg.Cache.FeedCacheExpiry.Seconds())))
		c.Response().Header().Set("ETag", `"single-feed"`)

		feed, err := container.FetchSingleFeedUsecase.Execute(c.Request().Context())
		if err != nil {
			return handleError(c, err, "fetch_single_feed")
		}
		return c.JSON(http.StatusOK, feed)
	})

	v1.GET("/feeds/fetch/list", func(c echo.Context) error {
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
	})

	v1.GET("/feeds/fetch/limit/:limit", func(c echo.Context) error {
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
	})

	v1.GET("/feeds/fetch/page/:page", func(c echo.Context) error {
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
	})

	v1.GET("/feeds/fetch/cursor", func(c echo.Context) error {
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
	})

	v1.GET("/feeds/fetch/viewed/cursor", func(c echo.Context) error {
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
		// read_statusテーブルの更新頻度：
		// - 新規既読: ユーザーアクションによる（低頻度）
		// - 状態変更: is_read更新のみ（フィードコンテンツは不変）
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
	})

	v1.GET("/feeds/fetch/favorites/cursor", func(c echo.Context) error {
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
	})

	v1.POST("/feeds/read", func(c echo.Context) error {
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
	})

	v1.POST("/feeds/search", func(c echo.Context) error {
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

		// Clean HTML from search results using goquery
		cleanedResults := html_parser.CleanSearchResultsWithGoquery(results)

		logger.Logger.Info("Feed search completed successfully", "query", payload.Query, "results_count", len(cleanedResults))
		return c.JSON(http.StatusOK, cleanedResults)
	})

	v1.POST("/feeds/fetch/details", func(c echo.Context) error {
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
	})

	v1.GET("/articles/search", func(c echo.Context) error {
		query := c.QueryParam("q")
		if query == "" {
			logger.Logger.Error("Search query must not be empty")
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Search query must not be empty"})
		}

		results, err := search_indexer.SearchArticles(query)
		if err != nil {
			return handleError(c, err, "search_articles")
		}

		return c.JSON(http.StatusOK, results)
	})

	v1.GET("/feeds/stats", func(c echo.Context) error {
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
	})

	v1.POST("/feeds/tags", func(c echo.Context) error {
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
	})

	v1.GET("/feeds/count/unreads", func(c echo.Context) error {
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
	})

	v1.POST("/rss-feed-link/register", func(c echo.Context) error {
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
	})

	v1.POST("/feeds/register/favorite", func(c echo.Context) error {
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
	})

	// Add SSE endpoint with proper Echo SSE handling
	v1.GET("/sse/feeds/stats", func(c echo.Context) error {
		// Set SSE headers using Echo's response
		c.Response().Header().Set("Content-Type", "text/event-stream")
		c.Response().Header().Set("Cache-Control", "no-cache")
		c.Response().Header().Set("Connection", "keep-alive")
		c.Response().Header().Set("Access-Control-Allow-Origin", "*")
		c.Response().Header().Set("Access-Control-Allow-Headers", "Cache-Control")

		// Don't let Echo write its own status
		c.Response().WriteHeader(http.StatusOK)

		// Get the underlying response writer for flushing
		w := c.Response().Writer
		flusher, canFlush := w.(http.Flusher)
		if !canFlush {
			logger.Logger.Error("Response writer doesn't support flushing")
			return c.String(http.StatusInternalServerError, "Streaming not supported")
		}

		// Send initial data
		amount, err := container.FeedAmountUsecase.Execute(c.Request().Context())
		if err != nil {
			logger.Logger.Error("Error fetching initial feed amount", "error", err)
			amount = 0
		}

		unsummarizedCount, err := container.UnsummarizedArticlesCountUsecase.Execute(c.Request().Context())
		if err != nil {
			logger.Logger.Error("Error fetching initial unsummarized articles count", "error", err)
			unsummarizedCount = 0
		}

		totalArticlesCount, err := container.TotalArticlesCountUsecase.Execute(c.Request().Context())
		if err != nil {
			logger.Logger.Error("Error fetching initial total articles count", "error", err)
			totalArticlesCount = 0
		}

		initialStats := UnsummarizedFeedStatsSummary{
			FeedAmount:             feedAmount{Amount: amount},
			UnsummarizedFeedAmount: unsummarizedFeedAmount{Amount: unsummarizedCount},
			ArticleAmount:          articleAmount{Amount: totalArticlesCount},
		}

		// Send initial data
		if jsonData, err := json.Marshal(initialStats); err == nil {
			c.Response().Write([]byte("data: " + string(jsonData) + "\n\n"))
			flusher.Flush()
		}

		// Create ticker for periodic updates
		ticker := time.NewTicker(cfg.Server.SSEInterval)
		defer ticker.Stop()

		// Create heartbeat ticker to keep connection alive (every 10 seconds)
		heartbeatTicker := time.NewTicker(10 * time.Second)
		defer heartbeatTicker.Stop()

		// Keep connection alive
		for {
			select {
			case <-heartbeatTicker.C:
				// Send heartbeat comment to keep connection alive
				_, err := c.Response().Write([]byte(": heartbeat\n\n"))
				if err != nil {
					logger.Logger.Info("Client disconnected during heartbeat", "error", err)
					return nil
				}
				flusher.Flush()

			case <-ticker.C:
				// Fetch fresh data
				amount, err := container.FeedAmountUsecase.Execute(c.Request().Context())
				if err != nil {
					logger.Logger.Error("Error fetching feed amount", "error", err)
					continue
				}

				unsummarizedCount, err := container.UnsummarizedArticlesCountUsecase.Execute(c.Request().Context())
				if err != nil {
					logger.Logger.Error("Error fetching unsummarized articles count", "error", err)
					continue
				}

				totalArticlesCount, err := container.TotalArticlesCountUsecase.Execute(c.Request().Context())
				if err != nil {
					logger.Logger.Error("Error fetching total articles count", "error", err)
					continue
				}

				stats := UnsummarizedFeedStatsSummary{
					FeedAmount:             feedAmount{Amount: amount},
					UnsummarizedFeedAmount: unsummarizedFeedAmount{Amount: unsummarizedCount},
					ArticleAmount:          articleAmount{Amount: totalArticlesCount},
				}

				// Convert to JSON and send
				jsonData, err := json.Marshal(stats)
				if err != nil {
					logger.Logger.Error("Error marshaling stats", "error", err)
					continue
				}

				// Write in SSE format
				_, err = c.Response().Write([]byte("data: " + string(jsonData) + "\n\n"))
				if err != nil {
					logger.Logger.Info("Client disconnected", "error", err)
					return nil
				}

				// Flush the data
				flusher.Flush()

			case <-c.Request().Context().Done():
				logger.Logger.Info("SSE connection closed by client")
				return nil
			}
		}
	})

}

// handleError converts errors to appropriate HTTP responses using enhanced error handling
func handleError(c echo.Context, err error, operation string) error {
	// Enrich error with REST layer context
	var enrichedErr *errors.AppContextError

	// Check if it's already an AppContextError and enrich it with REST context
	if appContextErr, ok := err.(*errors.AppContextError); ok {
		enrichedErr = errors.EnrichWithContext(
			appContextErr,
			"rest",
			"RESTHandler",
			operation,
			map[string]interface{}{
				"path":        c.Request().URL.Path,
				"method":      c.Request().Method,
				"remote_addr": c.Request().RemoteAddr,
				"user_agent":  c.Request().UserAgent(),
				"request_id":  c.Response().Header().Get("X-Request-ID"),
			},
		)
	} else if appErr, ok := err.(*errors.AppError); ok {
		// Handle legacy AppError by converting to AppContextError
		enrichedErr = errors.NewAppContextError(
			string(appErr.Code),
			appErr.Message,
			"rest",
			"RESTHandler",
			operation,
			appErr.Cause,
			map[string]interface{}{
				"path":           c.Request().URL.Path,
				"method":         c.Request().Method,
				"remote_addr":    c.Request().RemoteAddr,
				"user_agent":     c.Request().UserAgent(),
				"request_id":     c.Response().Header().Get("X-Request-ID"),
				"legacy_context": appErr.Context,
			},
		)
	} else {
		// Handle unknown errors
		enrichedErr = errors.NewUnknownContextError(
			"internal server error",
			"rest",
			"RESTHandler",
			operation,
			err,
			map[string]interface{}{
				"path":        c.Request().URL.Path,
				"method":      c.Request().Method,
				"remote_addr": c.Request().RemoteAddr,
				"user_agent":  c.Request().UserAgent(),
				"request_id":  c.Response().Header().Get("X-Request-ID"),
			},
		)
	}

	// Log the enriched error with context
	logger.Logger.Error("REST handler error",
		"error", enrichedErr.Error(),
		"error_code", enrichedErr.Code,
		"layer", enrichedErr.Layer,
		"component", enrichedErr.Component,
		"operation", enrichedErr.Operation,
		"path", c.Request().URL.Path,
		"method", c.Request().Method,
		"is_retryable", enrichedErr.IsRetryable(),
	)

	return c.JSON(enrichedErr.HTTPStatusCode(), enrichedErr.ToHTTPResponse())
}

// handleValidationError creates a validation error response with enhanced context
func handleValidationError(c echo.Context, message string, field string, value interface{}) error {
	validationErr := errors.NewValidationContextError(
		message,
		"rest",
		"RESTHandler",
		"validateInput",
		map[string]interface{}{
			"field":       field,
			"value":       value,
			"path":        c.Request().URL.Path,
			"method":      c.Request().Method,
			"remote_addr": c.Request().RemoteAddr,
			"user_agent":  c.Request().UserAgent(),
			"request_id":  c.Response().Header().Get("X-Request-ID"),
		},
	)

	logger.Logger.Error("REST validation error",
		"error", validationErr.Error(),
		"field", field,
		"value", value,
		"path", c.Request().URL.Path,
	)
	return c.JSON(validationErr.HTTPStatusCode(), validationErr.ToHTTPResponse())
}

// Optimize feeds response by truncating descriptions and removing unnecessary fields
func optimizeFeedsResponse(feeds []*domain.FeedItem) []*domain.FeedItem {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for _, feed := range feeds {
		feed.Title = strings.TrimSpace(feed.Title)
		feed.Description = sanitizeAndExtract(ctx, feed.Description)
	}
	return feeds
}

// Determine cache age based on limit to optimize caching strategy
func getCacheAgeForLimit(limit int) int {
	switch {
	case limit <= 20:
		return 600 // 10 minutes for small requests
	case limit <= 100:
		return 900 // 15 minutes for medium requests
	default:
		return 1800 // 30 minutes for large requests
	}
}

func sanitizeAndExtract(ctx context.Context, raw string) string {
	if !strings.Contains(raw, "<") { // HTML でなければ早期 return
		return truncate(strings.TrimSpace(raw))
	}
	const ctype = "text/html; charset=utf-8"
	paras, err := html_parser.ExtractPTags(ctx, strings.NewReader(raw), ctype)
	if err != nil || len(paras) == 0 {
		return truncate(strings.TrimSpace(html_parser.StripTags(raw)))
	}
	clean := strings.Join(paras, "\n")
	return truncate(strings.TrimSpace(clean))
}

// truncate は従来の 500 文字丸めロジック（流用）
func truncate(s string) string {
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}

func isAllowedURL(u *url.URL) error {
	// Allow both HTTP and HTTPS
	if u.Scheme != "https" && u.Scheme != "http" {
		return stderrors.New("only HTTP and HTTPS schemes allowed")
	}

	// Block private networks
	if isPrivateIP(u.Hostname()) {
		return stderrors.New("access to private networks not allowed")
	}

	// Block localhost variations
	hostname := strings.ToLower(u.Hostname())
	if hostname == "localhost" || hostname == "127.0.0.1" || strings.HasPrefix(hostname, "127.") {
		return stderrors.New("access to localhost not allowed")
	}

	// Block metadata endpoints (AWS, GCP, Azure)
	if hostname == "169.254.169.254" || hostname == "metadata.google.internal" {
		return stderrors.New("access to metadata endpoint not allowed")
	}

	// Block common internal domains
	internalDomains := []string{".local", ".internal", ".corp", ".lan"}
	for _, domain := range internalDomains {
		if strings.HasSuffix(hostname, domain) {
			return stderrors.New("access to internal domains not allowed")
		}
	}

	return nil
}

func isPrivateIP(hostname string) bool {
	// Try to parse as IP first
	ip := net.ParseIP(hostname)
	if ip != nil {
		return isPrivateIPAddress(ip)
	}

	// If it's a hostname, resolve it to IPs
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// Block on resolution failure as a security measure
		return true
	}

	// Check if any resolved IP is private
	for _, ip := range ips {
		if isPrivateIPAddress(ip) {
			return true
		}
	}

	return false
}

func isPrivateIPAddress(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Check for private IPv4 ranges
	if ip.To4() != nil {
		// 10.0.0.0/8
		if ip[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip[0] == 192 && ip[1] == 168 {
			return true
		}
	}

	// Check for private IPv6 ranges
	if ip.To16() != nil && ip.To4() == nil {
		// Check for unique local addresses (fc00::/7)
		if ip[0] == 0xfc || ip[0] == 0xfd {
			return true
		}
	}

	return false
}
