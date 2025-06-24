package rest

import (
	"alt/di"
	"alt/domain"
	"alt/driver/search_indexer"
	"alt/utils/html_parser"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func RegisterRoutes(e *echo.Echo, container *di.ApplicationComponents) {

	// Add performance middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Add compression middleware for better performance
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level: 5, // Balanced compression level
		Skipper: func(c echo.Context) bool {
			// Skip compression for already compressed content and SSE endpoints
			return strings.Contains(c.Request().Header.Get("Accept-Encoding"), "br") ||
				strings.Contains(c.Path(), "/health") ||
				strings.Contains(c.Path(), "/sse/")
		},
	}))

	// Add request timeout middleware (excluding SSE endpoints)
	e.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{
		Timeout: 30 * time.Second,
		Skipper: func(c echo.Context) bool {
			return strings.Contains(c.Path(), "/sse/")
		},
	}))

	// Add rate limiting middleware (skip for SSE endpoints)
	e.Use(middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: middleware.NewRateLimiterMemoryStore(100), // 100 requests per second
		Skipper: func(c echo.Context) bool {
			return strings.Contains(c.Path(), "/sse/")
		},
	}))

	// Add security headers
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		HSTSMaxAge:            31536000,
		ContentSecurityPolicy: "default-src 'self'",
	}))

	// Add CORS middleware with optimized settings
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"http://localhost:3000", "http://localhost:80", "https://curionoah.com", "*"},
		AllowMethods: []string{echo.GET, echo.POST, echo.PUT, echo.DELETE, echo.OPTIONS},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, "Cache-Control", "Authorization", "X-Requested-With"},
		MaxAge:       86400, // Cache preflight for 24 hours
	}))

	v1 := e.Group("/v1")

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

	v1.GET("/feeds/fetch/single", func(c echo.Context) error {
		// Add caching headers
		c.Response().Header().Set("Cache-Control", "public, max-age=300") // 5 minutes
		c.Response().Header().Set("ETag", `"single-feed"`)

		feed, err := container.FetchSingleFeedUsecase.Execute(c.Request().Context())
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, feed)
	})

	v1.GET("/feeds/fetch/list", func(c echo.Context) error {
		// Add caching headers for feed list
		c.Response().Header().Set("Cache-Control", "public, max-age=900") // 15 minutes
		c.Response().Header().Set("ETag", `"feeds-list"`)

		feeds, err := container.FetchFeedsListUsecase.Execute(c.Request().Context())
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		// Optimize response size
		optimizedFeeds := optimizeFeedsResponse(feeds)
		return c.JSON(http.StatusOK, optimizedFeeds)
	})

	v1.GET("/feeds/fetch/limit/:limit", func(c echo.Context) error {
		limit, err := strconv.Atoi(c.Param("limit"))
		if err != nil {
			logger.Logger.Error("Error parsing limit", "error", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
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
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
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

		logger.Logger.Info("Fetching feeds with cursor", "cursor", cursor, "limit", limit)
		feeds, err := container.FetchFeedsListCursorUsecase.Execute(c.Request().Context(), cursor, limit)
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
			logger.Logger.Error("Error searching articles", "error", err)
			return c.JSON(http.StatusInternalServerError, errors.New("error searching articles"))
		}

		return c.JSON(http.StatusOK, results)
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
			logger.Logger.Error("Error fetching initial summarized articles count", "error", err)
			unsummarizedCount = 0
		}

		initialStats := FeedStatsSummary{
			FeedAmount:           feedAmount{Amount: amount},
			SummarizedFeedAmount: summarizedFeedAmount{Amount: unsummarizedCount},
		}

		// Send initial data
		if jsonData, err := json.Marshal(initialStats); err == nil {
			c.Response().Write([]byte("data: " + string(jsonData) + "\n\n"))
			flusher.Flush()
		}

		// Create ticker for periodic updates
		ticker := time.NewTicker(5 * time.Second) // Shortened for testing
		defer ticker.Stop()

		// Keep connection alive
		for {
			select {
			case <-ticker.C:
				// Fetch fresh data
				amount, err := container.FeedAmountUsecase.Execute(c.Request().Context())
				if err != nil {
					logger.Logger.Error("Error fetching feed amount", "error", err)
					continue
				}

				summarizedCount, err := container.SummarizedArticlesCountUsecase.Execute(c.Request().Context())
				if err != nil {
					logger.Logger.Error("Error fetching summarized articles count", "error", err)
					continue
				}

				stats := FeedStatsSummary{
					FeedAmount:           feedAmount{Amount: amount},
					SummarizedFeedAmount: summarizedFeedAmount{Amount: summarizedCount},
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

	v1.GET("/feeds/stats", func(c echo.Context) error {
		// Add caching headers for stats (5 minutes)
		c.Response().Header().Set("Cache-Control", "public, max-age=300")
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

	v1.POST("/rss-feed-link/register", func(c echo.Context) error {
		var rssFeedLink RssFeedLink
		err := c.Bind(&rssFeedLink)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}

		if strings.TrimSpace(rssFeedLink.URL) == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "URL is required and cannot be empty"})
		}

		// Parse and validate URL for SSRF protection
		parsedURL, err := url.Parse(rssFeedLink.URL)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid URL format"})
		}

		// Apply SSRF protection
		err = isAllowedURL(parsedURL)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}

		err = container.RegisterFeedsUsecase.Execute(c.Request().Context(), rssFeedLink.URL)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		// Invalidate cache after registration
		c.Response().Header().Set("Cache-Control", "no-cache")
		return c.JSON(http.StatusOK, map[string]string{"message": "RSS feed link registered"})
	})

}

// Optimize feeds response by truncating descriptions and removing unnecessary fields
func optimizeFeedsResponse(feeds []*domain.FeedItem) []*domain.FeedItem {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for _, feed := range feeds {
		feed.Title = strings.TrimSpace(feed.Title)
		feed.Description = sanitizeAndExtract(ctx, feed.Description) // ★ ここだけ変更
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
		return errors.New("only HTTP and HTTPS schemes allowed")
	}

	// Block private networks
	if isPrivateIP(u.Hostname()) {
		return errors.New("access to private networks not allowed")
	}

	// Block localhost variations
	hostname := strings.ToLower(u.Hostname())
	if hostname == "localhost" || hostname == "127.0.0.1" || strings.HasPrefix(hostname, "127.") {
		return errors.New("access to localhost not allowed")
	}

	// Block metadata endpoints (AWS, GCP, Azure)
	if hostname == "169.254.169.254" || hostname == "metadata.google.internal" {
		return errors.New("access to metadata endpoint not allowed")
	}

	// Block common internal domains
	internalDomains := []string{".local", ".internal", ".corp", ".lan"}
	for _, domain := range internalDomains {
		if strings.HasSuffix(hostname, domain) {
			return errors.New("access to internal domains not allowed")
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
