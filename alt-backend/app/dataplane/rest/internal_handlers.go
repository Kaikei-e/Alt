package rest

import (
	"alt/di"
	"alt/orchestrator/usecase/fetch_recent_articles_usecase"
	"alt/utils/logger"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

// RegisterInternalRoutes wires the service-to-service REST endpoints used by
// pre-processor and rag-orchestrator. Neither carries auth middleware and
// neither takes a tenant argument — both answer system-level queries — so the
// listener they are mounted on is the whole access control.
//
// That listener is now cmd/datahub's mutual-TLS one, and this package is
// imported by nothing else, so "mounted on the browser-facing REST server by
// mistake" is no longer expressible: alt/orchestrator/rest cannot see these
// handlers at all.
//
// Converting these two routes to Connect RPCs is a separate wave: their
// callers are other services, so the protocol change needs a Pact CDC RED
// first (CLAUDE.md rule 7).
func RegisterInternalRoutes(e *echo.Echo, container *di.DataHubComponents) {
	v1 := e.Group("/v1/internal")

	v1.GET("/system-user", func(c echo.Context) error {
		ctx := c.Request().Context()

		// Fetch system user from Kratos (BFF/Aggregator pattern)
		// This allows us to get the first identity from the central identity provider
		// rather than maintaining a separate users table in alt-backend
		userID, err := container.KratosClient.GetFirstIdentityID(ctx)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to fetch system user from Kratos", "error", err)
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
//   - limit: Maximum articles to return (default: 100, max: 500, 0 means no limit - time constraint only)
func handleFetchRecentArticles(container *di.DataHubComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()

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

		// limit=0 means no limit (only time constraint applies)
		// This is useful for RAG use cases where all recent articles are needed
		limit := 100
		if limitStr := c.QueryParam("limit"); limitStr != "" {
			parsed, err := strconv.Atoi(limitStr)
			if err != nil || parsed < 0 {
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

		output, err := container.FetchRecentArticlesUsecase.Execute(ctx, input)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to fetch recent articles", "error", err)
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

// RecentArticleMetadata represents minimal article info for temporal queries.
// Used by rag-orchestrator for its temporal topics feature.
type RecentArticleMetadata struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	PublishedAt string   `json:"published_at"`
	FeedID      string   `json:"feed_id"`
	Tags        []string `json:"tags"`
}

// RecentArticlesResponse represents the response for recent articles query.
type RecentArticlesResponse struct {
	Articles []RecentArticleMetadata `json:"articles"`
	Since    string                  `json:"since"`
	Until    string                  `json:"until"`
	Count    int                     `json:"count"`
}
