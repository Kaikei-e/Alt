package rest

import (
	"alt/di"
	"alt/domain"
	"alt/utils/html_parser"
	"alt/utils/logger"
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type rssFeedLink struct {
	URL string `json:"url"`
}

type readStatus struct {
	FeedURL string `json:"feed_url"`
}

func RegisterRoutes(e *echo.Echo, container *di.ApplicationComponents) {
	// Add performance middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Add compression middleware for better performance
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level: 5, // Balanced compression level
		Skipper: func(c echo.Context) bool {
			// Skip compression for already compressed content
			return strings.Contains(c.Request().Header.Get("Accept-Encoding"), "br") ||
				strings.Contains(c.Path(), "/health")
		},
	}))

	// Add request timeout middleware
	e.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{
		Timeout: 30 * time.Second,
	}))

	// Add rate limiting middleware
	e.Use(middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: middleware.NewRateLimiterMemoryStore(100), // 100 requests per second
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

	v1.POST("/feeds/read", func(c echo.Context) error {
		var readStatus readStatus
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

	v1.POST("/rss-feed-link/register", func(c echo.Context) error {
		var rssFeedLink rssFeedLink
		err := c.Bind(&rssFeedLink)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}

		if strings.TrimSpace(rssFeedLink.URL) == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "URL is required and cannot be empty"})
		}

		if !strings.HasPrefix(rssFeedLink.URL, "https://") {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "URL must start with https://"})
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
