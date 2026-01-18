package rest_feeds

import (
	"alt/config"
	"alt/di"
	"alt/utils/logger"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

func RestHandleFetchFeedDetails(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		var payload FeedUrlPayload
		err := c.Bind(&payload)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Error binding feed URL", "error", err)
			return HandleValidationError(c, "Invalid request format", "body", nil)
		}

		feedURLParsed, err := url.Parse(payload.FeedURL)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Error parsing feed URL", "error", err)
			return HandleValidationError(c, "Invalid URL format", "feed_url", payload.FeedURL)
		}

		err = IsAllowedURL(feedURLParsed)
		if err != nil {
			return HandleValidationError(c, "URL not allowed for security reasons", "feed_url", payload.FeedURL)
		}

		details, err := container.FeedsSummaryUsecase.Execute(ctx, feedURLParsed)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Error fetching feed details", "error", err)
			return HandleError(c, err, "FetchFeedDetails")
		}
		return c.JSON(http.StatusOK, details)
	}
}

func RestHandleFeedStats(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		// Add caching headers for stats
		c.Response().Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(cfg.Cache.FeedCacheExpiry.Seconds())))
		c.Response().Header().Set("ETag", `"feeds-stats"`)

		// Fetch feed amount
		feedCount, err := container.FeedAmountUsecase.Execute(ctx)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Error fetching feed amount", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch feed statistics"})
		}

		// Fetch summarized articles count
		summarizedCount, err := container.SummarizedArticlesCountUsecase.Execute(ctx)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Error fetching summarized articles count", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch feed statistics"})
		}

		// Create response in expected format
		stats := FeedStatsSummary{
			FeedAmount:           feedAmount{Amount: feedCount},
			SummarizedFeedAmount: summarizedFeedAmount{Amount: summarizedCount},
		}

		logger.Logger.InfoContext(ctx, "Feed stats retrieved successfully",
			"feed_count", feedCount,
			"summarized_count", summarizedCount)

		return c.JSON(http.StatusOK, stats)
	}
}

func RestHandleDetailedFeedStats(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		// Add caching headers for stats
		c.Response().Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(cfg.Cache.FeedCacheExpiry.Seconds())))
		c.Response().Header().Set("ETag", `"feeds-stats-detailed"`)

		var wg sync.WaitGroup
		var mu sync.Mutex
		var firstErr error

		// Results storage
		var feedCount int
		var totalArticlesCount int
		var unsummarizedCount int

		// Fetch feed amount in parallel
		wg.Add(1)
		go func() {
			defer wg.Done()
			count, err := container.FeedAmountUsecase.Execute(ctx)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("failed to fetch feed amount: %w", err)
				}
				logger.Logger.ErrorContext(ctx, "Error fetching feed amount", "error", err)
				return
			}
			feedCount = count
		}()

		// Fetch total articles count in parallel
		wg.Add(1)
		go func() {
			defer wg.Done()
			count, err := container.TotalArticlesCountUsecase.Execute(ctx)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("failed to fetch total articles count: %w", err)
				}
				logger.Logger.ErrorContext(ctx, "Error fetching total articles count", "error", err)
				return
			}
			totalArticlesCount = count
		}()

		// Fetch unsummarized articles count in parallel
		wg.Add(1)
		go func() {
			defer wg.Done()
			count, err := container.UnsummarizedArticlesCountUsecase.Execute(ctx)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("failed to fetch unsummarized articles count: %w", err)
				}
				logger.Logger.ErrorContext(ctx, "Error fetching unsummarized articles count", "error", err)
				return
			}
			unsummarizedCount = count
		}()

		// Wait for all goroutines to complete
		wg.Wait()

		// Check for errors
		mu.Lock()
		err := firstErr
		mu.Unlock()

		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch feed statistics"})
		}

		// Create response in expected format
		stats := DetailedFeedStatsSummary{
			FeedAmount:             feedAmount{Amount: feedCount},
			ArticleAmount:          articleAmount{Amount: totalArticlesCount},
			UnsummarizedFeedAmount: unsummarizedFeedAmount{Amount: unsummarizedCount},
		}

		logger.Logger.InfoContext(ctx, "Detailed feed stats retrieved successfully",
			"feed_count", feedCount,
			"total_articles_count", totalArticlesCount,
			"unsummarized_count", unsummarizedCount)

		return c.JSON(http.StatusOK, stats)
	}
}

func RestHandleFetchFeedTags(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		// Parse request body
		var req FeedTagsPayload
		if err := c.Bind(&req); err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to bind request body", "error", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request format"})
		}

		// Parse and validate the article url
		parsedArticleURL, err := url.Parse(req.FeedURL)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Invalid the url format", "error", err.Error(), "article_url", req.FeedURL)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid article url format"})
		}

		// Apply URL security validation (same as other endpoints)
		err = IsAllowedURL(parsedArticleURL)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Article URL not allowed", "error", err, "article_url", req.FeedURL)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Article URL not allowed for security reasons"})
		}

		// Set default limit if not provided
		limit := 20 // Default limit
		if req.Limit > 0 {
			limit = req.Limit
		}

		// Parse cursor if provided (same pattern as existing cursor endpoints)
		var cursor *time.Time
		if req.Cursor != "" {
			parsedCursor, err := time.Parse(time.RFC3339, req.Cursor)
			if err != nil {
				logger.Logger.ErrorContext(ctx, "Invalid cursor parameter", "error", err, "cursor", req.Cursor)
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid cursor format. Use RFC3339 format"})
			}
			cursor = &parsedCursor
		}

		// Add caching headers (tags update infrequently)
		c.Response().Header().Set("Cache-Control", "public, max-age=3600") // 1 hour

		logger.Logger.InfoContext(ctx, "Fetching feed tags", "feed_url", req.FeedURL, "cursor", cursor, "limit", limit)
		tags, err := container.FetchFeedTagsUsecase.Execute(ctx, req.FeedURL, cursor, limit)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Error fetching feed tags", "error", err, "feed_url", req.FeedURL, "limit", limit)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch feed tags"})
		}

		// Convert domain tags to response format
		tagResponses := make([]FeedTagResponse, len(tags))
		for i, tag := range tags {
			tagResponses[i] = FeedTagResponse{
				ID:        tag.ID,
				Name:      tag.TagName,
				CreatedAt: tag.CreatedAt.Format(time.RFC3339),
			}
		}

		// Create response with next_cursor for pagination (same pattern as other cursor endpoints)
		response := map[string]interface{}{
			"feed_url": req.FeedURL,
			"tags":     tagResponses,
		}

		// Add next cursor if there are results
		if len(tags) > 0 {
			lastTag := tags[len(tags)-1]
			response["next_cursor"] = lastTag.CreatedAt.Format(time.RFC3339)
		}

		logger.Logger.InfoContext(ctx, "Feed tags retrieved successfully", "feed_url", req.FeedURL, "count", len(tags))
		return c.JSON(http.StatusOK, response)
	}
}
