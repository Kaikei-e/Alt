package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"pre-processor/domain"
	"pre-processor/repository"
	"pre-processor/utils/html_parser"
)

// SummarizeQueueWorker handles processing of queued summarization jobs
type SummarizeQueueWorker struct {
	jobRepo     repository.SummarizeJobRepository
	articleRepo repository.ArticleRepository
	apiRepo     repository.ExternalAPIRepository
	summaryRepo repository.SummaryRepository
	logger      *slog.Logger
	batchSize   int
}

// NewSummarizeQueueWorker creates a new summarize queue worker
func NewSummarizeQueueWorker(
	jobRepo repository.SummarizeJobRepository,
	articleRepo repository.ArticleRepository,
	apiRepo repository.ExternalAPIRepository,
	summaryRepo repository.SummaryRepository,
	logger *slog.Logger,
	batchSize int,
) *SummarizeQueueWorker {
	return &SummarizeQueueWorker{
		jobRepo:     jobRepo,
		articleRepo: articleRepo,
		apiRepo:     apiRepo,
		summaryRepo: summaryRepo,
		logger:      logger,
		batchSize:   batchSize,
	}
}

// ProcessQueue processes pending jobs from the queue
func (w *SummarizeQueueWorker) ProcessQueue(ctx context.Context) error {
	// Get pending jobs
	jobs, err := w.jobRepo.GetPendingJobs(ctx, w.batchSize)
	if err != nil {
		w.logger.ErrorContext(ctx, "failed to get pending jobs", "error", err)
		return fmt.Errorf("failed to get pending jobs: %w", err)
	}

	if len(jobs) == 0 {
		w.logger.DebugContext(ctx, "no pending jobs to process")
		return nil
	}

	w.logger.InfoContext(ctx, "processing queued summarization jobs", "count", len(jobs))

	// Process each job
	for i, job := range jobs {
		if ctx.Err() != nil {
			w.logger.WarnContext(ctx, "context canceled, skipping remaining jobs",
				"remaining", len(jobs)-i)
			break
		}

		if err := w.processJob(ctx, job); err != nil {
			// Check for downstream overload (429) - back off and skip remaining jobs
			if errors.Is(err, domain.ErrServiceOverloaded) {
				w.logger.WarnContext(ctx, "downstream service overloaded, backing off and skipping remaining jobs",
					"job_id", job.JobID,
					"article_id", job.ArticleID,
					"remaining", len(jobs)-i-1)
				return domain.ErrServiceOverloaded
			}
			w.logger.ErrorContext(ctx, "failed to process job", "error", err, "job_id", job.JobID, "article_id", job.ArticleID)
			// Continue processing other jobs even if one fails
			continue
		}
	}

	return nil
}

// processJob processes a single summarization job
func (w *SummarizeQueueWorker) processJob(ctx context.Context, job *domain.SummarizeJob) error {
	// Update status to running
	if err := w.jobRepo.UpdateJobStatus(ctx, job.JobID.String(), domain.SummarizeJobStatusRunning, "", ""); err != nil {
		w.logger.ErrorContext(ctx, "failed to update job status to running", "error", err, "job_id", job.JobID)
		return fmt.Errorf("failed to update job status: %w", err)
	}

	w.logger.InfoContext(ctx, "processing summarization job", "job_id", job.JobID, "article_id", job.ArticleID)

	// Fetch article from database
	article, err := w.articleRepo.FindByID(ctx, job.ArticleID)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to fetch article: %v", err)
		w.logger.ErrorContext(ctx, "failed to fetch article", "error", err, "article_id", job.ArticleID)
		if updateErr := w.jobRepo.UpdateJobStatus(ctx, job.JobID.String(), domain.SummarizeJobStatusFailed, "", errorMsg); updateErr != nil {
			w.logger.ErrorContext(ctx, "failed to update job status to failed", "error", updateErr, "job_id", job.JobID)
		}
		return fmt.Errorf("failed to fetch article: %w", err)
	}

	if article == nil {
		errorMsg := "Article not found in database"
		w.logger.WarnContext(ctx, "article not found", "article_id", job.ArticleID)
		if updateErr := w.jobRepo.UpdateJobStatus(ctx, job.JobID.String(), domain.SummarizeJobStatusFailed, "", errorMsg); updateErr != nil {
			w.logger.ErrorContext(ctx, "failed to update job status to failed", "error", updateErr, "job_id", job.JobID)
		}
		return fmt.Errorf("article not found: %s", job.ArticleID)
	}

	if article.Content == "" {
		errorMsg := "Article content is empty"
		w.logger.WarnContext(ctx, "article content is empty", "article_id", job.ArticleID)
		if updateErr := w.jobRepo.UpdateJobStatus(ctx, job.JobID.String(), domain.SummarizeJobStatusFailed, "", errorMsg); updateErr != nil {
			w.logger.ErrorContext(ctx, "failed to update job status to failed", "error", updateErr, "job_id", job.JobID)
		}
		return fmt.Errorf("article content is empty: %s", job.ArticleID)
	}

	// Extract text from HTML if needed
	content := article.Content
	if strings.Contains(content, "<") && strings.Contains(content, ">") {
		w.logger.InfoContext(ctx, "detected HTML content, extracting text", "article_id", job.ArticleID)
		extractedText := html_parser.ExtractArticleText(content)
		if extractedText != "" {
			content = extractedText
			w.logger.InfoContext(ctx, "HTML content extracted successfully", "article_id", job.ArticleID, "original_length", len(article.Content), "extracted_length", len(extractedText))
		} else {
			w.logger.WarnContext(ctx, "HTML extraction returned empty, using original content", "article_id", job.ArticleID)
		}
	}

	// Create article model for summarization
	articleModel := &domain.Article{
		ID:      job.ArticleID,
		Content: content,
	}

	// Call summarization service with LOW priority (queue worker is a background job)
	summarizeStartTime := time.Now()
	summarized, err := w.apiRepo.SummarizeArticle(ctx, articleModel, "low")
	summarizeDuration := time.Since(summarizeStartTime)

	if err != nil {
		errorMsg := fmt.Sprintf("Failed to generate summary: %v", err)
		w.logger.ErrorContext(ctx, "failed to summarize article",
			"error", err,
			"article_id", job.ArticleID,
			"duration_ms", summarizeDuration.Milliseconds(),
			"retry_count", job.RetryCount,
			"max_retries", job.MaxRetries)

		// Log whether this will be retried or moved to dead_letter
		// Note: The repository handles the status transition:
		// - If retry_count + 1 >= max_retries -> dead_letter
		// - Otherwise -> pending (will be retried)
		nextRetryCount := job.RetryCount + 1
		if nextRetryCount >= job.MaxRetries {
			w.logger.WarnContext(ctx, "job exceeded max retries, moving to dead_letter",
				"job_id", job.JobID,
				"article_id", job.ArticleID,
				"retry_count", nextRetryCount,
				"max_retries", job.MaxRetries)
		} else {
			w.logger.InfoContext(ctx, "job will be retried",
				"job_id", job.JobID,
				"article_id", job.ArticleID,
				"retry_count", nextRetryCount,
				"max_retries", job.MaxRetries)
		}

		if updateErr := w.jobRepo.UpdateJobStatus(ctx, job.JobID.String(), domain.SummarizeJobStatusFailed, "", errorMsg); updateErr != nil {
			w.logger.ErrorContext(ctx, "failed to update job status to failed", "error", updateErr, "job_id", job.JobID)
		}
		return fmt.Errorf("failed to summarize article: %w", err)
	}

	w.logger.InfoContext(ctx, "article summarized successfully",
		"job_id", job.JobID,
		"article_id", job.ArticleID,
		"summarize_duration_ms", summarizeDuration.Milliseconds())

	// Save summary to database
	articleTitle := article.Title
	if articleTitle == "" {
		articleTitle = "Untitled"
	}

	articleSummary := &domain.ArticleSummary{
		ArticleID:       job.ArticleID,
		UserID:          article.UserID,
		ArticleTitle:    articleTitle,
		SummaryJapanese: summarized.SummaryJapanese,
	}

	saveSummaryStartTime := time.Now()
	if err := w.summaryRepo.Create(ctx, articleSummary); err != nil {
		w.logger.ErrorContext(ctx, "failed to save summary to database", "error", err, "article_id", job.ArticleID)
		// Continue even if save fails - we still have the summary to return
		w.logger.WarnContext(ctx, "continuing despite DB save failure", "article_id", job.ArticleID)
	} else {
		saveSummaryDuration := time.Since(saveSummaryStartTime)
		w.logger.InfoContext(ctx, "summary saved to database successfully",
			"article_id", job.ArticleID,
			"save_duration_ms", saveSummaryDuration.Milliseconds())
	}

	// Update job status to completed
	updateStatusStartTime := time.Now()
	if err := w.jobRepo.UpdateJobStatus(ctx, job.JobID.String(), domain.SummarizeJobStatusCompleted, summarized.SummaryJapanese, ""); err != nil {
		w.logger.ErrorContext(ctx, "failed to update job status to completed", "error", err, "job_id", job.JobID)
		return fmt.Errorf("failed to update job status: %w", err)
	}
	updateStatusDuration := time.Since(updateStatusStartTime)

	totalDuration := time.Since(summarizeStartTime)
	w.logger.InfoContext(ctx, "summarization job completed successfully",
		"job_id", job.JobID,
		"article_id", job.ArticleID,
		"update_status_duration_ms", updateStatusDuration.Milliseconds(),
		"total_duration_ms", totalDuration.Milliseconds(),
		"completed_at", time.Now().UnixNano())
	return nil
}
