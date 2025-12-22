package handler

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"pre-processor/driver"
	"pre-processor/models"
	"pre-processor/repository"
	"pre-processor/utils/html_parser"

	"github.com/labstack/echo/v4"
)

// SummarizeRequest represents the request body for article summarization
type SummarizeRequest struct {
	Content   string `json:"content"`
	ArticleID string `json:"article_id" validate:"required"`
	Title     string `json:"title"`
}

// SummarizeResponse represents the response for article summarization
type SummarizeResponse struct {
	Success   bool   `json:"success"`
	Summary   string `json:"summary"`
	ArticleID string `json:"article_id"`
}

// SummarizeHandler handles on-demand article summarization requests
type SummarizeHandler struct {
	apiRepo     repository.ExternalAPIRepository
	summaryRepo repository.SummaryRepository
	articleRepo repository.ArticleRepository
	jobRepo     repository.SummarizeJobRepository
	logger      *slog.Logger
}

// NewSummarizeHandler creates a new summarize handler
func NewSummarizeHandler(apiRepo repository.ExternalAPIRepository, summaryRepo repository.SummaryRepository, articleRepo repository.ArticleRepository, jobRepo repository.SummarizeJobRepository, logger *slog.Logger) *SummarizeHandler {
	return &SummarizeHandler{
		apiRepo:     apiRepo,
		summaryRepo: summaryRepo,
		articleRepo: articleRepo,
		jobRepo:     jobRepo,
		logger:      logger,
	}
}

// HandleSummarize handles POST /api/v1/summarize requests
func (h *SummarizeHandler) HandleSummarize(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse request body
	var req SummarizeRequest
	if err := c.Bind(&req); err != nil {
		h.logger.Error("failed to bind request", "error", err)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request format")
	}

	// Validate required fields
	if req.ArticleID == "" {
		h.logger.Warn("empty article_id provided")
		return echo.NewHTTPError(http.StatusBadRequest, "Article ID cannot be empty")
	}

	// If content is empty, try to fetch from DB
	if req.Content == "" {
		h.logger.Info("content is empty, fetching from DB", "article_id", req.ArticleID)
		fetchedArticle, err := h.articleRepo.FindByID(ctx, req.ArticleID)
		if err != nil {
			h.logger.Error("failed to fetch article from DB", "error", err, "article_id", req.ArticleID)
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch article content")
		}
		if fetchedArticle == nil {
			h.logger.Warn("article not found in DB", "article_id", req.ArticleID)
			return echo.NewHTTPError(http.StatusNotFound, "Article not found")
		}
		if fetchedArticle.Content == "" {
			h.logger.Warn("article found but content is empty", "article_id", req.ArticleID)
			return echo.NewHTTPError(http.StatusBadRequest, "Article content is empty in database")
		}
		// Zero Trust: Always extract text from content (HTML or not)
		// This ensures we never send raw HTML to downstream services
		content := fetchedArticle.Content
		originalLength := len(content)
		h.logger.Info("extracting text from content (Zero Trust validation)", "article_id", req.ArticleID, "original_length", originalLength)

		extractedText := html_parser.ExtractArticleText(content)
		extractedLength := len(extractedText)

		if extractedText != "" {
			content = extractedText
			reductionRatio := (1.0 - float64(extractedLength)/float64(originalLength)) * 100.0
			h.logger.Info("text extraction completed",
				"article_id", req.ArticleID,
				"original_length", originalLength,
				"extracted_length", extractedLength,
				"reduction_ratio", fmt.Sprintf("%.2f%%", reductionRatio))
		} else {
			h.logger.Warn("text extraction returned empty, using original content", "article_id", req.ArticleID, "original_length", originalLength)
		}
		req.Content = content
		// Also update title if missing
		if req.Title == "" {
			req.Title = fetchedArticle.Title
		}
		h.logger.Info("content fetched from DB successfully", "article_id", req.ArticleID, "content_length", len(req.Content))
	}

	if req.Content == "" {
		h.logger.Warn("empty content provided and not found in DB", "article_id", req.ArticleID)
		return echo.NewHTTPError(http.StatusBadRequest, "Content cannot be empty")
	}

	// Zero Trust: Ensure content is extracted text before processing
	// If content still contains HTML-like patterns, extract again
	if strings.Contains(req.Content, "<") && strings.Contains(req.Content, ">") {
		h.logger.Warn("content still contains HTML after extraction, re-extracting", "article_id", req.ArticleID, "content_length", len(req.Content))
		req.Content = html_parser.ExtractArticleText(req.Content)
		h.logger.Info("re-extraction completed", "article_id", req.ArticleID, "final_length", len(req.Content))
	}

	h.logger.Info("processing summarization request", "article_id", req.ArticleID, "content_length", len(req.Content))

	// Create article model for summarization
	article := &models.Article{
		ID:      req.ArticleID,
		Content: req.Content,
	}

	// Call summarization service
	summarized, err := h.apiRepo.SummarizeArticle(ctx, article)
	if err != nil {
		h.logger.Error("failed to summarize article", "error", err, "article_id", req.ArticleID)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to generate summary")
	}

	h.logger.Info("article summarized successfully", "article_id", req.ArticleID)

	// Save summary to database
	articleTitle := req.Title
	if articleTitle == "" {
		articleTitle = "Untitled" // Fallback if no title provided
	}

	articleSummary := &models.ArticleSummary{
		ArticleID:       req.ArticleID,
		ArticleTitle:    articleTitle,
		SummaryJapanese: summarized.SummaryJapanese,
	}

	if err := h.summaryRepo.Create(ctx, articleSummary); err != nil {
		h.logger.Error("failed to save summary to database", "error", err, "article_id", req.ArticleID)
		// Don't fail the request if DB save fails - still return the summary
		// This ensures the user gets the summary even if DB has issues
		h.logger.Warn("continuing despite DB save failure", "article_id", req.ArticleID)
	} else {
		h.logger.Info("summary saved to database successfully", "article_id", req.ArticleID)
	}

	// Return response
	response := SummarizeResponse{
		Success:   true,
		Summary:   summarized.SummaryJapanese,
		ArticleID: req.ArticleID,
	}

	return c.JSON(http.StatusOK, response)
}

// SummarizeQueueRequest represents the request body for queueing a summarization job
type SummarizeQueueRequest struct {
	ArticleID string `json:"article_id" validate:"required"`
	Title     string `json:"title"`
}

// SummarizeQueueResponse represents the response for queueing a summarization job
type SummarizeQueueResponse struct {
	JobID   string `json:"job_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// HandleStreamSummarize handles POST /api/v1/summarize/stream requests
func (h *SummarizeHandler) HandleStreamSummarize(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse request body
	var req SummarizeRequest
	if err := c.Bind(&req); err != nil {
		h.logger.Error("failed to bind request", "error", err)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request format")
	}

	// Validate required fields
	if req.ArticleID == "" {
		h.logger.Warn("empty article_id provided")
		return echo.NewHTTPError(http.StatusBadRequest, "Article ID cannot be empty")
	}

	// If content is empty, try to fetch from DB
	if req.Content == "" {
		h.logger.Info("content is empty, fetching from DB for stream", "article_id", req.ArticleID)
		fetchedArticle, err := h.articleRepo.FindByID(ctx, req.ArticleID)
		if err != nil {
			h.logger.Error("failed to fetch article from DB", "error", err, "article_id", req.ArticleID)
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch article content")
		}
		if fetchedArticle == nil {
			h.logger.Warn("article not found in DB", "article_id", req.ArticleID)
			return echo.NewHTTPError(http.StatusNotFound, "Article not found")
		}

		// Zero Trust extraction logic
		content := fetchedArticle.Content
		if fetchedArticle.Content != "" {
			extractedText := html_parser.ExtractArticleText(fetchedArticle.Content)
			if extractedText != "" {
				content = extractedText
			}
		}
		req.Content = content
		if req.Title == "" {
			req.Title = fetchedArticle.Title
		}
	}

	if req.Content == "" {
		h.logger.Warn("empty content provided and not found in DB", "article_id", req.ArticleID)
		return echo.NewHTTPError(http.StatusBadRequest, "Content cannot be empty")
	}

	// Zero Trust re-extraction
	if strings.Contains(req.Content, "<") && strings.Contains(req.Content, ">") {
		req.Content = html_parser.ExtractArticleText(req.Content)
	}

	h.logger.Info("processing streaming summarization request", "article_id", req.ArticleID, "content_length", len(req.Content))

	article := &models.Article{
		ID:      req.ArticleID,
		Content: req.Content,
	}

	// Call streaming service
	stream, err := h.apiRepo.StreamSummarizeArticle(ctx, article)
	if err != nil {
		// Check if it's a content too short error
		if errors.Is(err, driver.ErrContentTooShort) {
			h.logger.Warn("content too short for streaming summarization", "article_id", req.ArticleID, "content_length", len(req.Content))
			return echo.NewHTTPError(http.StatusBadRequest, "Content is too short for summarization (minimum 100 characters required)")
		}
		h.logger.Error("failed to start streaming summary", "error", err, "article_id", req.ArticleID, "error_type", fmt.Sprintf("%T", err))
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to generate summary stream: %v", err))
	}
	defer func() {
		if cerr := stream.Close(); cerr != nil {
			h.logger.Warn("failed to close summary stream", "error", cerr, "article_id", req.ArticleID)
		}
	}()

	h.logger.Info("stream obtained from news-creator", "article_id", req.ArticleID)

	// Set headers for SSE streaming
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream; charset=utf-8")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().Header().Set(echo.HeaderConnection, "keep-alive")
	c.Response().Header().Set("X-Accel-Buffering", "no") // Disable Nginx buffering for SSE
	c.Response().WriteHeader(http.StatusOK)

	h.logger.Info("response headers set, starting to read stream", "article_id", req.ArticleID)

	// Stream response with smaller buffer for incremental rendering
	buf := make([]byte, 128) // Reduced to 128 bytes for faster incremental rendering
	bytesWritten := 0
	hasData := false
	readAttempts := 0
	for {
		readAttempts++
		n, err := stream.Read(buf)
		if n > 0 {
			hasData = true
			bytesWritten += n
			// Log first few chunks and periodically for debugging incremental rendering
			if readAttempts <= 3 || bytesWritten%5120 == 0 {
				h.logger.Info("stream data received and flushed", "article_id", req.ArticleID, "bytes_written", bytesWritten, "chunk_size", n, "read_attempts", readAttempts)
			} else if readAttempts <= 10 {
				// Log more frequently for first 10 chunks to verify incremental rendering
				h.logger.Debug("stream chunk flushed", "article_id", req.ArticleID, "chunk_size", n, "read_attempts", readAttempts)
			}
			// Write immediately and flush for incremental rendering
			if _, wErr := c.Response().Write(buf[:n]); wErr != nil {
				h.logger.Error("error writing to response stream", "error", wErr, "article_id", req.ArticleID, "bytes_written", bytesWritten)
				return wErr
			}
			// Flush immediately after each chunk for incremental rendering
			c.Response().Flush()
		}
		if err != nil {
			if err == io.EOF {
				h.logger.Info("stream reached EOF", "article_id", req.ArticleID, "bytes_written", bytesWritten, "read_attempts", readAttempts)
				break
			}
			h.logger.Error("error reading from stream", "error", err, "article_id", req.ArticleID, "bytes_written", bytesWritten, "read_attempts", readAttempts)
			return err
		}
		if n == 0 && readAttempts > 1 {
			// No data read but no error - might be a timeout or empty stream
			h.logger.Warn("no data read from stream", "article_id", req.ArticleID, "read_attempts", readAttempts)
		}
	}

	// Check if any data was actually streamed
	if !hasData {
		h.logger.Warn("stream completed but no data was sent", "article_id", req.ArticleID)
		// Still return success, but log the warning
	} else {
		h.logger.Info("stream completed successfully", "article_id", req.ArticleID, "bytes_written", bytesWritten)
	}

	return nil
}

// HandleSummarizeQueue handles POST /api/v1/summarize/queue requests
// This endpoint queues a summarization job and returns immediately with a job ID
func (h *SummarizeHandler) HandleSummarizeQueue(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse request body
	var req SummarizeQueueRequest
	if err := c.Bind(&req); err != nil {
		h.logger.Error("failed to bind request", "error", err)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request format")
	}

	// Validate required fields
	if req.ArticleID == "" {
		h.logger.Warn("empty article_id provided")
		return echo.NewHTTPError(http.StatusBadRequest, "Article ID cannot be empty")
	}

	h.logger.Info("queueing summarization job", "article_id", req.ArticleID)

	// Create job in queue
	jobID, err := h.jobRepo.CreateJob(ctx, req.ArticleID)
	if err != nil {
		h.logger.Error("failed to create summarization job", "error", err, "article_id", req.ArticleID)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to queue summarization job")
	}

	h.logger.Info("summarization job queued successfully", "job_id", jobID, "article_id", req.ArticleID)

	// Return 202 Accepted with job ID
	response := SummarizeQueueResponse{
		JobID:   jobID,
		Status:  "pending",
		Message: "Summarization job queued successfully",
	}

	return c.JSON(http.StatusAccepted, response)
}

// SummarizeStatusResponse represents the response for job status check
type SummarizeStatusResponse struct {
	JobID        string `json:"job_id"`
	Status       string `json:"status"`
	Summary      string `json:"summary,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	ArticleID    string `json:"article_id"`
}

// HandleSummarizeStatus handles GET /api/v1/summarize/status/{job_id} requests
// This endpoint returns the current status of a summarization job
func (h *SummarizeHandler) HandleSummarizeStatus(c echo.Context) error {
	ctx := c.Request().Context()

	jobID := c.Param("job_id")
	if jobID == "" {
		h.logger.Warn("empty job_id provided")
		return echo.NewHTTPError(http.StatusBadRequest, "Job ID cannot be empty")
	}

	h.logger.Debug("checking summarization job status", "job_id", jobID)

	// Get job from queue
	job, err := h.jobRepo.GetJob(ctx, jobID)
	if err != nil {
		h.logger.Warn("summarization job not found", "job_id", jobID, "error", err)
		return echo.NewHTTPError(http.StatusNotFound, "Job not found")
	}

	response := SummarizeStatusResponse{
		JobID:     job.JobID.String(),
		Status:    string(job.Status),
		ArticleID: job.ArticleID,
	}

	// Include summary if completed
	if job.Status == models.SummarizeJobStatusCompleted && job.Summary != nil {
		response.Summary = *job.Summary
	}

	// Include error message if failed
	if job.Status == models.SummarizeJobStatusFailed && job.ErrorMessage != nil {
		response.ErrorMessage = *job.ErrorMessage
	}

	h.logger.Debug("summarization job status retrieved", "job_id", jobID, "status", job.Status)
	return c.JSON(http.StatusOK, response)
}
