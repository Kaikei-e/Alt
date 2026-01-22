package summarization

import (
	"alt/config"
	"alt/di"
	"alt/domain"
	"alt/utils/logger"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v4"
)

// RestHandleSummarizeFeed proxies a request to the pre-processor and persists the result.
func RestHandleSummarizeFeed(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()

		// Get user context for saving summary
		userCtx, err := domain.GetUserFromContext(ctx)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to get user context for summarization", "error", err)
			return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
		}

		var req struct {
			FeedURL string `json:"feed_url" validate:"required"`
		}

		if err := c.Bind(&req); err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to bind summarize request", "error", err)
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid request format")
		}

		if req.FeedURL == "" {
			logger.Logger.WarnContext(ctx, "Empty feed_url provided for summarization")
			return echo.NewHTTPError(http.StatusBadRequest, "feed_url is required")
		}

		if _, err := url.Parse(req.FeedURL); err != nil {
			logger.Logger.ErrorContext(ctx, "Invalid feed_url format", "error", err, "url", req.FeedURL)
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid feed_url format")
		}

		logger.Logger.InfoContext(ctx, "Processing summarization request", "feed_url", req.FeedURL)

		articleID, articleTitle, existed, err := ensureArticleRecord(ctx, container, req.FeedURL)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to resolve article before summarization", "error", err, "url", req.FeedURL)
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to check article existence")
		}

		if existed {
			logger.Logger.InfoContext(ctx, "Article found in database", "article_id", articleID, "url", req.FeedURL)
		} else {
			logger.Logger.InfoContext(ctx, "Article not found in database, fetched and saved", "article_id", articleID, "url", req.FeedURL)
		}

		var summary string
		existingSummary, err := container.AltDBRepository.FetchArticleSummaryByArticleID(ctx, articleID)
		cachedSummary := err == nil && existingSummary != nil && existingSummary.Summary != ""

		if cachedSummary {
			logger.Logger.InfoContext(ctx, "Found existing summary in database", "article_id", articleID, "feed_url", req.FeedURL)
			summary = parseSSESummary(existingSummary.Summary)
		} else {
			logger.Logger.InfoContext(ctx, "No existing summary found, generating new summary", "article_id", articleID, "feed_url", req.FeedURL)
			time.Sleep(100 * time.Millisecond)
			summary, err = callPreProcessorSummarize(ctx, "", articleID, articleTitle, cfg.PreProcessor.URL)
			if err != nil {
				logger.Logger.ErrorContext(ctx, "Failed to summarize article", "error", err, "url", req.FeedURL, "article_id", articleID)
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to generate summary")
			}

			if err := container.AltDBRepository.SaveArticleSummary(ctx, articleID, userCtx.UserID.String(), articleTitle, summary); err != nil {
				logger.Logger.ErrorContext(ctx, "Failed to save article summary to database", "error", err, "article_id", articleID, "feed_url", req.FeedURL)
			} else {
				logger.Logger.InfoContext(ctx, "Article summary saved to database", "article_id", articleID, "feed_url", req.FeedURL)
			}
		}

		logger.Logger.InfoContext(ctx, "Article summarized successfully", "feed_url", req.FeedURL, "from_cache", cachedSummary)

		return c.JSON(http.StatusOK, map[string]interface{}{
			"success":    true,
			"summary":    summary,
			"article_id": articleID,
			"feed_url":   req.FeedURL,
		})
	}
}

// RestHandleSummarizeFeedQueue enqueues a summarization job when no cached summary exists.
func RestHandleSummarizeFeedQueue(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		var req struct {
			FeedURL string `json:"feed_url" validate:"required"`
		}

		if err := c.Bind(&req); err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to bind summarize queue request", "error", err)
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid request format")
		}

		if req.FeedURL == "" {
			logger.Logger.WarnContext(ctx, "Empty feed_url provided for summarization")
			return echo.NewHTTPError(http.StatusBadRequest, "feed_url is required")
		}

		if _, err := url.Parse(req.FeedURL); err != nil {
			logger.Logger.ErrorContext(ctx, "Invalid feed_url format", "error", err, "url", req.FeedURL)
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid feed_url format")
		}

		logger.Logger.InfoContext(ctx, "Queueing summarization request", "feed_url", req.FeedURL)

		articleID, articleTitle, existed, err := ensureArticleRecord(ctx, container, req.FeedURL)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to resolve article before queueing", "error", err, "url", req.FeedURL)
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to check article existence")
		}

		if existed {
			logger.Logger.InfoContext(ctx, "Article found in database", "article_id", articleID, "url", req.FeedURL)
		} else {
			logger.Logger.InfoContext(ctx, "Article not found in database, fetched and saved", "article_id", articleID, "url", req.FeedURL)
		}

		existingSummary, err := container.AltDBRepository.FetchArticleSummaryByArticleID(ctx, articleID)
		if err == nil && existingSummary != nil && existingSummary.Summary != "" {
			logger.Logger.InfoContext(ctx, "Found existing summary in database", "article_id", articleID, "feed_url", req.FeedURL)
			return respondWithSummary(c, parseSSESummary(existingSummary.Summary), articleID, req.FeedURL)
		}

		logger.Logger.InfoContext(ctx, "No existing summary found, queueing summarization job", "article_id", articleID, "feed_url", req.FeedURL)
		time.Sleep(100 * time.Millisecond)

		jobID, err := callPreProcessorSummarizeQueue(ctx, articleID, articleTitle, cfg.PreProcessor.URL)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to queue summarization job", "error", err, "url", req.FeedURL, "article_id", articleID)
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to queue summarization job")
		}

		logger.Logger.InfoContext(ctx, "Summarization job queued successfully", "job_id", jobID, "article_id", articleID, "feed_url", req.FeedURL)

		return c.JSON(http.StatusAccepted, map[string]interface{}{
			"job_id":     jobID,
			"status":     "pending",
			"status_url": fmt.Sprintf("/v1/feeds/summarize/status/%s", jobID),
			"article_id": articleID,
			"feed_url":   req.FeedURL,
		})
	}
}
