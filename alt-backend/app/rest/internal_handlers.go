package rest

import (
	"alt/di"
	middleware_custom "alt/middleware"
	"alt/usecase/fetch_recent_articles_usecase"
	"alt/utils/logger"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

// registerInternalRoutes wires service-to-service endpoints used by internal
// callers (pre-processor, rag-orchestrator, etc.). Access is gated by
// X-Service-Token shared-secret authentication per ADR-000618. This is an
// application-layer defence complementing the transport-layer mTLS provided
// by the Linkerd sidecar; loss of either still leaves the other in place.
//
// Future work: migrate to per-caller signed service tokens (JWT with iss/sub)
// to give distinct identities to pre-processor, rag-orchestrator, etc.
func registerInternalRoutes(e *echo.Echo, container *di.ApplicationComponents) {
	serviceAuth := middleware_custom.NewServiceAuthMiddleware(logger.Logger)
	v1 := e.Group("/v1/internal", serviceAuth.RequireServiceAuth())

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
func handleFetchRecentArticles(container *di.ApplicationComponents) echo.HandlerFunc {
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
