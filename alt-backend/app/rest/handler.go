package rest

import (
	"alt/di"
	"alt/domain"
	"alt/utils/logger"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type rssFeedLink struct {
	URL string `json:"url"`
}

type readStatus struct {
	FeedID string `json:"feed_id"`
}

func RegisterRoutes(e *echo.Echo, container *di.ApplicationComponents) {
	// Add CORS middleware
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"http://localhost:3000", "http://localhost:80", "*"},
		AllowMethods: []string{echo.GET, echo.POST, echo.PUT, echo.DELETE, echo.OPTIONS},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
	}))

	v1 := e.Group("/v1")
	v1.GET("/health", func(c echo.Context) error {
		response := map[string]string{
			"status": "healthy",
		}
		return c.JSON(http.StatusOK, response)
	})

	v1.GET("/feeds/fetch/single", func(c echo.Context) error {
		feed, err := container.FetchSingleFeedUsecase.Execute(c.Request().Context())
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, feed)
	})

	v1.GET("/feeds/fetch/list", func(c echo.Context) error {
		feeds, err := container.FetchFeedsListUsecase.Execute(c.Request().Context())
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, feeds)
	})

	v1.GET("/feeds/fetch/limit/:limit", func(c echo.Context) error {
		limit, err := strconv.Atoi(c.Param("limit"))
		if err != nil {
			logger.Logger.Error("Error parsing limit", "error", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		feeds, err := container.FetchFeedsListUsecase.ExecuteLimit(c.Request().Context(), limit)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, feeds)
	})

	v1.GET("/feeds/fetch/page/:page", func(c echo.Context) error {
		page, err := strconv.Atoi(c.Param("page"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		feeds, err := container.FetchFeedsListUsecase.ExecutePage(c.Request().Context(), page)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		feeds = removeEscapedString(feeds)

		return c.JSON(http.StatusOK, feeds)
	})

	v1.POST("/feeds/read", func(c echo.Context) error {
		var readStatus readStatus
		err := c.Bind(&readStatus)
		if err != nil {
			logger.Logger.Error("Error binding read status", "error", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		feedURL, err := url.Parse(readStatus.FeedID)
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
		return c.JSON(http.StatusOK, map[string]string{"message": "RSS feed link registered"})
	})
}

func removeEscapedString(feeds []*domain.FeedItem) []*domain.FeedItem {
	for _, feed := range feeds {
		feed.Description = strings.ReplaceAll(feed.Description, "\n", "")
	}
	return feeds
}
