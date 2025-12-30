package rest

import (
	"alt/config"
	"alt/di"
	"alt/domain"
	middleware_custom "alt/middleware"
	"alt/usecase/archive_article_usecase"
	"alt/utils/html_parser"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

func fetchArticleRoutes(v1 *echo.Group, container *di.ApplicationComponents, cfg *config.Config) {
	authMiddleware := middleware_custom.NewAuthMiddleware(logger.Logger, cfg.Auth.SharedSecret, cfg)
	articles := v1.Group("/articles", authMiddleware.RequireAuth())
	articles.GET("/fetch/content", handleFetchArticle(container))
	articles.GET("/fetch/cursor", handleFetchArticlesCursor(container))
	articles.POST("/archive", handleArchiveArticle(container))
}

func handleArchiveArticle(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		var payload ArchiveArticleRequest
		if err := c.Bind(&payload); err != nil {
			return HandleValidationError(c, "Invalid request format", "body", "malformed JSON")
		}

		if strings.TrimSpace(payload.FeedURL) == "" {
			return HandleValidationError(c, "Article URL is required", "feed_url", payload.FeedURL)
		}

		articleURL, err := url.Parse(payload.FeedURL)
		if err != nil {
			return HandleValidationError(c, "Invalid article URL", "feed_url", payload.FeedURL)
		}

		if err := IsAllowedURL(articleURL); err != nil {
			return HandleValidationError(c, "Article URL not allowed", "feed_url", payload.FeedURL)
		}

		input := archive_article_usecase.ArchiveArticleInput{
			URL:   articleURL.String(),
			Title: payload.Title,
		}

		if err := container.ArchiveArticleUsecase.Execute(c.Request().Context(), input); err != nil {
			return HandleError(c, fmt.Errorf("archive article failed for %q: %w", articleURL.String(), err), "archive_article")
		}

		c.Response().Header().Set("Cache-Control", "no-cache")
		return c.JSON(http.StatusOK, map[string]string{"message": "article archived"})
	}
}

func handleFetchArticle(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		targetURL := c.QueryParam("url")
		parsedURL, err := validateFetchRequest(c, targetURL)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}

		user, err := domain.GetUserFromContext(c.Request().Context())
		if err != nil {
			logger.Logger.Warn("No user context for fetch article, proceeding as anonymous", "error", err)
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		}

		// Call the usecase
		content, articleID, err := container.ArticleUsecase.FetchCompliantArticle(c.Request().Context(), parsedURL, *user)
		if err != nil {
			var complianceErr *domain.ComplianceError
			if errors.As(err, &complianceErr) {
				return c.JSON(complianceErr.Code, map[string]string{"error": complianceErr.Message})
			}

			if errors.Is(err, context.DeadlineExceeded) {
				return c.JSON(http.StatusGatewayTimeout, map[string]string{"error": "Request timeout"})
			}
			logger.Logger.Error("Failed to fetch compliant article", "error", err, "url", targetURL)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch article"})
		}

		return returnArticleResponse(c, parsedURL, content, articleID)
	}
}

func validateFetchRequest(c echo.Context, targetURL string) (*url.URL, error) {
	if targetURL == "" {
		return nil, fmt.Errorf("url parameter is required")
	}

	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL")
	}

	if err := IsAllowedURL(parsedURL); err != nil {
		return nil, fmt.Errorf("invalid URL scheme or private IP blocked")
	}

	return parsedURL, nil
}

func returnArticleResponse(c echo.Context, articleURL *url.URL, content string, articleID string) error {
	escapedContent := html_parser.StripTags(content)
	return c.JSON(http.StatusOK, map[string]string{
		"url":        articleURL.String(),
		"content":    escapedContent,
		"article_id": articleID,
	})
}

func registerArticleRoutes(v1 *echo.Group, container *di.ApplicationComponents, cfg *config.Config) {
	authMiddleware := middleware_custom.NewAuthMiddleware(logger.Logger, cfg.Auth.SharedSecret, cfg)
	articles := v1.Group("/articles", authMiddleware.RequireAuth())
	articles.GET("/search", handleSearchArticles(container))
}

func handleSearchArticles(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		_, err := domain.GetUserFromContext(c.Request().Context())
		if err != nil {
			logger.Logger.Error("user context not found", "error", err)
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		}

		query := c.QueryParam("q")
		if query == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "search query must not be empty"})
		}

		results, err := container.ArticleSearchUsecase.Execute(c.Request().Context(), query)
		if err != nil {
			return HandleError(c, err, "search_articles")
		}

		return c.JSON(http.StatusOK, results)
	}
}

func handleFetchArticlesCursor(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		_, err := domain.GetUserFromContext(c.Request().Context())
		if err != nil {
			logger.Logger.Error("user context not found", "error", err)
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		}

		limit := 20
		if limitStr := c.QueryParam("limit"); limitStr != "" {
			parsedLimit, err := strconv.Atoi(limitStr)
			if err != nil || parsedLimit <= 0 {
				return HandleValidationError(c, "Invalid limit parameter", "limit", limitStr)
			}
			limit = parsedLimit
			if limit > 100 {
				limit = 100
			}
		}

		var cursor *time.Time
		if cursorStr := c.QueryParam("cursor"); cursorStr != "" {
			parsedCursor, err := time.Parse(time.RFC3339, cursorStr)
			if err != nil {
				return HandleValidationError(c, "Invalid cursor format (expected RFC3339)", "cursor", cursorStr)
			}
			cursor = &parsedCursor
		}

		articles, err := container.FetchArticlesCursorUsecase.Execute(c.Request().Context(), cursor, limit+1)
		if err != nil {
			return HandleError(c, err, "fetch_articles_cursor")
		}

		hasMore := len(articles) > limit
		if hasMore {
			articles = articles[:limit]
		}

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

		c.Response().Header().Set("Cache-Control", "private, max-age=60")
		return c.JSON(http.StatusOK, response)
	}
}
