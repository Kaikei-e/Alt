package summarization

import (
	"alt/config"
	"alt/di"
	"alt/domain"
	"alt/utils/logger"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// RestHandleSummarizeFeedStream streams a summary from the pre-processor via SSE.
func RestHandleSummarizeFeedStream(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		startTime := time.Now()
		var req FeedSummarizePayload
		ctx := c.Request().Context()
		if err := c.Bind(&req); err != nil {
			logger.Logger.WarnContext(ctx, "Failed to bind request body for stream summarization", "error", err)
			return handleValidationError(c, "Invalid request format", "body", "malformed JSON")
		}

		// Get user context for saving summary
		userCtx, err := domain.GetUserFromContext(ctx)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to get user context for stream summarization", "error", err)
			return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
		}

		logger.Logger.InfoContext(ctx, "Stream summarization request received", "article_id", req.ArticleID, "feed_url", req.FeedURL, "has_content", req.Content != "", "content_length", len(req.Content))

		if req.ArticleID == "" && req.FeedURL == "" {
			return handleValidationError(c, "feed_url or article_id is required", "feed_url", "empty")
		}

		articleID, title, content, err := container.SummarizeArticleUsecase.ResolveStreamArticle(ctx, req.ArticleID, req.FeedURL, req.Title, req.Content)
		if err != nil {
			return handleError(c, err, "resolve_stream_article")
		}
		req.ArticleID, req.Title, req.Content = articleID, title, content

		if req.Content == "" {
			logger.Logger.WarnContext(ctx, "Empty content provided for streaming", "article_id", req.ArticleID, "feed_url", req.FeedURL)
			return handleValidationError(c, "Content cannot be empty for streaming", "content", "empty")
		}

		if !req.ForceRefresh {
			if cached, ok := container.SummarizeArticleUsecase.GetCachedSummary(ctx, req.ArticleID); ok {
				logger.Logger.InfoContext(ctx, "Found existing summary in database for streaming", "article_id", req.ArticleID)
				return streamCachedSummary(ctx, c, cached, req.ArticleID)
			}
		} else {
			logger.Logger.InfoContext(ctx, "Force refresh: skipping summary cache", "article_id", req.ArticleID)
		}

		logger.Logger.InfoContext(ctx, "Starting stream summarization", "article_id", req.ArticleID, "content_length", len(req.Content))

		stream, err := container.SummarizeArticleUsecase.StreamSummary(ctx, req.Content, req.ArticleID, req.Title)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to start stream summarization", "error", err, "article_id", req.ArticleID)
			return handleError(c, err, "summarize_feed_stream")
		}
		defer func() {
			if closeErr := stream.Close(); closeErr != nil {
				logger.Logger.DebugContext(ctx, "Failed to close stream", "error", closeErr)
			}
		}()

		logger.Logger.InfoContext(ctx, "Stream obtained from pre-processor", "article_id", req.ArticleID)
		setStreamingHeaders(c)

		summary, err := streamAndCapture(ctx, c, req.ArticleID, stream)
		if err != nil {
			return err
		}

		duration := time.Since(startTime)
		if summary != "" && req.ArticleID != "" {
			saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			if err := container.SummarizeArticleUsecase.SaveStreamedSummary(saveCtx, req.ArticleID, userCtx.UserID.String(), req.Title, summary); err != nil {
				logger.Logger.ErrorContext(ctx, "Failed to save streamed summary to database", "error", err, "article_id", req.ArticleID)
			} else {
				logger.Logger.InfoContext(ctx, "Streamed summary saved to database", "article_id", req.ArticleID, "summary_length", len(summary))
			}
		}

		logger.Logger.InfoContext(ctx, "Stream summarization request completed", "article_id", req.ArticleID, "total_duration_ms", duration.Milliseconds())
		return nil
	}
}

func setStreamingHeaders(c echo.Context) {
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream; charset=utf-8")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().Header().Set(echo.HeaderConnection, "keep-alive")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)
}

func streamCachedSummary(ctx context.Context, c echo.Context, summary, articleID string) error {
	setStreamingHeaders(c)

	cleanSummary := parseSSESummary(summary)
	jsonSummary, err := json.Marshal(cleanSummary)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to marshal existing summary", "error", err)
		return err
	}

	if _, err := fmt.Fprintf(c.Response().Writer, "data: %s\n\n", jsonSummary); err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to write existing summary to stream", "error", err)
		return err
	}
	c.Response().Flush()

	logger.Logger.InfoContext(ctx, "Existing summary streamed from cache", "article_id", articleID)
	return nil
}

func streamAndCapture(ctx context.Context, c echo.Context, articleID string, stream io.Reader) (string, error) {
	var buf bytes.Buffer
	tee := io.TeeReader(stream, &buf)

	// 4KB buffer for reduced syscall overhead (TTFT optimization)
	responseBuf := make([]byte, 4096)
	bytesWritten := 0
	hasData := false
	readAttempts := 0

	for {
		readAttempts++
		n, err := tee.Read(responseBuf)
		if n > 0 {
			hasData = true
			bytesWritten += n
			if readAttempts <= 3 || bytesWritten%5120 == 0 {
				logger.Logger.InfoContext(ctx, "Stream data received and flushed", "article_id", articleID, "bytes_written", bytesWritten, "chunk_size", n, "read_attempts", readAttempts)
			} else if readAttempts <= 10 {
				logger.Logger.DebugContext(ctx, "Stream chunk flushed", "article_id", articleID, "chunk_size", n, "read_attempts", readAttempts)
			}

			if _, wErr := c.Response().Writer.Write(responseBuf[:n]); wErr != nil {
				logger.Logger.ErrorContext(ctx, "Failed to write to response stream", "error", wErr, "article_id", articleID, "bytes_written", bytesWritten)
				return "", wErr
			}
			c.Response().Flush()
		}

		if err != nil {
			if err == io.EOF {
				logger.Logger.InfoContext(ctx, "Stream reached EOF", "article_id", articleID, "bytes_written", bytesWritten, "read_attempts", readAttempts)
				break
			}
			logger.Logger.ErrorContext(ctx, "Failed to read from stream", "error", err, "article_id", articleID, "bytes_written", bytesWritten, "read_attempts", readAttempts)
			return "", err
		}
		if n == 0 && readAttempts > 1 {
			logger.Logger.WarnContext(ctx, "No data read from stream", "article_id", articleID, "read_attempts", readAttempts)
		}
	}

	if hasData {
		logger.Logger.InfoContext(ctx, "Stream completed successfully", "article_id", articleID, "bytes_written", bytesWritten, "read_attempts", readAttempts)
	} else {
		logger.Logger.WarnContext(ctx, "Stream completed but no data was sent", "article_id", articleID, "read_attempts", readAttempts)
	}

	return parseSSESummary(buf.String()), nil
}
