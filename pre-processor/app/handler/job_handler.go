package handler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"pre-processor/domain"
	"pre-processor/orchestrator"
	"pre-processor/service"
)

// jobHandler implementation.
type jobHandler struct {
	articleSummarizer service.ArticleSummarizerService
	qualityChecker    service.QualityCheckerService
	articleSync       service.ArticleSyncService
	healthChecker     service.HealthCheckerService
	queueWorker       *service.SummarizeQueueWorker
	logger            *slog.Logger
	jobGroup          *orchestrator.JobGroup
	batchSize         int
}

// NewJobHandler creates a new job handler.
func NewJobHandler(
	ctx context.Context,
	articleSummarizer service.ArticleSummarizerService,
	qualityChecker service.QualityCheckerService,
	articleSync service.ArticleSyncService,
	healthChecker service.HealthCheckerService,
	queueWorker *service.SummarizeQueueWorker,
	batchSize int,
	logger *slog.Logger,
) JobHandler {
	return &jobHandler{
		articleSummarizer: articleSummarizer,
		qualityChecker:    qualityChecker,
		articleSync:       articleSync,
		healthChecker:     healthChecker,
		queueWorker:       queueWorker,
		logger:            logger,
		jobGroup:          orchestrator.NewJobGroup(ctx, logger),
		batchSize:         batchSize,
	}
}

// StartArticleSyncJob starts the article synchronization job.
func (h *jobHandler) StartArticleSyncJob(ctx context.Context) error {
	h.logger.InfoContext(ctx, "starting article sync job")

	h.jobGroup.Add(orchestrator.NewJobRunner(orchestrator.JobConfig{
		Name:           "article-sync",
		Interval:       1 * time.Hour,
		RunImmediately: true,
	}, func(ctx context.Context) error {
		return h.articleSync.SyncArticles(ctx)
	}, h.logger))

	return nil
}

// StartSummarizationJob starts the article summarization job.
func (h *jobHandler) StartSummarizationJob(ctx context.Context) error {
	h.logger.InfoContext(ctx, "starting summarization job")

	// Wait for news creator to be healthy
	if err := h.healthChecker.WaitForHealthy(ctx); err != nil {
		h.logger.ErrorContext(ctx, "failed to wait for news creator health", "error", err)
		return fmt.Errorf("failed to wait for news creator health: %w", err)
	}

	h.jobGroup.Add(orchestrator.NewJobRunner(orchestrator.JobConfig{
		Name:     "summarization",
		Interval: 5 * time.Minute, // Fallback safety net; primary path is event-driven via ArticleCreated events
	}, func(ctx context.Context) error {
		return h.processSummarizationBatch(ctx)
	}, h.logger))

	return nil
}

// StartQualityCheckJob starts the quality check job.
func (h *jobHandler) StartQualityCheckJob(ctx context.Context) error {
	h.logger.InfoContext(ctx, "starting quality check job")

	// Wait for news creator to be healthy
	if err := h.healthChecker.WaitForHealthy(ctx); err != nil {
		h.logger.ErrorContext(ctx, "failed to wait for news creator health", "error", err)
		return fmt.Errorf("failed to wait for news creator health: %w", err)
	}

	h.jobGroup.Add(orchestrator.NewJobRunner(orchestrator.JobConfig{
		Name:     "quality-check",
		Interval: 5 * time.Minute,
	}, func(ctx context.Context) error {
		return h.processQualityCheckBatch(ctx)
	}, h.logger))

	return nil
}

// StartSummarizeQueueWorker starts the summarize queue worker job.
func (h *jobHandler) StartSummarizeQueueWorker(ctx context.Context) error {
	if h.queueWorker == nil {
		h.logger.WarnContext(ctx, "queue worker is nil, skipping start")
		return nil
	}

	h.logger.InfoContext(ctx, "starting summarize queue worker")

	// Wait for news creator to be healthy
	if err := h.healthChecker.WaitForHealthy(ctx); err != nil {
		h.logger.ErrorContext(ctx, "failed to wait for news creator health", "error", err)
		return fmt.Errorf("failed to wait for news creator health: %w", err)
	}

	h.jobGroup.Add(orchestrator.NewJobRunner(orchestrator.JobConfig{
		Name:            "queue-worker",
		Interval:        10 * time.Second,
		InitialBackoff:  30 * time.Second,
		MaxBackoff:      5 * time.Minute,
		BackoffOnErrors: []error{domain.ErrServiceOverloaded},
	}, func(ctx context.Context) error {
		return h.queueWorker.ProcessQueue(ctx)
	}, h.logger))

	return nil
}

// Stop stops all jobs.
func (h *jobHandler) Stop() error {
	h.logger.Info("stopping all jobs")
	h.jobGroup.StopAll()
	h.logger.Info("all jobs stopped")
	return nil
}

// processSummarizationBatch processes a batch of articles for summarization.
func (h *jobHandler) processSummarizationBatch(ctx context.Context) error {
	result, err := h.articleSummarizer.SummarizeArticles(ctx, h.batchSize)
	if err != nil {
		return err
	}

	h.logger.InfoContext(ctx, "summarization completed",
		"processed", result.ProcessedCount,
		"success", result.SuccessCount,
		"errors", result.ErrorCount,
		"has_more", result.HasMore)

	if !result.HasMore && result.ProcessedCount > 0 {
		h.logger.InfoContext(ctx, "reached end of articles, resetting pagination cursor")
		if err := h.articleSummarizer.ResetPagination(); err != nil {
			h.logger.ErrorContext(ctx, "failed to reset summarizer pagination", "error", err)
		}
	}

	return nil
}

// processQualityCheckBatch processes a batch of articles for quality checking.
func (h *jobHandler) processQualityCheckBatch(ctx context.Context) error {
	result, err := h.qualityChecker.CheckQuality(ctx, h.batchSize)
	if err != nil {
		return err
	}

	h.logger.InfoContext(ctx, "quality check completed",
		"processed", result.ProcessedCount,
		"success", result.SuccessCount,
		"errors", result.ErrorCount,
		"removed", result.RemovedCount,
		"retained", result.RetainedCount,
		"has_more", result.HasMore)

	if !result.HasMore && result.ProcessedCount > 0 {
		h.logger.InfoContext(ctx, "reached end of articles, resetting pagination cursor")
		if err := h.qualityChecker.ResetPagination(); err != nil {
			h.logger.ErrorContext(ctx, "failed to reset quality checker pagination", "error", err)
		}
	}

	return nil
}
