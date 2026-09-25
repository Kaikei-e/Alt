package rest

import (
	"alt/config"
	"alt/di"
	"alt/domain"
	middleware_custom "alt/middleware"
	"alt/orchestrator/rest/resterr"
	"alt/utils/logger"
	"net/http"

	"github.com/labstack/echo/v4"
)

func registerArticleRoutes(v1 *echo.Group, container *di.ApplicationComponents, cfg config.AuthConfig) {
	authMiddleware := middleware_custom.NewAuthMiddleware(logger.Logger, cfg)
	articles := v1.Group("/articles", authMiddleware.RequireAuth())
	articles.GET("/search", handleSearchArticles(container))
}

func handleSearchArticles(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		_, err := domain.GetUserFromContext(ctx)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "user context not found", "error", err)
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		}

		query := c.QueryParam("q")
		if query == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "search query must not be empty"})
		}

		results, err := container.ArticleSearchUsecase.Execute(ctx, query)
		if err != nil {
			return resterr.HandleError(c, err, "search_articles")
		}

		return c.JSON(http.StatusOK, results)
	}
}
