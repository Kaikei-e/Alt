package rest

import (
	"alt/di"
	"alt/usecase/fetch_recent_articles_usecase"
	"alt/utils/logger"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

func registerInternalRoutes(e *echo.Echo, container *di.ApplicationComponents) {
	// Internal routes group - restricted access recommended in production (e.g. via network policy)
	v1 := e.Group("/v1/internal")

	v1.GET("/system-user", func(c echo.Context) error {
		ctx := c.Request().Context()

		// Query the database directly via pool
		// We just need any valid user ID to associate system-generated/synced articles with
		// In a single-user system, getting the first user is sufficient
		var userID string
		err := container.AltDBRepository.GetPool().QueryRow(ctx, "SELECT id FROM users LIMIT 1").Scan(&userID)
		if err != nil {
			logger.Logger.Error("Failed to fetch system user", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to fetch system user",
			})
		}

		return c.JSON(http.StatusOK, map[string]string{
			"user_id": userID,
		})
	})

	// GET /v1/internal/articles/recent - Fetch recent articles for rag-orchestrator
	v1.GET("/articles/recent", handleFetchRecentArticles(container))
}

// handleFetchRecentArticles returns articles published within the specified time window
// Query params:
//   - within_hours: Time window in hours (default: 24, max: 168)
//   - limit: Maximum articles to return (default: 100, max: 500)
func handleFetchRecentArticles(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Parse query parameters
		withinHours := 24
		if withinHoursStr := c.QueryParam("within_hours"); withinHoursStr != "" {
			parsed, err := strconv.Atoi(withinHoursStr)
			if err != nil || parsed <= 0 {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "Invalid within_hours parameter",
				})
			}
			withinHours = parsed
		}

		limit := 100
		if limitStr := c.QueryParam("limit"); limitStr != "" {
			parsed, err := strconv.Atoi(limitStr)
			if err != nil || parsed <= 0 {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "Invalid limit parameter",
				})
			}
			limit = parsed
		}

		input := fetch_recent_articles_usecase.FetchRecentArticlesInput{
			WithinHours: withinHours,
			Limit:       limit,
		}

		output, err := container.FetchRecentArticlesUsecase.Execute(c.Request().Context(), input)
		if err != nil {
			logger.Logger.Error("Failed to fetch recent articles", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to fetch recent articles",
			})
		}

		// Convert to response format
		articles := make([]RecentArticleMetadata, len(output.Articles))
		for i, article := range output.Articles {
			articles[i] = RecentArticleMetadata{
				ID:          article.ID.String(),
				Title:       article.Title,
				URL:         article.URL,
				PublishedAt: article.PublishedAt.Format(time.RFC3339),
				FeedID:      article.FeedID.String(),
				Tags:        article.Tags,
			}
		}

		response := RecentArticlesResponse{
			Articles: articles,
			Since:    output.Since.Format(time.RFC3339),
			Until:    output.Until.Format(time.RFC3339),
			Count:    output.Count,
		}

		c.Response().Header().Set("Cache-Control", "private, max-age=60")
		return c.JSON(http.StatusOK, response)
	}
}
