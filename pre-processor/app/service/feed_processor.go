package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"pre-processor/repository"
	"pre-processor/utils"
	utilsLogger "pre-processor/utils/logger"
)

// FeedProcessorService implementation.
type feedProcessorService struct {
	feedRepo          repository.FeedRepository
	articleRepo       repository.ArticleRepository
	fetcher           ArticleFetcherService
	logger            *slog.Logger
	contextLogger     *utilsLogger.ContextLogger
	performanceLogger *utilsLogger.PerformanceLogger
	cursor            *repository.Cursor
	workerPool        *utils.FeedWorkerPool
}

// NewFeedProcessorService creates a new feed processor service.
func NewFeedProcessorService(
	feedRepo repository.FeedRepository,
	articleRepo repository.ArticleRepository,
	fetcher ArticleFetcherService,
	logger *slog.Logger,
) FeedProcessorService {
	// Initialize enhanced logging components
	contextLogger := utilsLogger.NewContextLogger("json", "info")
	performanceLogger := utilsLogger.NewPerformanceLogger(5 * time.Second)

	// Get worker count from environment variable
	workerCount := 3 // default
	if envWorkers := os.Getenv("FEED_WORKER_COUNT"); envWorkers != "" {
		if count, err := strconv.Atoi(envWorkers); err == nil && count > 0 {
			workerCount = count
		}
	}
	
	workerPool := utils.NewFeedWorkerPool(workerCount, 100, logger)

	return &feedProcessorService{
		feedRepo:          feedRepo,
		articleRepo:       articleRepo,
		fetcher:           fetcher,
		logger:            logger,
		contextLogger:     contextLogger,
		performanceLogger: performanceLogger,
		cursor:            &repository.Cursor{},
		workerPool:        workerPool,
	}
}

// ProcessFeeds processes a batch of feeds.
func (s *feedProcessorService) ProcessFeeds(ctx context.Context, batchSize int) (*ProcessingResult, error) {
	// Add operation context and start timing
	ctx = utilsLogger.WithOperation(ctx, "process_batch")
	timer := s.performanceLogger.StartTimer(ctx, "process_batch")
	defer timer.End()

	log := s.contextLogger.WithContext(ctx)
	log.Info("starting batch processing", "batch_size", batchSize)

	// Legacy logger for backward compatibility
	s.logger.Info("Starting feed processing", "batch_size", batchSize)

	// Get unprocessed feeds
	urls, cursor, err := s.feedRepo.GetUnprocessedFeeds(ctx, s.cursor, batchSize)
	if err != nil {
		s.logger.Error("Failed to get unprocessed feeds", "error", err)
		return nil, fmt.Errorf("failed to get unprocessed feeds: %w", err)
	}

	if len(urls) == 0 {
		s.logger.Info("No unprocessed feeds found")

		return &ProcessingResult{
			ProcessedCount: 0,
			SuccessCount:   0,
			ErrorCount:     0,
			Errors:         []error{},
			HasMore:        false,
		}, nil
	}

	// Update cursor for next batch
	s.cursor = cursor

	// Convert URLs to strings for existence check
	urlStrings := make([]string, len(urls))
	for i, url := range urls {
		urlStrings[i] = url.String()
	}

	// Check if articles already exist
	exists, err := s.articleRepo.CheckExists(ctx, urlStrings)
	if err != nil {
		s.logger.Error("Failed to check article existence", "error", err)
		return nil, fmt.Errorf("failed to check article existence: %w", err)
	}

	if exists {
		s.logger.Info("Articles already exist for this batch")

		return &ProcessingResult{
			ProcessedCount: 0,
			SuccessCount:   0,
			ErrorCount:     0,
			Errors:         []error{},
			HasMore:        false,
		}, nil
	}

	// Process URLs in parallel using worker pool
	jobs := make([]utils.FeedJob, len(urls))
	for i, url := range urls {
		jobs[i] = utils.FeedJob{URL: url.String()}
	}

	s.logger.Info("Starting parallel feed processing", 
		"feed_count", len(jobs),
		"worker_count", s.workerPool.Workers())

	results := s.workerPool.ProcessFeeds(ctx, jobs, s.fetcher)

	// Process results and save articles
	var successCount, errorCount int
	var errors []error

	for _, result := range results {
		if result.Error != nil {
			s.logger.Error("Failed to fetch article", "url", result.Job.URL, "error", result.Error)
			errorCount++
			errors = append(errors, result.Error)
			continue
		}

		if result.Article == nil {
			s.logger.Info("Article was skipped", "url", result.Job.URL)
			continue
		}

		// Save article
		if err := s.articleRepo.Create(ctx, result.Article); err != nil {
			s.logger.Error("Failed to save article", "url", result.Job.URL, "error", err)
			errorCount++
			errors = append(errors, err)
			continue
		}

		successCount++
		s.logger.Info("Successfully processed article", "url", result.Job.URL)
	}

	result := &ProcessingResult{
		ProcessedCount: len(urls),
		SuccessCount:   successCount,
		ErrorCount:     errorCount,
		Errors:         errors,
		HasMore:        len(urls) == batchSize, // Has more if we got a full batch
	}

	// Enhanced structured logging
	log.Info("batch processing completed",
		"processed", result.ProcessedCount,
		"success", result.SuccessCount,
		"errors", result.ErrorCount,
		"has_more", result.HasMore)

	// Legacy logger for backward compatibility
	s.logger.Info("Feed processing completed",
		"processed", result.ProcessedCount,
		"success", result.SuccessCount,
		"errors", result.ErrorCount,
		"has_more", result.HasMore)

	return result, nil
}

// GetProcessingStats returns current processing statistics.
func (s *feedProcessorService) GetProcessingStats(ctx context.Context) (*ProcessingStats, error) {
	s.logger.Info("Getting processing statistics")

	repoStats, err := s.feedRepo.GetProcessingStats(ctx)
	if err != nil {
		s.logger.Error("Failed to get processing statistics", "error", err)
		return nil, fmt.Errorf("failed to get processing statistics: %w", err)
	}

	stats := &ProcessingStats{
		TotalFeeds:     repoStats.TotalFeeds,
		ProcessedFeeds: repoStats.ProcessedFeeds,
		RemainingFeeds: repoStats.RemainingFeeds,
	}

	s.logger.Info("Processing statistics retrieved",
		"total", stats.TotalFeeds,
		"processed", stats.ProcessedFeeds,
		"remaining", stats.RemainingFeeds)

	return stats, nil
}

// ResetPagination resets the pagination cursor.
func (s *feedProcessorService) ResetPagination() error {
	s.logger.Info("Resetting pagination cursor")
	s.cursor = &repository.Cursor{}
	s.logger.Info("Pagination cursor reset")

	return nil
}
