package rest

import (
	"alt/di"
	"alt/driver/search_indexer"
	"alt/utils/logger"
	"net/http"

	"github.com/labstack/echo/v4"
)

func registerArticleRoutes(v1 *echo.Group, container *di.ApplicationComponents) {
	v1.GET("/articles/search", handleSearchArticles(container))
}

func handleSearchArticles(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
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
	}
}