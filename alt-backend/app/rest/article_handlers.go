package rest

import (
	"alt/di"
	"alt/driver/search_indexer"
	middleware_custom "alt/middleware"
	"alt/utils/logger"
	"fmt"
	"net/http"
	"net/url"

	"github.com/labstack/echo/v4"
)

func fetchArticleRoutes(v1 *echo.Group, container *di.ApplicationComponents) {
	authMiddleware := middleware_custom.NewAuthMiddleware(logger.Logger)
	articles := v1.Group("/articles", authMiddleware.RequireAuth())
	articles.GET("/fetch/content", handleFetchArticle(container))
}

func handleFetchArticle(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		articleURLStr := c.QueryParam("url")
		if articleURLStr == "" {
			return handleValidationError(c, "Article URL is required", "url", "missing parameter")
		}

		articleURL, err := url.Parse(articleURLStr)
		if err != nil {
			return handleValidationError(c, "Invalid article URL", "url", "invalid format")
		}

		err = isAllowedURL(articleURL)
		if err != nil {
			return handleValidationError(c, "Article URL not allowed", "url", "not allowed")
		}

		content, err := container.ArticleUsecase.Execute(c.Request().Context(), articleURL.String())
		if err != nil {
			return handleError(c, fmt.Errorf("fetch article content failed for %q: %w", articleURL.String(), err), "fetch_article")
		}

		// Return JSON object matching FeedContentOnTheFlyResponse interface
		// Handle nil content to prevent panic
		contentStr := ""
		if content != nil {
			contentStr = *content
		}

		// Ensure UTF-8 JSON and disallow MIME sniffing
		c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSONCharsetUTF8)
		c.Response().Header().Set("X-Content-Type-Options", "nosniff")

		response := map[string]string{
			"content": contentStr,
		}
		return c.JSON(http.StatusOK, response)
	}
}

func registerArticleRoutes(v1 *echo.Group, container *di.ApplicationComponents) {
	// 認証ミドルウェアの初期化
	authMiddleware := middleware_custom.NewAuthMiddleware(logger.Logger)

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
