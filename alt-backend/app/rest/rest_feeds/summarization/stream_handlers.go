package summarization

import (
	"alt/config"
	"alt/di"
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
		if err := c.Bind(&req); err != nil {
			logger.Logger.Warn("Failed to bind request body for stream summarization", "error", err)
			return handleValidationError(c, "Invalid request format", "body", "malformed JSON")
		}

		ctx := c.Request().Context()
		logger.Logger.Info("Stream summarization request received", "article_id", req.ArticleID, "feed_url", req.FeedURL, "has_content", req.Content != "", "content_length", len(req.Content))

		if req.ArticleID != "" && req.Content == "" {
			article, err := container.AltDBRepository.FetchArticleByID(ctx, req.ArticleID)
			if err != nil {
				return handleError(c, err, "fetch_article_by_id")
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
			logger.Logger.Warn("Empty content provided for streaming", "article_id", req.ArticleID, "feed_url", req.FeedURL)
			return handleValidationError(c, "Content cannot be empty for streaming", "content", "empty")
		}

		existingSummary, err := container.AltDBRepository.FetchArticleSummaryByArticleID(ctx, req.ArticleID)
		if err == nil && existingSummary != nil && existingSummary.Summary != "" {
			logger.Logger.Info("Found existing summary in database for streaming", "article_id", req.ArticleID)
			return streamCachedSummary(c, existingSummary.Summary, req.ArticleID)
		}

		logger.Logger.Info("Starting stream summarization", "article_id", req.ArticleID, "content_length", len(req.Content))

		stream, err := streamPreProcessorSummarize(ctx, req.Content, req.ArticleID, req.Title, cfg.PreProcessor.URL)
		if err != nil {
			logger.Logger.Error("Failed to start stream summarization", "error", err, "article_id", req.ArticleID)
			return handleError(c, err, "summarize_feed_stream")
		}
		defer stream.Close()

		logger.Logger.Info("Stream obtained from pre-processor", "article_id", req.ArticleID)
		setStreamingHeaders(c)

		summary, err := streamAndCapture(c, req.ArticleID, stream)
		if err != nil {
			return err
		}

		duration := time.Since(startTime)
		if summary != "" && req.ArticleID != "" {
			if err := container.AltDBRepository.SaveArticleSummary(context.Background(), req.ArticleID, req.Title, summary); err != nil {
				logger.Logger.Error("Failed to save streamed summary to database", "error", err, "article_id", req.ArticleID)
			} else {
				logger.Logger.Info("Streamed summary saved to database", "article_id", req.ArticleID, "summary_length", len(summary))
			}
		}

		logger.Logger.Info("Stream summarization request completed", "article_id", req.ArticleID, "total_duration_ms", duration.Milliseconds())
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

func streamCachedSummary(c echo.Context, summary, articleID string) error {
	setStreamingHeaders(c)

	cleanSummary := parseSSESummary(summary)
	jsonSummary, err := json.Marshal(cleanSummary)
	if err != nil {
		logger.Logger.Error("Failed to marshal existing summary", "error", err)
		return err
	}

	if _, err := fmt.Fprintf(c.Response().Writer, "data: %s\n\n", jsonSummary); err != nil {
		logger.Logger.Error("Failed to write existing summary to stream", "error", err)
		return err
	}
	c.Response().Flush()

	logger.Logger.Info("Existing summary streamed from cache", "article_id", articleID)
	return nil
}

func streamAndCapture(c echo.Context, articleID string, stream io.Reader) (string, error) {
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
				logger.Logger.Info("Stream data received and flushed", "article_id", articleID, "bytes_written", bytesWritten, "chunk_size", n, "read_attempts", readAttempts)
			} else if readAttempts <= 10 {
				logger.Logger.Debug("Stream chunk flushed", "article_id", articleID, "chunk_size", n, "read_attempts", readAttempts)
			}

			if _, wErr := c.Response().Writer.Write(responseBuf[:n]); wErr != nil {
				logger.Logger.Error("Failed to write to response stream", "error", wErr, "article_id", articleID, "bytes_written", bytesWritten)
				return "", wErr
			}
			c.Response().Flush()
		}

		if err != nil {
			if err == io.EOF {
				logger.Logger.Info("Stream reached EOF", "article_id", articleID, "bytes_written", bytesWritten, "read_attempts", readAttempts)
				break
			}
			logger.Logger.Error("Failed to read from stream", "error", err, "article_id", articleID, "bytes_written", bytesWritten, "read_attempts", readAttempts)
			return "", err
		}
		if n == 0 && readAttempts > 1 {
			logger.Logger.Warn("No data read from stream", "article_id", articleID, "read_attempts", readAttempts)
		}
	}

	if hasData {
		logger.Logger.Info("Stream completed successfully", "article_id", articleID, "bytes_written", bytesWritten, "read_attempts", readAttempts)
	} else {
		logger.Logger.Warn("Stream completed but no data was sent", "article_id", articleID, "read_attempts", readAttempts)
	}

	return parseSSESummary(buf.String()), nil
}
