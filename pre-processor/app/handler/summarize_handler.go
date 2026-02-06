package handler

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"pre-processor/domain"
	"pre-processor/repository"
	summarizeuc "pre-processor/usecase/summarize"
	apperrors "pre-processor/utils/errors"

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
	onDemand    *summarizeuc.OnDemandService
	logger      *slog.Logger
}

// NewSummarizeHandler creates a new summarize handler
func NewSummarizeHandler(apiRepo repository.ExternalAPIRepository, summaryRepo repository.SummaryRepository, articleRepo repository.ArticleRepository, jobRepo repository.SummarizeJobRepository, logger *slog.Logger) *SummarizeHandler {
	return &SummarizeHandler{
		apiRepo:     apiRepo,
		summaryRepo: summaryRepo,
		articleRepo: articleRepo,
		jobRepo:     jobRepo,
		onDemand:    summarizeuc.NewOnDemandService(articleRepo, summaryRepo, apiRepo, logger),
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
	if !tryAcquireLock(req.ArticleID) {
		h.logger.WarnContext(ctx, "article is already being processed, rejecting duplicate request",
			"article_id", req.ArticleID)
		return apperrors.NewConflictContextError(
			"article is already being processed",
			"handler", "SummarizeHandler", "HandleSummarize",
			map[string]interface{}{"article_id": req.ArticleID},
		)
	}
	defer releaseLock(req.ArticleID)

	// Delegate to shared on-demand summarization usecase
	result, err := h.onDemand.Summarize(ctx, summarizeuc.SummarizeRequest{
		ArticleID: req.ArticleID,
		Content:   req.Content,
		Title:     req.Title,
		Priority:  "high", // UI-triggered requests
	})
	if err != nil {
		return mapDomainErrorToHTTP(err, req.ArticleID)
	}

	return c.JSON(http.StatusOK, SummarizeResponse{
		Success:   true,
		Summary:   result.Summary,
		ArticleID: req.ArticleID,
	})
}

// mapDomainErrorToHTTP converts domain errors to appropriate HTTP error responses.
func mapDomainErrorToHTTP(err error, articleID string) error {
	switch {
	case errors.Is(err, domain.ErrArticleNotFound):
		return apperrors.NewNotFoundContextError(
			domain.ErrArticleNotFound.Error(),
			"handler", "SummarizeHandler", "HandleSummarize",
			map[string]interface{}{"article_id": articleID},
		)
	case errors.Is(err, domain.ErrArticleContentEmpty):
		return apperrors.NewValidationContextError(
			domain.ErrArticleContentEmpty.Error(),
			"handler", "SummarizeHandler", "HandleSummarize",
			map[string]interface{}{"article_id": articleID},
		)
	case errors.Is(err, domain.ErrEmptyContent):
		return apperrors.NewValidationContextError(
			domain.ErrEmptyContent.Error(),
			"handler", "SummarizeHandler", "HandleSummarize",
			map[string]interface{}{"article_id": articleID},
		)
	case errors.Is(err, domain.ErrContentTooShort):
		return apperrors.NewValidationContextError(
			domain.ErrContentTooShort.Error(),
			"handler", "SummarizeHandler", "HandleSummarize",
			map[string]interface{}{"article_id": articleID},
		)
	default:
		return apperrors.NewExternalAPIContextError(
			"failed to generate summary",
			"handler", "SummarizeHandler", "HandleSummarize",
			err,
			map[string]interface{}{"article_id": articleID},
		)
	}
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
	if !tryAcquireLock(req.ArticleID) {
		h.logger.WarnContext(ctx, "article is already being processed (stream), rejecting duplicate request",
			"article_id", req.ArticleID)
		return apperrors.NewConflictContextError(
			"article is already being processed",
			"handler", "SummarizeHandler", "HandleStreamSummarize",
			map[string]interface{}{"article_id": req.ArticleID},
		)
	}
	defer releaseLock(req.ArticleID)

	// Resolve article content using shared usecase
	resolved, err := h.onDemand.ResolveArticle(ctx, summarizeuc.SummarizeRequest{
		ArticleID: req.ArticleID,
		Content:   req.Content,
		Title:     req.Title,
	})
	if err != nil {
		return mapDomainErrorToHTTP(err, req.ArticleID)
	}

	h.logger.InfoContext(ctx, "processing streaming summarization request", "article_id", req.ArticleID, "content_length", len(resolved.Content))

	article := &domain.Article{
		ID:      req.ArticleID,
		Content: resolved.Content,
	}

	// Call streaming service with HIGH priority for UI-triggered requests
	stream, err := h.apiRepo.StreamSummarizeArticle(ctx, article, "high")
	if err != nil {
		return mapDomainErrorToHTTP(err, req.ArticleID)
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
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)

	// Stream response with smaller buffer for incremental rendering
	buf := make([]byte, 128)
	bytesWritten := 0
	hasData := false
	readAttempts := 0
	for {
		readAttempts++
		n, err := stream.Read(buf)
		if n > 0 {
			hasData = true
			bytesWritten += n
			if readAttempts <= 3 || bytesWritten%5120 == 0 {
				h.logger.InfoContext(ctx, "stream data received and flushed", "article_id", req.ArticleID, "bytes_written", bytesWritten, "chunk_size", n, "read_attempts", readAttempts)
			}
			if _, wErr := c.Response().Write(buf[:n]); wErr != nil {
				h.logger.ErrorContext(ctx, "error writing to response stream", "error", wErr, "article_id", req.ArticleID, "bytes_written", bytesWritten)
				return wErr
			}
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
	}

	if !hasData {
		h.logger.WarnContext(ctx, "stream completed but no data was sent", "article_id", req.ArticleID)
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
	if job.Status == domain.SummarizeJobStatusCompleted && job.Summary != nil {
		response.Summary = *job.Summary
	}

	// Include error message if failed
	if job.Status == domain.SummarizeJobStatusFailed && job.ErrorMessage != nil {
		response.ErrorMessage = *job.ErrorMessage
	}

	h.logger.DebugContext(ctx, "summarization job status retrieved", "job_id", jobID, "status", job.Status)
	return c.JSON(http.StatusOK, response)
}
