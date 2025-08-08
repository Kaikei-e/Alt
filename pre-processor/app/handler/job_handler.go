package handler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"pre-processor/service"
)

// JobHandler implementation.
type jobHandler struct {
	feedProcessor     service.FeedProcessorService
	articleSummarizer service.ArticleSummarizerService
	qualityChecker    service.QualityCheckerService
	healthChecker     service.HealthCheckerService
	logger            *slog.Logger

	// Job control
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	batchSize int
}

// NewJobHandler creates a new job handler.
func NewJobHandler(
	feedProcessor service.FeedProcessorService,
	articleSummarizer service.ArticleSummarizerService,
	qualityChecker service.QualityCheckerService,
	healthChecker service.HealthCheckerService,
	batchSize int,
	logger *slog.Logger,
) JobHandler {
	ctx, cancel := context.WithCancel(context.Background())

	return &jobHandler{
		feedProcessor:     feedProcessor,
		articleSummarizer: articleSummarizer,
		qualityChecker:    qualityChecker,
		healthChecker:     healthChecker,
		logger:            logger,
		ctx:               ctx,
		cancel:            cancel,
		batchSize:         batchSize,
	}
}

// StartFeedProcessingJob starts the feed processing job.
func (h *jobHandler) StartFeedProcessingJob(ctx context.Context) error {
	h.logger.Info("starting feed processing job")

	h.wg.Add(1)

	go func() {
		defer h.wg.Done()
		h.runFeedProcessingLoop()
	}()

	return nil
}

// StartSummarizationJob starts the article summarization job.
func (h *jobHandler) StartSummarizationJob(ctx context.Context) error {
	h.logger.Info("starting summarization job")

	// Wait for news creator to be healthy
	if err := h.healthChecker.WaitForHealthy(ctx); err != nil {
		h.logger.Error("failed to wait for news creator health", "error", err)
		return fmt.Errorf("failed to wait for news creator health: %w", err)
	}

	h.wg.Add(1)

	go func() {
		defer h.wg.Done()
		h.runSummarizationLoop()
	}()

	return nil
}

// StartQualityCheckJob starts the quality check job.
func (h *jobHandler) StartQualityCheckJob(ctx context.Context) error {
	h.logger.Info("starting quality check job")

	// Wait for news creator to be healthy
	if err := h.healthChecker.WaitForHealthy(ctx); err != nil {
		h.logger.Error("failed to wait for news creator health", "error", err)
		return fmt.Errorf("failed to wait for news creator health: %w", err)
	}

	h.wg.Add(1)

	go func() {
		defer h.wg.Done()
		h.runQualityCheckLoop()
	}()

	return nil
}

// Stop stops all jobs.
func (h *jobHandler) Stop() error {
	h.logger.Info("stopping all jobs")
	h.cancel()
	h.wg.Wait()
	h.logger.Info("all jobs stopped")

	return nil
}

// runFeedProcessingLoop runs the feed processing loop.
func (h *jobHandler) runFeedProcessingLoop() {
	h.logger.Info("runFeedProcessingLoop: Starting feed processing loop goroutine")
	
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	h.logger.Info("runFeedProcessingLoop: Ticker created, waiting for first tick in 5 minutes")

	for {
		select {
		case <-h.ctx.Done():
			h.logger.Info("feed processing job stopped")
			return
		case <-ticker.C:
			h.logger.Info("runFeedProcessingLoop: Ticker fired, calling processFeedsBatch")
			h.processFeedsBatch()
		}
	}
}

// runSummarizationLoop runs the summarization loop.
func (h *jobHandler) runSummarizationLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			h.logger.Info("summarization job stopped")
			return
		case <-ticker.C:
			h.processSummarizationBatch()
		}
	}
}

// runQualityCheckLoop runs the quality check loop.
func (h *jobHandler) runQualityCheckLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			h.logger.Info("quality check job stopped")
			return
		case <-ticker.C:
			h.processQualityCheckBatch()
		}
	}
}

// processFeedsBatch processes a batch of feeds.
func (h *jobHandler) processFeedsBatch() {
	defer func() {
		if r := recover(); r != nil {
			h.logger.Error("panic in processFeedsBatch", "panic", r)
		}
	}()
	
	h.logger.Info("Starting feed processing batch", "batch_size", h.batchSize)
	
	result, err := h.feedProcessor.ProcessFeeds(h.ctx, h.batchSize)
	if err != nil {
		h.logger.Error("feed processing failed", "error", err)
		return
	}

	h.logger.Info("feed processing completed",
		"processed", result.ProcessedCount,
		"success", result.SuccessCount,
		"errors", result.ErrorCount,
		"has_more", result.HasMore)

	// Only reset pagination if we actually processed feeds and reached the end
	// Don't reset if there were simply no feeds to process (ProcessedCount == 0)
	if !result.HasMore && result.ProcessedCount > 0 {
		h.logger.Info("reached end of feeds, resetting pagination cursor")

		if err := h.feedProcessor.ResetPagination(); err != nil {
			h.logger.Error("failed to reset feed processor pagination", "error", err)
		}
	}
}

// processSummarizationBatch processes a batch of articles for summarization.
func (h *jobHandler) processSummarizationBatch() {
	result, err := h.articleSummarizer.SummarizeArticles(h.ctx, h.batchSize)
	if err != nil {
		h.logger.Error("summarization failed", "error", err)
		return
	}

	h.logger.Info("summarization completed",
		"processed", result.ProcessedCount,
		"success", result.SuccessCount,
		"errors", result.ErrorCount,
		"has_more", result.HasMore)

	// Only reset pagination if we actually processed articles and reached the end
	// Don't reset if there were simply no articles to process (ProcessedCount == 0)
	if !result.HasMore && result.ProcessedCount > 0 {
		h.logger.Info("reached end of articles, resetting pagination cursor")

		if err := h.articleSummarizer.ResetPagination(); err != nil {
			h.logger.Error("failed to reset summarizer pagination", "error", err)
		}
	}
}

// processQualityCheckBatch processes a batch of articles for quality checking.
func (h *jobHandler) processQualityCheckBatch() {
	result, err := h.qualityChecker.CheckQuality(h.ctx, h.batchSize)
	if err != nil {
		h.logger.Error("quality check failed", "error", err)
		return
	}

	h.logger.Info("quality check completed",
		"processed", result.ProcessedCount,
		"success", result.SuccessCount,
		"errors", result.ErrorCount,
		"removed", result.RemovedCount,
		"retained", result.RetainedCount,
		"has_more", result.HasMore)

	// Only reset pagination if we actually processed articles and reached the end
	// Don't reset if there were simply no articles to process (ProcessedCount == 0)
	if !result.HasMore && result.ProcessedCount > 0 {
		h.logger.Info("reached end of articles, resetting pagination cursor")

		if err := h.qualityChecker.ResetPagination(); err != nil {
			h.logger.Error("failed to reset quality checker pagination", "error", err)
		}
	}
}
