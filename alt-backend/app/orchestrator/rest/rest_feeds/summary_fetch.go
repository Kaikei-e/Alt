package rest_feeds

import (
	"alt/config"
	"alt/di"
	"alt/domain"
	"alt/utils/errors"
	"alt/utils/logger"
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v4"
)

func RestHandleFetchInoreaderSummary(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		var req FeedSummaryRequest
		if err := c.Bind(&req); err != nil {
			return HandleValidationError(c, "Invalid request format", "body", "malformed JSON")
		}

		// Manual validation - check if feed_urls is provided and within limits
		if len(req.FeedURLs) == 0 {
			return HandleValidationError(c, "feed_urls is required and cannot be empty", "feed_urls", req.FeedURLs)
		}
		if len(req.FeedURLs) > 50 {
			return HandleValidationError(c, "Maximum 50 URLs allowed per request", "feed_urls", len(req.FeedURLs))
		}

		// SSRF protection for all URLs
		for _, feedURL := range req.FeedURLs {
			parsedURL, err := url.Parse(feedURL)
			if err != nil {
				return HandleValidationError(c, "Invalid URL format", "feed_urls", feedURL)
			}

			if err := IsAllowedURL(parsedURL); err != nil {
				securityErr := errors.NewValidationContextError(
					"URL not allowed for security reasons",
					"rest",
					"RESTHandler",
					"fetch_inoreader_summary",
					map[string]interface{}{
						"url":         feedURL,
						"reason":      err.Error(),
						"path":        c.Request().URL.Path,
						"method":      c.Request().Method,
						"remote_addr": c.Request().RemoteAddr,
						"request_id":  c.Response().Header().Get("X-Request-ID"),
					},
				)
				logger.Logger.ErrorContext(ctx, "URL validation failed", "error", securityErr.Error(), "url", feedURL)
				return c.JSON(securityErr.HTTPStatusCode(), securityErr.ToHTTPResponse())
			}
		}

		// Log requested URLs for debugging
		logger.Logger.InfoContext(ctx, "Fetching summaries for URLs", "urls", req.FeedURLs, "url_count", len(req.FeedURLs))

		// Execute usecase
		summaries, err := container.FetchInoreaderSummaryUsecase.Execute(ctx, req.FeedURLs)
		if err != nil {
			return HandleError(c, err, "fetch_inoreader_summary")
		}

		// Convert domain entities to response DTOs
		responses := make([]InoreaderSummaryResponse, 0, len(summaries))
		for _, summary := range summaries {
			authorStr := ""
			if summary.Author != nil {
				authorStr = *summary.Author
			}

			resp := InoreaderSummaryResponse{
				ArticleURL:  summary.ArticleURL,
				Title:       summary.Title,
				Author:      authorStr,
				Content:     summary.Content,
				ContentType: summary.ContentType,
				PublishedAt: summary.PublishedAt.Format(time.RFC3339),
				FetchedAt:   summary.FetchedAt.Format(time.RFC3339),
				InoreaderID: summary.InoreaderID,
			}
			responses = append(responses, resp)
		}

		// Build final response
		finalResponse := FeedSummaryProvidedResponse{
			MatchedArticles: responses,
			TotalMatched:    len(responses),
			RequestedCount:  len(req.FeedURLs),
		}

		// Set caching headers (15 minutes as per XPLAN11.md)
		c.Response().Header().Set("Cache-Control", "public, max-age=900")
		c.Response().Header().Set("Content-Type", "application/json")

		return c.JSON(http.StatusOK, finalResponse)
	}
}

// handleFetchArticleSummary handles article summary fetch requests
// It fetches summaries from alt-db if available, or generates them via news-creator
func RestHandleFetchArticleSummary(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()

		// Get user context for saving summary
		userCtx, err := domain.GetUserFromContext(ctx)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to get user context for article summary fetch", "error", err)
			return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
		}

		var req FeedSummaryRequest
		if err := c.Bind(&req); err != nil {
			return HandleValidationError(c, "Invalid request format", "body", "malformed JSON")
		}

		// Manual validation - check if feed_urls is provided and within limits
		if len(req.FeedURLs) == 0 {
			return HandleValidationError(c, "feed_urls is required and cannot be empty", "feed_urls", req.FeedURLs)
		}
		if len(req.FeedURLs) > 50 {
			return HandleValidationError(c, "Maximum 50 URLs allowed per request", "feed_urls", len(req.FeedURLs))
		}

		// SSRF protection for all URLs
		for _, feedURL := range req.FeedURLs {
			parsedURL, err := url.Parse(feedURL)
			if err != nil {
				return HandleValidationError(c, "Invalid URL format", "feed_urls", feedURL)
			}

			if err := IsAllowedURL(parsedURL); err != nil {
				securityErr := errors.NewValidationContextError(
					"URL not allowed for security reasons",
					"rest",
					"RESTHandler",
					"fetch_article_summary",
					map[string]interface{}{
						"url":         feedURL,
						"reason":      err.Error(),
						"path":        c.Request().URL.Path,
						"method":      c.Request().Method,
						"remote_addr": c.Request().RemoteAddr,
						"request_id":  c.Response().Header().Get("X-Request-ID"),
					},
				)
				logger.Logger.ErrorContext(ctx, "URL validation failed", "error", securityErr.Error(), "url", feedURL)
				return c.JSON(securityErr.HTTPStatusCode(), securityErr.ToHTTPResponse())
			}
		}

		logger.Logger.InfoContext(ctx, "Fetching article summaries", "url_count", len(req.FeedURLs))

		summaryResults := container.FetchArticleSummariesUsecase.Execute(ctx, userCtx.UserID.String(), req.FeedURLs)

		matchedArticles := make([]InoreaderSummaryResponse, 0, len(summaryResults))
		for _, result := range summaryResults {
			matchedArticles = append(matchedArticles, InoreaderSummaryResponse{
				ArticleURL:  result.FeedURL,
				Title:       result.Title,
				Author:      "", // Author is not available from articles table
				Content:     result.Summary,
				ContentType: "text/html",
				PublishedAt: time.Now().Format(time.RFC3339), // Use current time as fallback
				FetchedAt:   time.Now().Format(time.RFC3339),
				InoreaderID: result.ArticleID, // Use article_id as source_id
			})
		}

		// Build final response
		finalResponse := FeedSummaryProvidedResponse{
			MatchedArticles: matchedArticles,
			TotalMatched:    len(matchedArticles),
			RequestedCount:  len(req.FeedURLs),
		}

		// Set caching headers
		c.Response().Header().Set("Cache-Control", "private, max-age=300") // 5 minutes for generated summaries
		c.Response().Header().Set("Content-Type", "application/json")

		logger.Logger.InfoContext(ctx, "Article summaries fetched successfully",
			"requested_count", len(req.FeedURLs),
			"matched_count", len(matchedArticles))

		return c.JSON(http.StatusOK, finalResponse)
	}
}
