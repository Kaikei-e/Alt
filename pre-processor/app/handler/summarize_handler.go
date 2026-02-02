package handler

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"pre-processor/domain"
	"pre-processor/models"
	"pre-processor/repository"
	apperrors "pre-processor/utils/errors"
	"pre-processor/utils/html_parser"

	"github.com/labstack/echo/v4"
)

// processingLock holds lock metadata for timeout-based lock management.
type processingLock struct {
	startTime time.Time
}

// processingArticles tracks article IDs currently being processed to prevent duplicate requests.
// This prevents the retry loop issue where timeout causes immediate retry which fills the queue.
var processingArticles sync.Map

// processingTimeout defines how long a lock can be held before it's considered stale.
// This prevents "zombie locks" when the original request hangs or client disconnects
// with context.Background() being used upstream.
const processingTimeout = 5 * time.Minute

// tryAcquireLock attempts to acquire a processing lock for the given article ID.
// Returns true if lock was acquired, false if article is already being processed.
// Stale locks (older than processingTimeout) are automatically released.
func tryAcquireLock(articleID string) bool {
	now := time.Now()
	newLock := &processingLock{startTime: now}

	// Try to store the lock
	if actual, loaded := processingArticles.LoadOrStore(articleID, newLock); loaded {
		// Lock exists - check if it's stale
		existingLock := actual.(*processingLock)
		if time.Since(existingLock.startTime) > processingTimeout {
			// Stale lock - try to replace it with CompareAndSwap
			if processingArticles.CompareAndSwap(articleID, existingLock, newLock) {
				return true // Successfully replaced stale lock
			}
			// Another goroutine beat us - retry the whole operation
			return tryAcquireLock(articleID)
		}
		return false // Lock is still valid
	}
	return true // Successfully acquired new lock
}

// releaseLock releases the processing lock for the given article ID.
func releaseLock(articleID string) {
	processingArticles.Delete(articleID)
}

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
		return apperrors.NewValidationContextError(
			"invalid request format",
			"handler", "SummarizeHandler", "HandleSummarize",
			map[string]interface{}{"bind_error": err.Error()},
		)
	}

	// Validate required fields
	if req.ArticleID == "" {
		return apperrors.NewValidationContextError(
			domain.ErrMissingArticleID.Error(),
			"handler", "SummarizeHandler", "HandleSummarize",
			nil,
		)
	}

	// Check if this article is already being processed to prevent duplicate requests.
	// This prevents the retry loop issue where timeout causes immediate retry which fills the queue.
	// Uses timeout-based lock to prevent zombie locks when upstream uses context.Background().
	if !tryAcquireLock(req.ArticleID) {
		h.logger.WarnContext(ctx, "article is already being processed, rejecting duplicate request",
			"article_id", req.ArticleID)
		return apperrors.NewConflictContextError(
			"article is already being processed",
			"handler", "SummarizeHandler", "HandleSummarize",
			map[string]interface{}{"article_id": req.ArticleID},
		)
	}
	// Ensure we clean up the tracking entry when done
	defer releaseLock(req.ArticleID)

	// Fetch article to get user_id (always needed for summary storage)
	fetchedArticle, err := h.articleRepo.FindByID(ctx, req.ArticleID)
	if err != nil {
		return apperrors.NewDatabaseContextError(
			"failed to fetch article",
			"handler", "SummarizeHandler", "HandleSummarize",
			err,
			map[string]interface{}{"article_id": req.ArticleID},
		)
	}
	if fetchedArticle == nil {
		return apperrors.NewNotFoundContextError(
			domain.ErrArticleNotFound.Error(),
			"handler", "SummarizeHandler", "HandleSummarize",
			map[string]interface{}{"article_id": req.ArticleID},
		)
	}

	// If content is empty, use article content from DB
	if req.Content == "" {
		h.logger.InfoContext(ctx, "content is empty, using content from DB", "article_id", req.ArticleID)
		if fetchedArticle.Content == "" {
			return apperrors.NewValidationContextError(
				domain.ErrArticleContentEmpty.Error(),
				"handler", "SummarizeHandler", "HandleSummarize",
				map[string]interface{}{"article_id": req.ArticleID},
			)
		}
		// Zero Trust: Always extract text from content (HTML or not)
		// This ensures we never send raw HTML to downstream services
		content := fetchedArticle.Content
		originalLength := len(content)
		h.logger.InfoContext(ctx, "extracting text from content (Zero Trust validation)", "article_id", req.ArticleID, "original_length", originalLength)

		extractedText := html_parser.ExtractArticleText(content)
		extractedLength := len(extractedText)

		if extractedText != "" {
			content = extractedText
			reductionRatio := (1.0 - float64(extractedLength)/float64(originalLength)) * 100.0
			h.logger.InfoContext(ctx, "text extraction completed",
				"article_id", req.ArticleID,
				"original_length", originalLength,
				"extracted_length", extractedLength,
				"reduction_ratio", fmt.Sprintf("%.2f%%", reductionRatio))
		} else {
			h.logger.WarnContext(ctx, "text extraction returned empty, using original content", "article_id", req.ArticleID, "original_length", originalLength)
		}
		req.Content = content
		// Also update title if missing
		if req.Title == "" {
			req.Title = fetchedArticle.Title
		}
		h.logger.InfoContext(ctx, "content fetched from DB successfully", "article_id", req.ArticleID, "content_length", len(req.Content))
	}

	if req.Content == "" {
		return apperrors.NewValidationContextError(
			domain.ErrEmptyContent.Error(),
			"handler", "SummarizeHandler", "HandleSummarize",
			map[string]interface{}{"article_id": req.ArticleID},
		)
	}

	// Zero Trust: Ensure content is extracted text before processing
	// If content still contains HTML-like patterns, extract again
	if strings.Contains(req.Content, "<") && strings.Contains(req.Content, ">") {
		h.logger.WarnContext(ctx, "content still contains HTML after extraction, re-extracting", "article_id", req.ArticleID, "content_length", len(req.Content))
		req.Content = html_parser.ExtractArticleText(req.Content)
		h.logger.InfoContext(ctx, "re-extraction completed", "article_id", req.ArticleID, "final_length", len(req.Content))
	}

	h.logger.InfoContext(ctx, "processing summarization request", "article_id", req.ArticleID, "content_length", len(req.Content))

	// Create article model for summarization
	article := &models.Article{
		ID:      req.ArticleID,
		Content: req.Content,
	}

	// Call summarization service with HIGH priority for UI-triggered requests
	summarized, err := h.apiRepo.SummarizeArticle(ctx, article, "high")
	if err != nil {
		return apperrors.NewExternalAPIContextError(
			"failed to generate summary",
			"handler", "SummarizeHandler", "HandleSummarize",
			err,
			map[string]interface{}{"article_id": req.ArticleID},
		)
	}

	h.logger.InfoContext(ctx, "article summarized successfully", "article_id", req.ArticleID)

	// Save summary to database
	articleTitle := req.Title
	if articleTitle == "" {
		articleTitle = "Untitled" // Fallback if no title provided
	}

	articleSummary := &models.ArticleSummary{
		ArticleID:       req.ArticleID,
		UserID:          fetchedArticle.UserID,
		ArticleTitle:    articleTitle,
		SummaryJapanese: summarized.SummaryJapanese,
	}

	if err := h.summaryRepo.Create(ctx, articleSummary); err != nil {
		h.logger.ErrorContext(ctx, "failed to save summary to database", "error", err, "article_id", req.ArticleID)
		// Don't fail the request if DB save fails - still return the summary
		// This ensures the user gets the summary even if DB has issues
		h.logger.WarnContext(ctx, "continuing despite DB save failure", "article_id", req.ArticleID)
	} else {
		h.logger.InfoContext(ctx, "summary saved to database successfully", "article_id", req.ArticleID)
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
		return apperrors.NewValidationContextError(
			"invalid request format",
			"handler", "SummarizeHandler", "HandleStreamSummarize",
			map[string]interface{}{"bind_error": err.Error()},
		)
	}

	// Validate required fields
	if req.ArticleID == "" {
		return apperrors.NewValidationContextError(
			domain.ErrMissingArticleID.Error(),
			"handler", "SummarizeHandler", "HandleStreamSummarize",
			nil,
		)
	}

	// Check if this article is already being processed to prevent duplicate requests.
	// This prevents the retry loop issue where timeout causes immediate retry which fills the queue.
	// Uses timeout-based lock to prevent zombie locks when upstream uses context.Background().
	if !tryAcquireLock(req.ArticleID) {
		h.logger.WarnContext(ctx, "article is already being processed (stream), rejecting duplicate request",
			"article_id", req.ArticleID)
		return apperrors.NewConflictContextError(
			"article is already being processed",
			"handler", "SummarizeHandler", "HandleStreamSummarize",
			map[string]interface{}{"article_id": req.ArticleID},
		)
	}
	// Ensure we clean up the tracking entry when done
	defer releaseLock(req.ArticleID)

	// If content is empty, try to fetch from DB
	if req.Content == "" {
		h.logger.InfoContext(ctx, "content is empty, fetching from DB for stream", "article_id", req.ArticleID)
		fetchedArticle, err := h.articleRepo.FindByID(ctx, req.ArticleID)
		if err != nil {
			return apperrors.NewDatabaseContextError(
				"failed to fetch article content",
				"handler", "SummarizeHandler", "HandleStreamSummarize",
				err,
				map[string]interface{}{"article_id": req.ArticleID},
			)
		}
		if fetchedArticle == nil {
			return apperrors.NewNotFoundContextError(
				domain.ErrArticleNotFound.Error(),
				"handler", "SummarizeHandler", "HandleStreamSummarize",
				map[string]interface{}{"article_id": req.ArticleID},
			)
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
		return apperrors.NewValidationContextError(
			domain.ErrEmptyContent.Error(),
			"handler", "SummarizeHandler", "HandleStreamSummarize",
			map[string]interface{}{"article_id": req.ArticleID},
		)
	}

	// Zero Trust re-extraction
	if strings.Contains(req.Content, "<") && strings.Contains(req.Content, ">") {
		req.Content = html_parser.ExtractArticleText(req.Content)
	}

	h.logger.InfoContext(ctx, "processing streaming summarization request", "article_id", req.ArticleID, "content_length", len(req.Content))

	article := &models.Article{
		ID:      req.ArticleID,
		Content: req.Content,
	}

	// Call streaming service with HIGH priority for UI-triggered requests
	stream, err := h.apiRepo.StreamSummarizeArticle(ctx, article, "high")
	if err != nil {
		// Check if it's a content too short error
		if errors.Is(err, domain.ErrContentTooShort) {
			return apperrors.NewValidationContextError(
				domain.ErrContentTooShort.Error(),
				"handler", "SummarizeHandler", "HandleStreamSummarize",
				map[string]interface{}{"article_id": req.ArticleID, "content_length": len(req.Content)},
			)
		}
		return apperrors.NewExternalAPIContextError(
			"failed to generate summary stream",
			"handler", "SummarizeHandler", "HandleStreamSummarize",
			err,
			map[string]interface{}{"article_id": req.ArticleID},
		)
	}
	defer func() {
		if cerr := stream.Close(); cerr != nil {
			h.logger.WarnContext(ctx, "failed to close summary stream", "error", cerr, "article_id", req.ArticleID)
		}
	}()

	h.logger.InfoContext(ctx, "stream obtained from news-creator", "article_id", req.ArticleID)

	// Set headers for SSE streaming
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream; charset=utf-8")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().Header().Set(echo.HeaderConnection, "keep-alive")
	c.Response().Header().Set("X-Accel-Buffering", "no") // Disable Nginx buffering for SSE
	c.Response().WriteHeader(http.StatusOK)

	h.logger.InfoContext(ctx, "response headers set, starting to read stream", "article_id", req.ArticleID)

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
				h.logger.InfoContext(ctx, "stream data received and flushed", "article_id", req.ArticleID, "bytes_written", bytesWritten, "chunk_size", n, "read_attempts", readAttempts)
			} else if readAttempts <= 10 {
				// Log more frequently for first 10 chunks to verify incremental rendering
				h.logger.DebugContext(ctx, "stream chunk flushed", "article_id", req.ArticleID, "chunk_size", n, "read_attempts", readAttempts)
			}
			// Write immediately and flush for incremental rendering
			if _, wErr := c.Response().Write(buf[:n]); wErr != nil {
				h.logger.ErrorContext(ctx, "error writing to response stream", "error", wErr, "article_id", req.ArticleID, "bytes_written", bytesWritten)
				return wErr
			}
			// Flush immediately after each chunk for incremental rendering
			c.Response().Flush()
		}
		if err != nil {
			if err == io.EOF {
				h.logger.InfoContext(ctx, "stream reached EOF", "article_id", req.ArticleID, "bytes_written", bytesWritten, "read_attempts", readAttempts)
				break
			}
			h.logger.ErrorContext(ctx, "error reading from stream", "error", err, "article_id", req.ArticleID, "bytes_written", bytesWritten, "read_attempts", readAttempts)
			return err
		}
		if n == 0 && readAttempts > 1 {
			// No data read but no error - might be a timeout or empty stream
			h.logger.WarnContext(ctx, "no data read from stream", "article_id", req.ArticleID, "read_attempts", readAttempts)
		}
	}

	// Check if any data was actually streamed
	if !hasData {
		h.logger.WarnContext(ctx, "stream completed but no data was sent", "article_id", req.ArticleID)
		// Still return success, but log the warning
	} else {
		h.logger.InfoContext(ctx, "stream completed successfully", "article_id", req.ArticleID, "bytes_written", bytesWritten)
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
		return apperrors.NewValidationContextError(
			"invalid request format",
			"handler", "SummarizeHandler", "HandleSummarizeQueue",
			map[string]interface{}{"bind_error": err.Error()},
		)
	}

	// Validate required fields
	if req.ArticleID == "" {
		return apperrors.NewValidationContextError(
			domain.ErrMissingArticleID.Error(),
			"handler", "SummarizeHandler", "HandleSummarizeQueue",
			nil,
		)
	}

	h.logger.InfoContext(ctx, "queueing summarization job", "article_id", req.ArticleID)

	// Create job in queue
	jobID, err := h.jobRepo.CreateJob(ctx, req.ArticleID)
	if err != nil {
		return apperrors.NewDatabaseContextError(
			"failed to queue summarization job",
			"handler", "SummarizeHandler", "HandleSummarizeQueue",
			err,
			map[string]interface{}{"article_id": req.ArticleID},
		)
	}

	h.logger.InfoContext(ctx, "summarization job queued successfully", "job_id", jobID, "article_id", req.ArticleID)

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
		return apperrors.NewValidationContextError(
			"job ID is required",
			"handler", "SummarizeHandler", "HandleSummarizeStatus",
			nil,
		)
	}

	h.logger.DebugContext(ctx, "checking summarization job status", "job_id", jobID)

	// Get job from queue
	job, err := h.jobRepo.GetJob(ctx, jobID)
	if err != nil {
		return apperrors.NewNotFoundContextError(
			domain.ErrJobNotFound.Error(),
			"handler", "SummarizeHandler", "HandleSummarizeStatus",
			map[string]interface{}{"job_id": jobID},
		)
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

	h.logger.DebugContext(ctx, "summarization job status retrieved", "job_id", jobID, "status", job.Status)
	return c.JSON(http.StatusOK, response)
}
