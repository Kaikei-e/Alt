package rest_feeds

import (
	"alt/config"
	"alt/di"
	"alt/utils/logger"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v4"
)

// FeedSummarizePayload represents the request body for feed summarization
type FeedSummarizePayload struct {
	FeedURL   string `json:"feed_url"`
	ArticleID string `json:"article_id"`
	Content   string `json:"content"`
	Title     string `json:"title"`
}

// handleSummarizeFeed handles article summarization requests by proxying to pre-processor
func RestHandleSummarizeFeed(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Parse request
		var req struct {
			FeedURL string `json:"feed_url" validate:"required"`
		}

		if err := c.Bind(&req); err != nil {
			logger.Logger.Error("Failed to bind summarize request", "error", err)
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid request format")
		}

		// Validate feed URL
		if req.FeedURL == "" {
			logger.Logger.Warn("Empty feed_url provided for summarization")
			return echo.NewHTTPError(http.StatusBadRequest, "feed_url is required")
		}

		// Validate URL format
		if _, err := url.Parse(req.FeedURL); err != nil {
			logger.Logger.Error("Invalid feed_url format", "error", err, "url", req.FeedURL)
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid feed_url format")
		}

		logger.Logger.Info("Processing summarization request", "feed_url", req.FeedURL)

		// Step 1: Check if article exists in DB
		var articleID string
		var articleTitle string
		// articleContent is not needed for summarization request as we pull from DB

		existingArticle, err := container.AltDBRepository.FetchArticleByURL(c.Request().Context(), req.FeedURL)
		if err != nil {
			logger.Logger.Error("Failed to check for existing article", "error", err, "url", req.FeedURL)
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to check article existence")
		}

		if existingArticle != nil {
			// Article exists in DB
			logger.Logger.Info("Article found in database", "article_id", existingArticle.ID, "url", req.FeedURL)
			articleID = existingArticle.ID
			articleTitle = existingArticle.Title
		} else {
			// Article does not exist, fetch from Web
			logger.Logger.Info("Article not found in database, fetching from Web", "url", req.FeedURL)
			fetchedContent, _, fetchedTitle, fetchErr := FetchArticleContent(c.Request().Context(), req.FeedURL, container)
			if fetchErr != nil {
				logger.Logger.Error("Failed to fetch article content", "error", fetchErr, "url", req.FeedURL)
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch article content")
			}

			var saveErr error
			articleID, saveErr = container.AltDBRepository.SaveArticle(c.Request().Context(), req.FeedURL, fetchedTitle, fetchedContent)
			if saveErr != nil {
				logger.Logger.Error("Failed to save article to database", "error", saveErr, "url", req.FeedURL)
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to save article")
			}
			articleTitle = fetchedTitle
		}

		// Step 2: Try to fetch existing summary from database
		var summary string
		existingSummary, err := container.AltDBRepository.FetchArticleSummaryByArticleID(c.Request().Context(), articleID)
		if err == nil && existingSummary != nil && existingSummary.Summary != "" {
			logger.Logger.Info("Found existing summary in database", "article_id", articleID, "feed_url", req.FeedURL)
			summary = existingSummary.Summary
		} else {
			// Step 3: Generate new summary if not found in database
			logger.Logger.Info("No existing summary found, generating new summary", "article_id", articleID, "feed_url", req.FeedURL)

			// Small delay to ensure DB transaction is committed before pre-processor reads
			// This is necessary because SaveArticle uses QueryRow which may not be immediately visible
			time.Sleep(100 * time.Millisecond)

			// Call pre-processor with empty content (it will fetch from DB)
			summary, err = CallPreProcessorSummarize(c.Request().Context(), "", articleID, articleTitle, cfg.PreProcessor.URL)
			if err != nil {
				logger.Logger.Error("Failed to summarize article", "error", err, "url", req.FeedURL, "article_id", articleID)
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to generate summary")
			}

			// Step 4: Save the generated summary to database
			if err := container.AltDBRepository.SaveArticleSummary(c.Request().Context(), articleID, articleTitle, summary); err != nil {
				logger.Logger.Error("Failed to save article summary to database", "error", err, "article_id", articleID, "feed_url", req.FeedURL)
				// Continue even if save fails - we still have the summary to return
			} else {
				logger.Logger.Info("Article summary saved to database", "article_id", articleID, "feed_url", req.FeedURL)
			}
		}

		logger.Logger.Info("Article summarized successfully", "feed_url", req.FeedURL, "from_cache", existingSummary != nil)

		// Step 4: Return response
		response := map[string]interface{}{
			"success":    true,
			"summary":    summary,
			"article_id": articleID,
			"feed_url":   req.FeedURL,
		}

		return c.JSON(http.StatusOK, response)
	}
}

// handleSummarizeFeedQueue handles async article summarization requests by queueing to pre-processor
func RestHandleSummarizeFeedQueue(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Parse request
		var req struct {
			FeedURL string `json:"feed_url" validate:"required"`
		}

		if err := c.Bind(&req); err != nil {
			logger.Logger.Error("Failed to bind summarize queue request", "error", err)
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid request format")
		}

		// Validate feed URL
		if req.FeedURL == "" {
			logger.Logger.Warn("Empty feed_url provided for summarization")
			return echo.NewHTTPError(http.StatusBadRequest, "feed_url is required")
		}

		// Validate URL format
		if _, err := url.Parse(req.FeedURL); err != nil {
			logger.Logger.Error("Invalid feed_url format", "error", err, "url", req.FeedURL)
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid feed_url format")
		}

		logger.Logger.Info("Queueing summarization request", "feed_url", req.FeedURL)

		// Step 1: Check if article exists in DB
		var articleID string
		var articleTitle string

		existingArticle, err := container.AltDBRepository.FetchArticleByURL(c.Request().Context(), req.FeedURL)
		if err != nil {
			logger.Logger.Error("Failed to check for existing article", "error", err, "url", req.FeedURL)
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to check article existence")
		}

		if existingArticle != nil {
			// Article exists in DB
			logger.Logger.Info("Article found in database", "article_id", existingArticle.ID, "url", req.FeedURL)
			articleID = existingArticle.ID
			articleTitle = existingArticle.Title
		} else {
			// Article does not exist, fetch from Web
			logger.Logger.Info("Article not found in database, fetching from Web", "url", req.FeedURL)
			fetchedContent, _, fetchedTitle, fetchErr := FetchArticleContent(c.Request().Context(), req.FeedURL, container)
			if fetchErr != nil {
				logger.Logger.Error("Failed to fetch article content", "error", fetchErr, "url", req.FeedURL)
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch article content")
			}

			var saveErr error
			articleID, saveErr = container.AltDBRepository.SaveArticle(c.Request().Context(), req.FeedURL, fetchedTitle, fetchedContent)
			if saveErr != nil {
				logger.Logger.Error("Failed to save article to database", "error", saveErr, "url", req.FeedURL)
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to save article")
			}
			articleTitle = fetchedTitle
		}

		// Step 2: Check if summary already exists
		existingSummary, err := container.AltDBRepository.FetchArticleSummaryByArticleID(c.Request().Context(), articleID)
		if err == nil && existingSummary != nil && existingSummary.Summary != "" {
			logger.Logger.Info("Found existing summary in database", "article_id", articleID, "feed_url", req.FeedURL)
			// Return existing summary immediately
			response := map[string]interface{}{
				"success":    true,
				"summary":    existingSummary.Summary,
				"article_id": articleID,
				"feed_url":   req.FeedURL,
			}
			return c.JSON(http.StatusOK, response)
		}

		// Step 3: Queue summarization job
		logger.Logger.Info("No existing summary found, queueing summarization job", "article_id", articleID, "feed_url", req.FeedURL)

		// Small delay to ensure DB transaction is committed before pre-processor reads
		time.Sleep(100 * time.Millisecond)

		// Call pre-processor queue endpoint
		jobID, err := CallPreProcessorSummarizeQueue(c.Request().Context(), articleID, articleTitle, cfg.PreProcessor.URL)
		if err != nil {
			logger.Logger.Error("Failed to queue summarization job", "error", err, "url", req.FeedURL, "article_id", articleID)
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to queue summarization job")
		}

		logger.Logger.Info("Summarization job queued successfully", "job_id", jobID, "article_id", articleID, "feed_url", req.FeedURL)

		// Return 202 Accepted with job ID and status URL
		response := map[string]interface{}{
			"job_id":     jobID,
			"status":     "pending",
			"status_url": fmt.Sprintf("/v1/feeds/summarize/status/%s", jobID),
			"article_id": articleID,
			"feed_url":   req.FeedURL,
		}

		return c.JSON(http.StatusAccepted, response)
	}
}

// handleSummarizeFeedStream handles streaming article summarization
func RestHandleSummarizeFeedStream(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		startTime := time.Now()
		var req FeedSummarizePayload
		if err := c.Bind(&req); err != nil {
			logger.Logger.Warn("Failed to bind request body for stream summarization", "error", err)
			return HandleValidationError(c, "Invalid request format", "body", "malformed JSON")
		}

		ctx := c.Request().Context()
		logger.Logger.Info("Stream summarization request received", "article_id", req.ArticleID, "feed_url", req.FeedURL, "has_content", req.Content != "", "content_length", len(req.Content))

		// If ArticleID is provided but Content is empty, try to fetch content from DB
		if req.ArticleID != "" && req.Content == "" {
			article, err := container.AltDBRepository.FetchArticleByID(ctx, req.ArticleID)
			if err != nil {
				return HandleError(c, err, "fetch_article_by_id")
			}
			if article != nil {
				logger.Logger.Info("Fetched article content from DB", "article_id", req.ArticleID, "content_length", len(article.Content))
				req.Content = article.Content
				if req.Title == "" {
					req.Title = article.Title
				}
			} else {
				logger.Logger.Warn("Article ID provided but not found in DB", "article_id", req.ArticleID)
			}
		}

		// Validate article_id or ensure article exists
		if req.ArticleID == "" {
			if req.FeedURL == "" {
				return HandleValidationError(c, "feed_url or article_id is required", "feed_url", "empty")
			}

			// Check if article exists in DB
			existingArticle, err := container.AltDBRepository.FetchArticleByURL(ctx, req.FeedURL)
			if err != nil {
				// Don't fail hard on DB check failure? Or should we?
				// Better to log and maybe proceed with stream only?
				// But we want persistence.
				return HandleError(c, err, "fetch_article_by_url")
			}

			if existingArticle != nil {
				req.ArticleID = existingArticle.ID
				if req.Title == "" {
					req.Title = existingArticle.Title
				}
			} else {
				// Article not in DB, save it.
				if req.Content != "" {
					// Use on-the-fly content
					if req.Title == "" {
						req.Title = "No Title"
					}
					id, err := container.AltDBRepository.SaveArticle(ctx, req.FeedURL, req.Title, req.Content)
					if err != nil {
						return HandleError(c, err, "save_article")
					}
					req.ArticleID = id
				} else {
					// Fetch content from Web
					content, _, title, err := FetchArticleContent(ctx, req.FeedURL, container)
					if err != nil {
						return HandleError(c, err, "fetch_article_content")
					}
					id, err := container.AltDBRepository.SaveArticle(ctx, req.FeedURL, title, content)
					if err != nil {
						return HandleError(c, err, "save_article")
					}
					req.ArticleID = id
					req.Title = title
					req.Content = content // Use fetched content for streaming
				}
			}
		}

		// Validate content before streaming
		if req.Content == "" {
			logger.Logger.Warn("Empty content provided for streaming", "article_id", req.ArticleID, "feed_url", req.FeedURL)
			return HandleValidationError(c, "Content cannot be empty for streaming", "content", "empty")
		}

		logger.Logger.Info("Starting stream summarization", "article_id", req.ArticleID, "content_length", len(req.Content))

		// Call streaming internal utility
		stream, err := StreamPreProcessorSummarize(ctx, req.Content, req.ArticleID, req.Title, cfg.PreProcessor.URL)
		if err != nil {
			logger.Logger.Error("Failed to start stream summarization", "error", err, "article_id", req.ArticleID)
			return HandleError(c, err, "summarize_feed_stream")
		}
		defer stream.Close()

		logger.Logger.Info("Stream obtained from pre-processor", "article_id", req.ArticleID)

		// Set headers for SSE streaming
		c.Response().Header().Set(echo.HeaderContentType, "text/event-stream; charset=utf-8")
		c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
		c.Response().Header().Set(echo.HeaderConnection, "keep-alive")
		c.Response().Header().Set("X-Accel-Buffering", "no") // Disable Nginx buffering for SSE
		c.Response().WriteHeader(http.StatusOK)

		logger.Logger.Info("Response headers set, starting to read stream", "article_id", req.ArticleID)

		// Create buffer to capture output for persistence
		var buf bytes.Buffer
		tee := io.TeeReader(stream, &buf)

		// Stream response with smaller buffer for incremental rendering
		responseBuf := make([]byte, 128) // Reduced to 128 bytes for faster incremental rendering
		bytesWritten := 0
		hasData := false
		readAttempts := 0
		for {
			readAttempts++
			n, err := tee.Read(responseBuf)
			if n > 0 {
				hasData = true
				bytesWritten += n
				// Log first few chunks and periodically for debugging incremental rendering
				if readAttempts <= 3 || bytesWritten%5120 == 0 {
					logger.Logger.Info("Stream data received and flushed", "article_id", req.ArticleID, "bytes_written", bytesWritten, "chunk_size", n, "read_attempts", readAttempts)
				} else if readAttempts <= 10 {
					// Log more frequently for first 10 chunks to verify incremental rendering
					logger.Logger.Debug("Stream chunk flushed", "article_id", req.ArticleID, "chunk_size", n, "read_attempts", readAttempts)
				}
				// Write immediately and flush for incremental rendering
				if _, wErr := c.Response().Writer.Write(responseBuf[:n]); wErr != nil {
					logger.Logger.Error("Failed to write to response stream", "error", wErr, "article_id", req.ArticleID, "bytes_written", bytesWritten)
					return wErr
				}
				// Flush immediately after each chunk for incremental rendering
				c.Response().Flush()
			}
			if err != nil {
				if err == io.EOF {
					logger.Logger.Info("Stream reached EOF", "article_id", req.ArticleID, "bytes_written", bytesWritten, "read_attempts", readAttempts)
					break
				}
				logger.Logger.Error("Failed to read from stream", "error", err, "article_id", req.ArticleID, "bytes_written", bytesWritten, "read_attempts", readAttempts)
				return err
			}
			if n == 0 && readAttempts > 1 {
				// No data read but no error - might be a timeout or empty stream
				logger.Logger.Warn("No data read from stream", "article_id", req.ArticleID, "read_attempts", readAttempts)
			}
		}

		// Check if any data was actually streamed
		duration := time.Since(startTime)
		if !hasData {
			logger.Logger.Warn("Stream completed but no data was sent", "article_id", req.ArticleID, "read_attempts", readAttempts, "duration_ms", duration.Milliseconds())
			// Still return success, but log the warning
		} else {
			logger.Logger.Info("Stream completed successfully", "article_id", req.ArticleID, "bytes_written", bytesWritten, "read_attempts", readAttempts, "duration_ms", duration.Milliseconds())
		}

		// Save complete summary to DB (Persistence consistency)
		summary := buf.String()
		if summary != "" && req.ArticleID != "" {
			// Use background context for saving to ensure it completes
			// (Use a detached context if available, or just Background)
			if err := container.AltDBRepository.SaveArticleSummary(context.Background(), req.ArticleID, req.Title, summary); err != nil {
				logger.Logger.Error("Failed to save streamed summary to database", "error", err, "article_id", req.ArticleID)
			} else {
				logger.Logger.Info("Streamed summary saved to database", "article_id", req.ArticleID, "summary_length", len(summary))
			}
		}

		logger.Logger.Info("Stream summarization request completed", "article_id", req.ArticleID, "total_duration_ms", duration.Milliseconds(), "bytes_written", bytesWritten)
		return nil
	}
}

func RestHandleSummarizeFeedStatus(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		jobID := c.Param("job_id")
		if jobID == "" {
			logger.Logger.Warn("Empty job_id provided")
			return echo.NewHTTPError(http.StatusBadRequest, "job_id is required")
		}

		logger.Logger.Debug("Checking summarization job status", "job_id", jobID)

		// Call pre-processor status endpoint
		status, err := CallPreProcessorSummarizeStatus(c.Request().Context(), jobID, cfg.PreProcessor.URL)
		if err != nil {
			logger.Logger.Error("Failed to get summarization job status", "error", err, "job_id", jobID)
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get job status")
		}

		// If job not found, return 404
		if status == nil {
			return echo.NewHTTPError(http.StatusNotFound, "Job not found")
		}

		response := map[string]interface{}{
			"job_id":     status.JobID,
			"status":     status.Status,
			"article_id": status.ArticleID,
		}

		// Include summary if completed
		if status.Status == "completed" && status.Summary != "" {
			response["summary"] = status.Summary
		}

		// Include error message if failed
		if status.Status == "failed" && status.ErrorMessage != "" {
			response["error_message"] = status.ErrorMessage
		}

		return c.JSON(http.StatusOK, response)
	}
}
