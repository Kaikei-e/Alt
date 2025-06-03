package rest

import (
	"alt/di"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

type rssFeedLink struct {
	URL string `json:"url"`
}

func RegisterRoutes(e *echo.Echo, container *di.ApplicationComponents) {
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
