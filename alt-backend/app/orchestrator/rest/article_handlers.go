package rest

import (
	"alt/config"
	"alt/di"
	"alt/domain"
	middleware_custom "alt/middleware"
	"alt/orchestrator/rest/resterr"
	"alt/utils/logger"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// maxArticlesPageSize is the largest page the cursor handlers will serve.
//
// It sits one below the usecases' own ceiling of 100 on purpose: these
// handlers ask for limit+1 rows so they can answer has_more without a separate
// COUNT, so a page of 100 would fetch 101 and the usecase would reject it.
// Clamping to 100 turned the documented maximum page size into an opaque 500.
const maxArticlesPageSize = 99

func fetchArticleRoutes(v1 *echo.Group, container *di.ApplicationComponents, cfg config.AuthConfig) {
	authMiddleware := middleware_custom.NewAuthMiddleware(logger.Logger, cfg)
	articles := v1.Group("/articles", authMiddleware.RequireAuth())
	articles.GET("/fetch/content", handleFetchArticle(container))
	articles.GET("/fetch/cursor", handleFetchArticlesCursor(container))
	articles.GET("/by-tag", handleFetchArticlesByTag(container))
	articles.GET("/:id/tags", handleFetchArticleTags(container))
	articles.POST("/archive", handleArchiveArticle(container))
}

// parseArticleLimit parses and clamps page limit for article pagination.
func parseArticleLimit(limitStr string) (int, error) {
	if limitStr == "" {
		return 20, nil
	}
	parsedLimit, err := strconv.Atoi(limitStr)
	if err != nil || parsedLimit <= 0 {
		return 0, fmt.Errorf("invalid limit parameter: %s", limitStr)
	}
	if parsedLimit > maxArticlesPageSize {
		return maxArticlesPageSize, nil
	}
	return parsedLimit, nil
}

// parseArticleCursor parses RFC3339 cursor for article pagination.
func parseArticleCursor(cursorStr string) (*time.Time, error) {
	if cursorStr == "" {
		return nil, nil
	}
	parsedCursor, err := time.Parse(time.RFC3339, cursorStr)
	if err != nil {
		return nil, err
	}
	return &parsedCursor, nil
}

func handleFetchArticlesCursor(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		_, err := domain.GetUserFromContext(ctx)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "user context not found", "error", err)
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		}

		limitStr := c.QueryParam("limit")
		limit, err := parseArticleLimit(limitStr)
		if err != nil {
			return resterr.HandleValidationError(c, "Invalid limit parameter", "limit", limitStr)
		}

		cursorStr := c.QueryParam("cursor")
		cursor, err := parseArticleCursor(cursorStr)
		if err != nil {
			return resterr.HandleValidationError(c, "Invalid cursor format (expected RFC3339)", "cursor", cursorStr)
		}

		articles, err := container.FetchArticlesCursorUsecase.Execute(ctx, cursor, limit+1)
		if err != nil {
			return resterr.HandleError(c, err, "fetch_articles_cursor")
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

		// Sub-second precision: the cursor comes back as the right-hand side of
		// a strict `created_at < $1`, and created_at is microsecond precision.
		var nextCursor *string
		if hasMore && len(articles) > 0 {
			lastArticle := articles[len(articles)-1]
			cursorStr := lastArticle.PublishedAt.Format(time.RFC3339Nano)
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

// TagTrailArticleResponse represents an article in the Tag Trail feature response.
type TagTrailArticleResponse struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Link        string `json:"link"`
	PublishedAt string `json:"published_at"`
	FeedTitle   string `json:"feed_title,omitempty"`
}

// ArticlesByTagResponse represents the paginated response for articles by tag.
type ArticlesByTagResponse struct {
	Articles   []TagTrailArticleResponse `json:"articles"`
	NextCursor *string                   `json:"next_cursor,omitempty"`
	HasMore    bool                      `json:"has_more"`
}

func handleFetchArticlesByTag(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		_, err := domain.GetUserFromContext(ctx)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "user context not found", "error", err)
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		}

		tagName := c.QueryParam("tag_name")
		tagID := c.QueryParam("tag_id")

		// Require at least one of tag_name or tag_id
		if strings.TrimSpace(tagName) == "" && strings.TrimSpace(tagID) == "" {
			return resterr.HandleValidationError(c, "tag_name or tag_id is required", "tag_name", tagName)
		}

		limitStr := c.QueryParam("limit")
		limit, err := parseArticleLimit(limitStr)
		if err != nil {
			return resterr.HandleValidationError(c, "Invalid limit parameter", "limit", limitStr)
		}

		cursorStr := c.QueryParam("cursor")
		cursor, err := parseArticleCursor(cursorStr)
		if err != nil {
			return resterr.HandleValidationError(c, "Invalid cursor format (expected RFC3339)", "cursor", cursorStr)
		}

		// Fetch limit+1 to determine if there are more results
		// Prioritize tag_name over tag_id for cross-feed discovery
		var articles []*domain.TagTrailArticle
		if strings.TrimSpace(tagName) != "" {
			articles, err = container.FetchArticlesByTagUsecase.ExecuteByTagName(ctx, tagName, cursor, limit+1)
		} else {
			articles, err = container.FetchArticlesByTagUsecase.Execute(ctx, tagID, cursor, limit+1)
		}
		if err != nil {
			return resterr.HandleError(c, err, "fetch_articles_by_tag")
		}

		hasMore := len(articles) > limit
		if hasMore {
			articles = articles[:limit]
		}

		articleResponses := make([]TagTrailArticleResponse, len(articles))
		for i, article := range articles {
			articleResponses[i] = TagTrailArticleResponse{
				ID:          article.ID,
				Title:       article.Title,
				Link:        article.Link,
				PublishedAt: article.PublishedAt.Format(time.RFC3339),
				FeedTitle:   article.FeedTitle,
			}
		}

		// Sub-second precision: the cursor comes back as the right-hand side of
		// a strict `created_at < $1`, and created_at is microsecond precision.
		var nextCursor *string
		if hasMore && len(articles) > 0 {
			lastArticle := articles[len(articles)-1]
			cursorStr := lastArticle.PublishedAt.Format(time.RFC3339Nano)
			nextCursor = &cursorStr
		}

		response := ArticlesByTagResponse{
			Articles:   articleResponses,
			NextCursor: nextCursor,
			HasMore:    hasMore,
		}

		c.Response().Header().Set("Cache-Control", "private, max-age=60")
		return c.JSON(http.StatusOK, response)
	}
}

// ArticleTagResponse represents a tag in the article tags response.
type ArticleTagResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// ArticleTagsResponse represents the response for fetching article tags.
type ArticleTagsResponse struct {
	ArticleID string               `json:"article_id"`
	Tags      []ArticleTagResponse `json:"tags"`
}

func handleFetchArticleTags(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		_, err := domain.GetUserFromContext(ctx)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "user context not found", "error", err)
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		}

		articleID := c.Param("id")
		if strings.TrimSpace(articleID) == "" {
			return resterr.HandleValidationError(c, "article id is required", "id", articleID)
		}

		tags, err := container.FetchArticleTagsUsecase.Execute(ctx, articleID)
		if err != nil {
			return resterr.HandleError(c, err, "fetch_article_tags")
		}

		tagResponses := make([]ArticleTagResponse, len(tags))
		for i, tag := range tags {
			tagResponses[i] = ArticleTagResponse{
				ID:        tag.ID,
				Name:      tag.TagName,
				CreatedAt: tag.CreatedAt.Format(time.RFC3339),
			}
		}

		response := ArticleTagsResponse{
			ArticleID: articleID,
			Tags:      tagResponses,
		}

		c.Response().Header().Set("Cache-Control", "private, max-age=60")
		return c.JSON(http.StatusOK, response)
	}
}
