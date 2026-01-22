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

		if req.ArticleID != "" && req.Content == "" {
			article, err := container.AltDBRepository.FetchArticleByID(ctx, req.ArticleID)
			if err != nil {
				return handleError(c, err, "fetch_article_by_id")
			}
			if article != nil {
				logger.Logger.InfoContext(ctx, "Fetched article content from DB", "article_id", req.ArticleID, "content_length", len(article.Content))
				req.Content = article.Content
				if req.Title == "" {
					req.Title = article.Title
				}
			} else {
				logger.Logger.WarnContext(ctx, "Article ID provided but not found in DB", "article_id", req.ArticleID)
			}
		}

		if req.ArticleID == "" {
			if req.FeedURL == "" {
				return handleValidationError(c, "feed_url or article_id is required", "feed_url", "empty")
			}

			existingArticle, err := container.AltDBRepository.FetchArticleByURL(ctx, req.FeedURL)
			if err != nil {
				return handleError(c, err, "fetch_article_by_url")
			}

			if existingArticle != nil {
				req.ArticleID = existingArticle.ID
				if req.Title == "" {
					req.Title = existingArticle.Title
				}
				if req.Content == "" {
					req.Content = existingArticle.Content
				}
			} else if req.Content != "" {
				if req.Title == "" {
					req.Title = "No Title"
				}
				id, err := container.AltDBRepository.SaveArticle(ctx, req.FeedURL, req.Title, req.Content)
				if err != nil {
					return handleError(c, err, "save_article")
				}
				req.ArticleID = id
			} else {
				content, _, title, err := fetchArticleContent(ctx, req.FeedURL, container)
				if err != nil {
					return handleError(c, err, "fetch_article_content")
				}
				id, err := container.AltDBRepository.SaveArticle(ctx, req.FeedURL, title, content)
				if err != nil {
					return handleError(c, err, "save_article")
				}
				req.ArticleID = id
				req.Title = title
				req.Content = content
			}
		}

		if req.Content == "" {
			logger.Logger.WarnContext(ctx, "Empty content provided for streaming", "article_id", req.ArticleID, "feed_url", req.FeedURL)
			return handleValidationError(c, "Content cannot be empty for streaming", "content", "empty")
		}

		existingSummary, err := container.AltDBRepository.FetchArticleSummaryByArticleID(ctx, req.ArticleID)
		if err == nil && existingSummary != nil && existingSummary.Summary != "" {
			logger.Logger.InfoContext(ctx, "Found existing summary in database for streaming", "article_id", req.ArticleID)
			return streamCachedSummary(ctx, c, existingSummary.Summary, req.ArticleID)
		}

		logger.Logger.InfoContext(ctx, "Starting stream summarization", "article_id", req.ArticleID, "content_length", len(req.Content))

		stream, err := streamPreProcessorSummarize(ctx, req.Content, req.ArticleID, req.Title, cfg.PreProcessor.URL)
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
			if err := container.AltDBRepository.SaveArticleSummary(context.Background(), req.ArticleID, userCtx.UserID.String(), req.Title, summary); err != nil {
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

	responseBuf := make([]byte, 128)
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
