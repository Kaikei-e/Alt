package rest

import (
	"alt/di"
	"alt/domain"
	middleware_custom "alt/middleware"
	"alt/usecase/archive_article_usecase"
	"alt/utils/logger"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

func fetchArticleRoutes(v1 *echo.Group, container *di.ApplicationComponents) {
	authMiddleware := middleware_custom.NewAuthMiddleware(logger.Logger)
	articles := v1.Group("/articles", authMiddleware.RequireAuth())
	articles.GET("/fetch/content", handleFetchArticle(container))
	articles.GET("/fetch/cursor", handleFetchArticlesCursor(container))
	articles.POST("/archive", handleArchiveArticle(container))
}

func handleArchiveArticle(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		var payload ArchiveArticleRequest
		if err := c.Bind(&payload); err != nil {
			return handleValidationError(c, "Invalid request format", "body", "malformed JSON")
		}

		if strings.TrimSpace(payload.FeedURL) == "" {
			return handleValidationError(c, "Article URL is required", "feed_url", payload.FeedURL)
		}

		articleURL, err := url.Parse(payload.FeedURL)
		if err != nil {
			return handleValidationError(c, "Invalid article URL", "feed_url", payload.FeedURL)
		}

		if err := isAllowedURL(articleURL); err != nil {
			return handleValidationError(c, "Article URL not allowed", "feed_url", payload.FeedURL)
		}

		input := archive_article_usecase.ArchiveArticleInput{
			URL:   articleURL.String(),
			Title: payload.Title,
		}

		if err := container.ArchiveArticleUsecase.Execute(c.Request().Context(), input); err != nil {
			return handleError(c, fmt.Errorf("archive article failed for %q: %w", articleURL.String(), err), "archive_article")
		}

		c.Response().Header().Set("Cache-Control", "no-cache")
		return c.JSON(http.StatusOK, map[string]string{"message": "article archived"})
	}
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
		// Verify user authentication from context
		_, err := domain.GetUserFromContext(c.Request().Context())
		if err != nil {
			logger.Logger.Error("user context not found", "error", err)
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "authentication required",
			})
		}

		query := c.QueryParam("q")
		if query == "" {
			logger.Logger.Error("search query must not be empty")
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "search query must not be empty",
			})
		}

		// Use ArticleSearchUsecase which searches via Meilisearch with user_id filtering
		results, err := container.ArticleSearchUsecase.Execute(c.Request().Context(), query)
		if err != nil {
			return handleError(c, err, "search_articles")
		}

		return c.JSON(http.StatusOK, results)
	}
}

func handleFetchArticlesCursor(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Verify user authentication from context
		_, err := domain.GetUserFromContext(c.Request().Context())
		if err != nil {
			logger.Logger.Error("user context not found", "error", err)
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "authentication required",
			})
		}

		// Parse limit parameter (default: 20, max: 100)
		limit := 20
		if limitStr := c.QueryParam("limit"); limitStr != "" {
			parsedLimit, err := strconv.Atoi(limitStr)
			if err != nil || parsedLimit <= 0 {
				return handleValidationError(c, "Invalid limit parameter", "limit", limitStr)
			}
			limit = parsedLimit
			if limit > 100 {
				limit = 100
			}
		}

		// Parse cursor parameter (optional, RFC3339 timestamp)
		var cursor *time.Time
		if cursorStr := c.QueryParam("cursor"); cursorStr != "" {
			parsedCursor, err := time.Parse(time.RFC3339, cursorStr)
			if err != nil {
				return handleValidationError(c, "Invalid cursor format (expected RFC3339)", "cursor", cursorStr)
			}
			cursor = &parsedCursor
		}

		// Fetch limit+1 to determine if there are more items
		articles, err := container.FetchArticlesCursorUsecase.Execute(c.Request().Context(), cursor, limit+1)
		if err != nil {
			return handleError(c, err, "fetch_articles_cursor")
		}

		// Prepare response
		hasMore := len(articles) > limit
		if hasMore {
			articles = articles[:limit]
		}

		// Convert to response format
		articleResponses := make([]ArticleResponse, len(articles))
		for i, article := range articles {
			articleResponses[i] = ArticleResponse{
				ID:          article.ID.String(),
				Title:       article.Title,
				URL:         article.URL,
				Content:     article.Content,
				PublishedAt: article.PublishedAt.Format(time.RFC3339),
				Tags:        article.Tags,
			}
		}

		// Generate next cursor from the last item
		var nextCursor *string
		if hasMore && len(articles) > 0 {
			lastArticle := articles[len(articles)-1]
			cursorStr := lastArticle.PublishedAt.Format(time.RFC3339)
			nextCursor = &cursorStr
		}

		response := ArticlesWithCursorResponse{
			Data:       articleResponses,
			NextCursor: nextCursor,
			HasMore:    hasMore,
		}

		// Set caching headers
		c.Response().Header().Set("Cache-Control", "private, max-age=60")
		return c.JSON(http.StatusOK, response)
	}
}
