package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"pre-processor/models"
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
		w.logger.Error("failed to get pending jobs", "error", err)
		return fmt.Errorf("failed to get pending jobs: %w", err)
	}

	if len(jobs) == 0 {
		w.logger.Debug("no pending jobs to process")
		return nil
	}

	w.logger.Info("processing queued summarization jobs", "count", len(jobs))

	// Process each job
	for _, job := range jobs {
		if err := w.processJob(ctx, job); err != nil {
			w.logger.Error("failed to process job", "error", err, "job_id", job.JobID, "article_id", job.ArticleID)
			// Continue processing other jobs even if one fails
			continue
		}
	}

	return nil
}

// processJob processes a single summarization job
func (w *SummarizeQueueWorker) processJob(ctx context.Context, job *models.SummarizeJob) error {
	// Update status to running
	if err := w.jobRepo.UpdateJobStatus(ctx, job.JobID.String(), models.SummarizeJobStatusRunning, "", ""); err != nil {
		w.logger.Error("failed to update job status to running", "error", err, "job_id", job.JobID)
		return fmt.Errorf("failed to update job status: %w", err)
	}

	w.logger.Info("processing summarization job", "job_id", job.JobID, "article_id", job.ArticleID)

	// Fetch article from database
	article, err := w.articleRepo.FindByID(ctx, job.ArticleID)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to fetch article: %v", err)
		w.logger.Error("failed to fetch article", "error", err, "article_id", job.ArticleID)
		if updateErr := w.jobRepo.UpdateJobStatus(ctx, job.JobID.String(), models.SummarizeJobStatusFailed, "", errorMsg); updateErr != nil {
			w.logger.Error("failed to update job status to failed", "error", updateErr, "job_id", job.JobID)
		}
		return fmt.Errorf("failed to fetch article: %w", err)
	}

	if article == nil {
		errorMsg := "Article not found in database"
		w.logger.Warn("article not found", "article_id", job.ArticleID)
		if updateErr := w.jobRepo.UpdateJobStatus(ctx, job.JobID.String(), models.SummarizeJobStatusFailed, "", errorMsg); updateErr != nil {
			w.logger.Error("failed to update job status to failed", "error", updateErr, "job_id", job.JobID)
		}
		return fmt.Errorf("article not found: %s", job.ArticleID)
	}

	if article.Content == "" {
		errorMsg := "Article content is empty"
		w.logger.Warn("article content is empty", "article_id", job.ArticleID)
		if updateErr := w.jobRepo.UpdateJobStatus(ctx, job.JobID.String(), models.SummarizeJobStatusFailed, "", errorMsg); updateErr != nil {
			w.logger.Error("failed to update job status to failed", "error", updateErr, "job_id", job.JobID)
		}
		return fmt.Errorf("article content is empty: %s", job.ArticleID)
	}

	// Extract text from HTML if needed
	content := article.Content
	if strings.Contains(content, "<") && strings.Contains(content, ">") {
		w.logger.Info("detected HTML content, extracting text", "article_id", job.ArticleID)
		extractedText := html_parser.ExtractArticleText(content)
		if extractedText != "" {
			content = extractedText
			w.logger.Info("HTML content extracted successfully", "article_id", job.ArticleID, "original_length", len(article.Content), "extracted_length", len(extractedText))
		} else {
			w.logger.Warn("HTML extraction returned empty, using original content", "article_id", job.ArticleID)
		}
	}

	// Create article model for summarization
	articleModel := &models.Article{
		ID:      job.ArticleID,
		Content: content,
	}

	// Call summarization service
	summarizeStartTime := time.Now()
	summarized, err := w.apiRepo.SummarizeArticle(ctx, articleModel)
	summarizeDuration := time.Since(summarizeStartTime)

	if err != nil {
		errorMsg := fmt.Sprintf("Failed to generate summary: %v", err)
		w.logger.Error("failed to summarize article",
			"error", err,
			"article_id", job.ArticleID,
			"duration_ms", summarizeDuration.Milliseconds())

		// Check if job can be retried
		if job.CanRetry() {
			w.logger.Info("job will be retried", "job_id", job.JobID, "retry_count", job.RetryCount+1)
			// Update status back to pending for retry (or keep as failed and let retry logic handle it)
			// For now, mark as failed and let the retry logic handle it
		}

		if updateErr := w.jobRepo.UpdateJobStatus(ctx, job.JobID.String(), models.SummarizeJobStatusFailed, "", errorMsg); updateErr != nil {
			w.logger.Error("failed to update job status to failed", "error", updateErr, "job_id", job.JobID)
		}
		return fmt.Errorf("failed to summarize article: %w", err)
	}

	w.logger.Info("article summarized successfully",
		"job_id", job.JobID,
		"article_id", job.ArticleID,
		"summarize_duration_ms", summarizeDuration.Milliseconds())

	// Save summary to database
	articleTitle := article.Title
	if articleTitle == "" {
		articleTitle = "Untitled"
	}

	articleSummary := &models.ArticleSummary{
		ArticleID:       job.ArticleID,
		UserID:          article.UserID,
		ArticleTitle:    articleTitle,
		SummaryJapanese: summarized.SummaryJapanese,
	}

	saveSummaryStartTime := time.Now()
	if err := w.summaryRepo.Create(ctx, articleSummary); err != nil {
		w.logger.Error("failed to save summary to database", "error", err, "article_id", job.ArticleID)
		// Continue even if save fails - we still have the summary to return
		w.logger.Warn("continuing despite DB save failure", "article_id", job.ArticleID)
	} else {
		saveSummaryDuration := time.Since(saveSummaryStartTime)
		w.logger.Info("summary saved to database successfully",
			"article_id", job.ArticleID,
			"save_duration_ms", saveSummaryDuration.Milliseconds())
	}

	// Update job status to completed
	updateStatusStartTime := time.Now()
	if err := w.jobRepo.UpdateJobStatus(ctx, job.JobID.String(), models.SummarizeJobStatusCompleted, summarized.SummaryJapanese, ""); err != nil {
		w.logger.Error("failed to update job status to completed", "error", err, "job_id", job.JobID)
		return fmt.Errorf("failed to update job status: %w", err)
	}
	updateStatusDuration := time.Since(updateStatusStartTime)

	totalDuration := time.Since(summarizeStartTime)
	w.logger.Info("summarization job completed successfully",
		"job_id", job.JobID,
		"article_id", job.ArticleID,
		"update_status_duration_ms", updateStatusDuration.Milliseconds(),
		"total_duration_ms", totalDuration.Milliseconds(),
		"completed_at", time.Now().UnixNano())
	return nil
}
