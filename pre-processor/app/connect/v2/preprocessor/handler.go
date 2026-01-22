// Package preprocessor provides Connect-RPC handlers for pre-processor operations.
package preprocessor

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"connectrpc.com/connect"

	"pre-processor/domain"
	preprocessorv2 "pre-processor/gen/proto/services/preprocessor/v2"
	"pre-processor/gen/proto/services/preprocessor/v2/preprocessorv2connect"
	"pre-processor/models"
	"pre-processor/repository"
	"pre-processor/utils/html_parser"
)

// Handler implements the PreProcessorService Connect-RPC handler.
type Handler struct {
	apiRepo     repository.ExternalAPIRepository
	summaryRepo repository.SummaryRepository
	articleRepo repository.ArticleRepository
	jobRepo     repository.SummarizeJobRepository
	logger      *slog.Logger
}

// NewHandler creates a new preprocessor handler.
func NewHandler(
	apiRepo repository.ExternalAPIRepository,
	summaryRepo repository.SummaryRepository,
	articleRepo repository.ArticleRepository,
	jobRepo repository.SummarizeJobRepository,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		apiRepo:     apiRepo,
		summaryRepo: summaryRepo,
		articleRepo: articleRepo,
		jobRepo:     jobRepo,
		logger:      logger,
	}
}

// Compile-time check that Handler implements PreProcessorServiceHandler.
var _ preprocessorv2connect.PreProcessorServiceHandler = (*Handler)(nil)

// Summarize performs synchronous article summarization.
func (h *Handler) Summarize(
	ctx context.Context,
	req *connect.Request[preprocessorv2.SummarizeRequest],
) (*connect.Response[preprocessorv2.SummarizeResponse], error) {
	articleID := req.Msg.ArticleId
	title := req.Msg.Title
	content := req.Msg.Content

	// Validate required fields
	if articleID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("article_id is required"))
	}

	// Fetch article to get user_id (always needed for summary storage)
	fetchedArticle, err := h.articleRepo.FindByID(ctx, articleID)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to fetch article", "error", err, "article_id", articleID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch article"))
	}
	if fetchedArticle == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("article not found"))
	}

	// If content is empty, use article content from DB
	if content == "" {
		h.logger.InfoContext(ctx, "content is empty, using content from DB", "article_id", articleID)
		if fetchedArticle.Content == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("article content is empty"))
		}
		content = html_parser.ExtractArticleText(fetchedArticle.Content)
		if content == "" {
			content = fetchedArticle.Content
		}
		if title == "" {
			title = fetchedArticle.Title
		}
	}

	// Zero Trust re-extraction
	if strings.Contains(content, "<") && strings.Contains(content, ">") {
		content = html_parser.ExtractArticleText(content)
	}

	h.logger.InfoContext(ctx, "processing summarization request", "article_id", articleID, "content_length", len(content))

	// Create article model for summarization
	article := &models.Article{
		ID:      articleID,
		Content: content,
	}

	// Call summarization service with HIGH priority for UI-triggered requests
	summarized, err := h.apiRepo.SummarizeArticle(ctx, article, "high")
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to generate summary", "error", err, "article_id", articleID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to generate summary"))
	}

	h.logger.InfoContext(ctx, "article summarized successfully", "article_id", articleID)

	// Save summary to database
	articleTitle := title
	if articleTitle == "" {
		articleTitle = "Untitled"
	}

	articleSummary := &models.ArticleSummary{
		ArticleID:       articleID,
		UserID:          fetchedArticle.UserID,
		ArticleTitle:    articleTitle,
		SummaryJapanese: summarized.SummaryJapanese,
	}

	if err := h.summaryRepo.Create(ctx, articleSummary); err != nil {
		h.logger.ErrorContext(ctx, "failed to save summary to database", "error", err, "article_id", articleID)
		// Don't fail the request if DB save fails
	} else {
		h.logger.InfoContext(ctx, "summary saved to database successfully", "article_id", articleID)
	}

	return connect.NewResponse(&preprocessorv2.SummarizeResponse{
		Success:   true,
		Summary:   summarized.SummaryJapanese,
		ArticleId: articleID,
	}), nil
}

// StreamSummarize performs streaming article summarization.
func (h *Handler) StreamSummarize(
	ctx context.Context,
	req *connect.Request[preprocessorv2.StreamSummarizeRequest],
	stream *connect.ServerStream[preprocessorv2.StreamSummarizeResponse],
) error {
	articleID := req.Msg.ArticleId
	content := req.Msg.Content

	// Validate required fields
	if articleID == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("article_id is required"))
	}

	// If content is empty, fetch from DB
	if content == "" {
		h.logger.InfoContext(ctx, "content is empty, fetching from DB for stream", "article_id", articleID)
		fetchedArticle, err := h.articleRepo.FindByID(ctx, articleID)
		if err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch article"))
		}
		if fetchedArticle == nil {
			return connect.NewError(connect.CodeNotFound, fmt.Errorf("article not found"))
		}

		content = html_parser.ExtractArticleText(fetchedArticle.Content)
		if content == "" {
			content = fetchedArticle.Content
		}
	}

	// Zero Trust re-extraction
	if strings.Contains(content, "<") && strings.Contains(content, ">") {
		content = html_parser.ExtractArticleText(content)
	}

	if content == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("content is empty"))
	}

	h.logger.InfoContext(ctx, "processing streaming summarization request", "article_id", articleID, "content_length", len(content))

	article := &models.Article{
		ID:      articleID,
		Content: content,
	}

	// Call streaming service with HIGH priority for UI-triggered requests
	ioStream, err := h.apiRepo.StreamSummarizeArticle(ctx, article, "high")
	if err != nil {
		if err == domain.ErrContentTooShort {
			return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("content too short"))
		}
		h.logger.ErrorContext(ctx, "failed to generate summary stream", "error", err, "article_id", articleID)
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to generate summary stream"))
	}
	defer func() { _ = ioStream.Close() }()

	h.logger.InfoContext(ctx, "stream obtained from news-creator", "article_id", articleID)

	// Stream response
	buf := make([]byte, 128)
	for {
		n, err := ioStream.Read(buf)
		if n > 0 {
			if sendErr := stream.Send(&preprocessorv2.StreamSummarizeResponse{
				Chunk:   string(buf[:n]),
				IsFinal: false,
			}); sendErr != nil {
				h.logger.ErrorContext(ctx, "error sending stream chunk", "error", sendErr, "article_id", articleID)
				return sendErr
			}
		}
		if err != nil {
			if err.Error() == "EOF" {
				// Send final chunk
				if sendErr := stream.Send(&preprocessorv2.StreamSummarizeResponse{
					Chunk:   "",
					IsFinal: true,
				}); sendErr != nil {
					return sendErr
				}
				break
			}
			h.logger.ErrorContext(ctx, "error reading from stream", "error", err, "article_id", articleID)
			return err
		}
	}

	h.logger.InfoContext(ctx, "stream completed successfully", "article_id", articleID)
	return nil
}

// QueueSummarize submits an article for async summarization.
func (h *Handler) QueueSummarize(
	ctx context.Context,
	req *connect.Request[preprocessorv2.SummarizeQueueRequest],
) (*connect.Response[preprocessorv2.SummarizeQueueResponse], error) {
	articleID := req.Msg.ArticleId

	// Validate required fields
	if articleID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("article_id is required"))
	}

	h.logger.InfoContext(ctx, "queueing summarization job", "article_id", articleID)

	// Create job in queue
	jobID, err := h.jobRepo.CreateJob(ctx, articleID)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to queue summarization job", "error", err, "article_id", articleID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to queue summarization job"))
	}

	h.logger.InfoContext(ctx, "summarization job queued successfully", "job_id", jobID, "article_id", articleID)

	return connect.NewResponse(&preprocessorv2.SummarizeQueueResponse{
		JobId:   jobID,
		Status:  "pending",
		Message: "Summarization job queued successfully",
	}), nil
}

// GetSummarizeStatus checks the status of a summarization job.
func (h *Handler) GetSummarizeStatus(
	ctx context.Context,
	req *connect.Request[preprocessorv2.SummarizeStatusRequest],
) (*connect.Response[preprocessorv2.SummarizeStatusResponse], error) {
	jobID := req.Msg.JobId

	// Validate required fields
	if jobID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("job_id is required"))
	}

	h.logger.DebugContext(ctx, "checking summarization job status", "job_id", jobID)

	// Get job from queue
	job, err := h.jobRepo.GetJob(ctx, jobID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("job not found"))
	}

	response := &preprocessorv2.SummarizeStatusResponse{
		JobId:     job.JobID.String(),
		Status:    string(job.Status),
		ArticleId: job.ArticleID,
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
	return connect.NewResponse(response), nil
}
