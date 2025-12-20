package rest

import (
	"alt/config"
	"alt/di"
	"alt/domain"
	middleware_custom "alt/middleware"
	"alt/usecase/archive_article_usecase"
	"alt/utils/html_parser"
	"alt/utils/logger"
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
		articleURLStr := c.QueryParam("url")
		if articleURLStr == "" {
			return HandleValidationError(c, "Article URL is required", "url", "missing parameter")
		}

		articleURL, err := url.Parse(articleURLStr)
		if err != nil {
			return HandleValidationError(c, "Invalid article URL", "url", "invalid format")
		}

		err = IsAllowedURL(articleURL)
		if err != nil {
			return HandleValidationError(c, "Article URL not allowed", "url", "not allowed")
		}

		var contentStr string

		// Step 1: Check if article exists in database
		existingArticle, err := container.AltDBRepository.FetchArticleByURL(c.Request().Context(), articleURL.String())
		if err != nil {
			logger.Logger.Error("Failed to check for existing article", "error", err, "url", articleURL.String())
			return HandleError(c, fmt.Errorf("failed to check article existence: %w", err), "fetch_article")
		}

		if existingArticle != nil {
			// Article exists in DB - Zero Trust: Always extract text from stored content
			originalLength := len(existingArticle.Content)
			logger.Logger.Info("Article content retrieved from database, extracting text (Zero Trust validation)",
				"article_id", existingArticle.ID,
				"url", articleURL.String(),
				"original_length", originalLength)

			contentStr = html_parser.ExtractArticleText(existingArticle.Content)
			extractedLength := len(contentStr)
			reductionRatio := (1.0 - float64(extractedLength)/float64(originalLength)) * 100.0

			logger.Logger.Info("Text extraction completed",
				"article_id", existingArticle.ID,
				"original_length", originalLength,
				"extracted_length", extractedLength,
				"reduction_ratio", fmt.Sprintf("%.2f%%", reductionRatio))
		} else {
			// Article does not exist, fetch from Web
			logger.Logger.Info("Article not found in database, fetching from Web", "url", articleURL.String())
			fetchedContent, _, fetchedTitle, fetchErr := FetchArticleContent(c.Request().Context(), articleURL.String(), container)
			if fetchErr != nil {
				logger.Logger.Error("Failed to fetch article content", "error", fetchErr, "url", articleURL.String())
				return HandleError(c, fmt.Errorf("fetch article content failed for %q: %w", articleURL.String(), fetchErr), "fetch_article")
			}

			// Save to database
			_, saveErr := container.AltDBRepository.SaveArticle(c.Request().Context(), articleURL.String(), fetchedTitle, fetchedContent)
			if saveErr != nil {
				logger.Logger.Error("Failed to save article to database", "error", saveErr, "url", articleURL.String())
				// Continue even if save fails - we still have the content to return
			} else {
				logger.Logger.Info("Article content fetched from Web and saved to database", "url", articleURL.String())
			}

			// Zero Trust: Always extract text from HTML
			originalLength := len(fetchedContent)
			logger.Logger.Info("Extracting text from fetched content (Zero Trust validation)",
				"url", articleURL.String(),
				"original_length", originalLength)

			contentStr = html_parser.ExtractArticleText(fetchedContent)
			extractedLength := len(contentStr)
			reductionRatio := (1.0 - float64(extractedLength)/float64(originalLength)) * 100.0

			logger.Logger.Info("Text extraction completed from fetched content",
				"url", articleURL.String(),
				"original_length", originalLength,
				"extracted_length", extractedLength,
				"reduction_ratio", fmt.Sprintf("%.2f%%", reductionRatio))
		}

		// Ensure UTF-8 JSON and disallow MIME sniffing
		c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		c.Response().Header().Set("X-Content-Type-Options", "nosniff")

		response := map[string]string{
			"content": contentStr,
		}
		return c.JSON(http.StatusOK, response)
	}
}

func registerArticleRoutes(v1 *echo.Group, container *di.ApplicationComponents, cfg *config.Config) {
	// 認証ミドルウェアの初期化
	authMiddleware := middleware_custom.NewAuthMiddleware(logger.Logger, cfg.Auth.SharedSecret, cfg)

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
			return HandleError(c, err, "search_articles")
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
				return HandleValidationError(c, "Invalid limit parameter", "limit", limitStr)
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
				return HandleValidationError(c, "Invalid cursor format (expected RFC3339)", "cursor", cursorStr)
			}
			cursor = &parsedCursor
		}

		// Fetch limit+1 to determine if there are more items
		articles, err := container.FetchArticlesCursorUsecase.Execute(c.Request().Context(), cursor, limit+1)
		if err != nil {
			return HandleError(c, err, "fetch_articles_cursor")
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
