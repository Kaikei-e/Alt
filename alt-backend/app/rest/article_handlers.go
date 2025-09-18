package rest

import (
	"alt/config"
	"alt/di"
	"alt/driver/search_indexer"
	middleware_custom "alt/middleware"
	"alt/utils/logger"
	"net/http"
	"net/url"

	"github.com/labstack/echo/v4"
)

func fetchArticleRoutes(v1 *echo.Group, container *di.ApplicationComponents, cfg *config.Config) {
	authMiddleware := middleware_custom.NewAuthMiddleware(container.AuthGateway, logger.Logger, cfg)
	articles := v1.Group("/articles", authMiddleware.RequireAuth())
	articles.GET("/fetch/content", handleFetchArticle(container))
}

func handleFetchArticle(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		var payload string
		if err := c.Bind(&payload); err != nil {
			return handleValidationError(c, "Invalid request format", "body", "malformed JSON")
		}

		if payload == "" {
			return handleValidationError(c, "Article URL is required", "body", "malformed JSON")
		}

		articleURL, err := url.Parse(payload)
		if err != nil {
			return handleValidationError(c, "Invalid article URL", "body", "malformed JSON")
		}

		err = isAllowedURL(articleURL)
		if err != nil {
			return handleValidationError(c, "Article URL not allowed", "body", "malformed JSON")
		}

		article, err := container.ArticleUsecase.Execute(c.Request().Context(), articleURL.String())
		if err != nil {
			return handleError(c, err, "fetch_article")
		}
		return c.JSON(http.StatusOK, article)
	}
}

func registerArticleRoutes(v1 *echo.Group, container *di.ApplicationComponents, cfg *config.Config) {
	// 認証ミドルウェアの初期化
	authMiddleware := middleware_custom.NewAuthMiddleware(container.AuthGateway, logger.Logger, cfg)

	// 記事検索も認証必須
	articles := v1.Group("/articles", authMiddleware.RequireAuth())
	articles.GET("/search", handleSearchArticles(container))
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
