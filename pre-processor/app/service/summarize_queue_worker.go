package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"pre-processor/domain"
	"pre-processor/repository"
	"pre-processor/utils/html_parser"
)

// Placeholder summaries saved when content cannot be summarized.
// These must match the knownPlaceholders in quality-checker/quality_judger.go
// so that the quality checker does not attempt to delete them.
const (
	placeholderTooShort = "本文が短すぎるため要約できませんでした。"
	placeholderTooLong  = "本文が長すぎるため要約できませんでした。"
)

// EnqueueResult represents the result of enqueuing unsummarized articles.
type EnqueueResult struct {
	Found    int
	Enqueued int
	Skipped  int
	Errors   int
	HasMore  bool
}

// SummarizeQueueWorker handles processing of queued summarization jobs
type SummarizeQueueWorker struct {
	jobRepo         repository.SummarizeJobRepository
	articleRepo     repository.ArticleRepository
	apiRepo         repository.ExternalAPIRepository
	summaryRepo     repository.SummaryRepository
	logger          *slog.Logger
	batchSize       int
	concurrency     int
	lastRecoveryRun time.Time
	enqueueCursor   *domain.Cursor
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
		concurrency: 1,
	}
}

// SetConcurrency configures how many queue jobs can be processed concurrently.
func (w *SummarizeQueueWorker) SetConcurrency(concurrency int) {
	if concurrency <= 0 {
		w.concurrency = 1
		return
	}
	w.concurrency = concurrency
}

// HasPendingJobs checks if there are any pending summarization jobs in the queue.
func (w *SummarizeQueueWorker) HasPendingJobs(ctx context.Context) (bool, error) {
	jobs, err := w.jobRepo.GetPendingJobs(ctx, 1)
	if err != nil {
		return false, fmt.Errorf("failed to check pending jobs: %w", err)
	}
	return len(jobs) > 0, nil
}

// RecoverStuckJobs recovers jobs stuck in 'running' state, throttled to once per 5 minutes.
func (w *SummarizeQueueWorker) RecoverStuckJobs(ctx context.Context) {
	if time.Since(w.lastRecoveryRun) < 5*time.Minute {
		return
	}
	w.lastRecoveryRun = time.Now()

	recovered, err := w.jobRepo.RecoverStuckJobs(ctx)
	if err != nil {
		w.logger.ErrorContext(ctx, "failed to recover stuck jobs", "error", err)
		return
	}
	if recovered > 0 {
		w.logger.WarnContext(ctx, "recovered stuck running jobs", "count", recovered)
	}
}

// ProcessQueue processes pending jobs from the queue
func (w *SummarizeQueueWorker) ProcessQueue(ctx context.Context) error {
	// Recover stuck running jobs (throttled to once per 5 minutes)
	w.RecoverStuckJobs(ctx)

	// Atomically dequeue pending jobs → running in a single transaction
	jobs, err := w.jobRepo.DequeueJobs(ctx, w.batchSize)
	if err != nil {
		w.logger.ErrorContext(ctx, "failed to dequeue jobs", "error", err)
		return fmt.Errorf("failed to dequeue jobs: %w", err)
	}

	if len(jobs) == 0 {
		w.logger.DebugContext(ctx, "no pending jobs to process")
		return nil
	}

	w.logger.InfoContext(ctx, "processing queued summarization jobs", "count", len(jobs))

	workerCount := w.concurrency
	if workerCount <= 0 {
		workerCount = 1
	}
	if workerCount > len(jobs) {
		workerCount = len(jobs)
	}

	jobCh := make(chan *domain.SummarizeJob)
	var wg sync.WaitGroup
	var overloadOnce sync.Once
	var overloaded atomic.Bool

	workerFn := func() {
		defer wg.Done()
		for job := range jobCh {
			if ctx.Err() != nil || overloaded.Load() {
				continue
			}

			if err := w.processJob(ctx, job); err != nil {
				if errors.Is(err, domain.ErrServiceOverloaded) {
					overloadOnce.Do(func() {
						overloaded.Store(true)
						w.logger.WarnContext(ctx, "downstream service overloaded, backing off queue worker",
							"job_id", job.JobID,
							"article_id", job.ArticleID)
					})
					continue
				}

				w.logger.ErrorContext(ctx, "failed to process job", "error", err, "job_id", job.JobID, "article_id", job.ArticleID)
			}
		}
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go workerFn()
	}

	for i, job := range jobs {
		if ctx.Err() != nil {
			w.logger.WarnContext(ctx, "context canceled, skipping remaining jobs",
				"remaining", len(jobs)-i)
			break
		}
		if overloaded.Load() {
			w.logger.WarnContext(ctx, "downstream service overloaded, skipping remaining queued jobs",
				"remaining", len(jobs)-i)
			break
		}
		jobCh <- job
	}

	close(jobCh)
	wg.Wait()

	if overloaded.Load() {
		return domain.ErrServiceOverloaded
	}

	return nil
}

// processJob processes a single summarization job.
// The job is already in "running" status (set atomically by DequeueJobs).
func (w *SummarizeQueueWorker) processJob(ctx context.Context, job *domain.SummarizeJob) error {
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

		// Handle non-retryable content errors
		if errors.Is(err, domain.ErrContentTooShort) {
			// Short content is expected for RSS feeds that only provide excerpts — skip gracefully.
			// Save a placeholder summary so the article is excluded from the Unsummarized count
			// and not re-enqueued indefinitely.
			w.logger.InfoContext(ctx, "skipping short content article",
				"job_id", job.JobID,
				"article_id", job.ArticleID,
				"duration_ms", summarizeDuration.Milliseconds())
			w.savePlaceholderSummary(ctx, job.ArticleID, article.UserID, article.Title, placeholderTooShort)
			if updateErr := w.jobRepo.UpdateJobStatus(ctx, job.JobID.String(), domain.SummarizeJobStatusCompleted, "", "skipped: content too short for summarization"); updateErr != nil {
				w.logger.ErrorContext(ctx, "failed to update job status", "error", updateErr, "job_id", job.JobID)
			}
			return nil
		}
		if errors.Is(err, domain.ErrContentTooLong) {
			// Content exceeds max length — save placeholder instead of retrying.
			w.logger.InfoContext(ctx, "skipping long content article",
				"job_id", job.JobID,
				"article_id", job.ArticleID,
				"duration_ms", summarizeDuration.Milliseconds())
			w.savePlaceholderSummary(ctx, job.ArticleID, article.UserID, article.Title, placeholderTooLong)
			if updateErr := w.jobRepo.UpdateJobStatus(ctx, job.JobID.String(), domain.SummarizeJobStatusCompleted, "", "skipped: content too long for summarization"); updateErr != nil {
				w.logger.ErrorContext(ctx, "failed to update job status", "error", updateErr, "job_id", job.JobID)
			}
			return nil
		}
		if errors.Is(err, domain.ErrContentNotProcessable) {
			w.logger.WarnContext(ctx, "non-retryable summarization error, moving to dead_letter immediately",
				"job_id", job.JobID,
				"article_id", job.ArticleID,
				"error", err,
				"duration_ms", summarizeDuration.Milliseconds())
			if updateErr := w.jobRepo.UpdateJobStatus(ctx, job.JobID.String(), domain.SummarizeJobStatusDeadLetter, "", errorMsg); updateErr != nil {
				w.logger.ErrorContext(ctx, "failed to update job status to dead_letter", "error", updateErr, "job_id", job.JobID)
			}
			return nil // Non-retryable, don't propagate error to skip remaining jobs
		}

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
		// Summary save failed — mark job as failed so it can be retried.
		// Do NOT mark completed without a persisted summary.
		errorMsg := fmt.Sprintf("Failed to save summary: %v", err)
		w.logger.ErrorContext(ctx, "failed to save summary to database", "error", err, "article_id", job.ArticleID)
		if updateErr := w.jobRepo.UpdateJobStatus(ctx, job.JobID.String(), domain.SummarizeJobStatusFailed, "", errorMsg); updateErr != nil {
			w.logger.ErrorContext(ctx, "failed to update job status to failed after save failure", "error", updateErr, "job_id", job.JobID)
		}
		return fmt.Errorf("failed to save summary: %w", err)
	}
	saveSummaryDuration := time.Since(saveSummaryStartTime)
	w.logger.InfoContext(ctx, "summary saved to database successfully",
		"article_id", job.ArticleID,
		"save_duration_ms", saveSummaryDuration.Milliseconds())

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

// savePlaceholderSummary saves a placeholder summary for articles that cannot be
// summarized (content too short or too long). This prevents the article from being
// re-enqueued indefinitely and removes it from the Unsummarized count in Stats.
func (w *SummarizeQueueWorker) savePlaceholderSummary(ctx context.Context, articleID, userID, title, placeholder string) {
	if title == "" {
		title = "Untitled"
	}
	summary := &domain.ArticleSummary{
		ArticleID:       articleID,
		UserID:          userID,
		ArticleTitle:    title,
		SummaryJapanese: placeholder,
	}
	if err := w.summaryRepo.Create(ctx, summary); err != nil {
		w.logger.ErrorContext(ctx, "failed to save placeholder summary",
			"error", err,
			"article_id", articleID,
			"placeholder", placeholder)
	} else {
		w.logger.InfoContext(ctx, "placeholder summary saved",
			"article_id", articleID,
			"placeholder", placeholder)
	}
}

// EnqueueUnsummarizedBatch fetches unsummarized articles from the backend and
// enqueues them into the job queue via the standard guard + CreateJob path.
// This replaces the old batch safety-net that called the LLM directly.
func (w *SummarizeQueueWorker) EnqueueUnsummarizedBatch(ctx context.Context, batchSize int) (*EnqueueResult, error) {
	articles, newCursor, err := w.articleRepo.FindForSummarization(ctx, w.enqueueCursor, batchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to find unsummarized articles: %w", err)
	}

	result := &EnqueueResult{
		Found:   len(articles),
		HasMore: newCursor != nil,
	}

	for _, article := range articles {
		if ctx.Err() != nil {
			break
		}

		shouldQueue, reason, guardErr := ShouldQueueSummarizeJob(ctx, article.ID, w.summaryRepo, w.jobRepo, w.logger)
		if guardErr != nil {
			w.logger.ErrorContext(ctx, "guard check failed", "article_id", article.ID, "error", guardErr)
			result.Errors++
			continue
		}
		if !shouldQueue {
			w.logger.InfoContext(ctx, "batch enqueue skipped by guard", "article_id", article.ID, "reason", reason)
			result.Skipped++
			continue
		}

		jobID, createErr := w.jobRepo.CreateJob(ctx, article.ID)
		if createErr != nil {
			w.logger.ErrorContext(ctx, "failed to create job", "article_id", article.ID, "error", createErr)
			result.Errors++
			continue
		}

		if jobID == "" {
			// Duplicate: pending/running job already exists
			result.Skipped++
			continue
		}

		result.Enqueued++
	}

	if newCursor != nil {
		w.enqueueCursor = newCursor
	}

	w.logger.InfoContext(ctx, "batch enqueue completed",
		"found", result.Found,
		"enqueued", result.Enqueued,
		"skipped", result.Skipped,
		"errors", result.Errors,
		"has_more", result.HasMore)

	return result, nil
}

// ResetEnqueueCursor resets the pagination cursor for unsummarized article scanning.
func (w *SummarizeQueueWorker) ResetEnqueueCursor() {
	w.enqueueCursor = nil
}
