// ABOUTME: Enhanced feed processor with OperationError integration and retry mechanisms
// ABOUTME: Provides circuit breaker protection and dead letter queue for failed operations
package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"time"

	"pre-processor/models"
	"pre-processor/repository"
	"pre-processor/utils"
	operrors "pre-processor/utils/errors"
	utilsLogger "pre-processor/utils/logger"
)

// FeedMetricsProcessorService extends the basic feed processor with enhanced error handling
type FeedMetricsProcessorService interface {
	FeedProcessorService
	ProcessFeedsWithRetry(ctx context.Context, batchSize int) (*ProcessingResult, error)
	GetDeadLetterQueueMetrics() utils.DeadLetterMetrics
	GetCircuitBreakerMetrics() utils.CircuitBreakerMetrics
	ProcessFailedItems(ctx context.Context) error
}

// FeedsProcessorService implementation with error handling enhancements
type FeedsProcessorService struct {
	feedRepo          repository.FeedRepository
	articleRepo       repository.ArticleRepository
	fetcher           ArticleFetcherService
	logger            *slog.Logger
	contextLogger     *utilsLogger.ContextLogger
	performanceLogger *utilsLogger.PerformanceLogger
	cursor            *repository.Cursor

	// Enhanced error handling components
	retryPolicy     *operrors.RetryPolicy
	retryExecutor   *operrors.RetryExecutor
	circuitBreaker  *utils.CircuitBreaker
	deadLetterQueue *utils.DeadLetterQueue
}

// NewEnhancedFeedProcessorService creates a new enhanced feed processor service
func NewFeedMetricsProcessorService(
	feedRepo repository.FeedRepository,
	articleRepo repository.ArticleRepository,
	fetcher ArticleFetcherService,
	logger *slog.Logger,
) FeedMetricsProcessorService {
	// Initialize advanced logging with metrics components
	contextLogger := utilsLogger.NewContextLogger(os.Stdout, "json", "info")
	performanceLogger := utilsLogger.NewPerformanceLogger(os.Stdout, 5*time.Second)

	// Initialize retry policy (3 attempts, 1 second base delay)
	retryPolicy := operrors.NewRetryPolicy(3, 1*time.Second)
	retryExecutor := operrors.NewRetryExecutor(retryPolicy)

	// Initialize circuit breaker (5 failures threshold, 30 second timeout)
	circuitBreaker := utils.NewCircuitBreaker(5, 30*time.Second)

	// Initialize dead letter queue
	deadLetterQueue := utils.NewDeadLetterQueue(logger)

	return &FeedsProcessorService{
		feedRepo:          feedRepo,
		articleRepo:       articleRepo,
		fetcher:           fetcher,
		logger:            logger,
		contextLogger:     contextLogger,
		performanceLogger: performanceLogger,
		cursor:            &repository.Cursor{},
		retryPolicy:       retryPolicy,
		retryExecutor:     retryExecutor,
		circuitBreaker:    circuitBreaker,
		deadLetterQueue:   deadLetterQueue,
	}
}

// ProcessFeedsWithRetry processes feeds with enhanced error handling and retry logic
func (s *FeedsProcessorService) ProcessFeedsWithRetry(ctx context.Context, batchSize int) (*ProcessingResult, error) {
	// Add operation context and start timing
	ctx = utilsLogger.WithOperation(ctx, "process_batch_with_retry")
	timer := s.performanceLogger.StartTimer(ctx, "process_batch_with_retry")
	defer timer.End()

	log := s.contextLogger.WithContext(ctx)
	log.Info("starting enhanced batch processing", "batch_size", batchSize)

	// Get unprocessed feeds
	urls, cursor, err := s.feedRepo.GetUnprocessedFeeds(ctx, s.cursor, batchSize)
	if err != nil {
		opErr := operrors.NewOperationError("get_unprocessed_feeds", err, false).WithContext(ctx)
		s.logger.Error("Failed to get unprocessed feeds", "error", opErr)
		return nil, opErr
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
		opErr := operrors.NewOperationError("check_article_existence", err, false).WithContext(ctx)
		s.logger.Error("Failed to check article existence", "error", opErr)
		return nil, opErr
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

	// Process each URL with enhanced error handling
	var successCount, errorCount int
	var errors []error

	for i, url := range urls {
		urlStr := url.String()

		// Add URL-specific context
		urlCtx := operrors.WithRequestID(ctx, fmt.Sprintf("url-%d", i))

		s.logger.Info("Processing feed with retry", "url", urlStr)

		// Process single URL with circuit breaker protection
		err := s.circuitBreaker.Call(func() error {
			return s.processSingleFeedWithRetry(urlCtx, urlStr)
		})

		if err != nil {
			errorCount++

			// Check if it's a circuit breaker error
			if err.Error() == "circuit breaker open" {
				opErr := operrors.NewOperationError("circuit_breaker_open", err, false).WithContext(urlCtx)
				errors = append(errors, opErr)
				s.logger.Warn("Circuit breaker open, skipping remaining feeds", "url", urlStr)
				break
			}

			// Add to errors list
			if opErr, ok := err.(*operrors.OperationError); ok {
				errors = append(errors, opErr)

				// Add persistent failures to dead letter queue
				if opErr.Retryable {
					s.deadLetterQueue.AddOperationError(
						fmt.Sprintf("feed-%s-%d", urlStr, time.Now().Unix()),
						&url,
						opErr,
						3, // Max retries in DLQ
					)
				}
			} else {
				// Wrap regular errors
				opErr := operrors.NewOperationError("process_feed", err, false).WithContext(urlCtx)
				errors = append(errors, opErr)
			}

			continue
		}

		successCount++
		s.logger.Info("Successfully processed article", "url", urlStr)
	}

	result := &ProcessingResult{
		ProcessedCount: len(urls),
		SuccessCount:   successCount,
		ErrorCount:     errorCount,
		Errors:         errors,
		HasMore:        len(urls) == batchSize,
	}

	// Enhanced structured logging
	log.Info("enhanced batch processing completed",
		"processed", result.ProcessedCount,
		"success", result.SuccessCount,
		"errors", result.ErrorCount,
		"has_more", result.HasMore,
		"circuit_breaker_state", s.circuitBreaker.State(),
		"dlq_items", s.deadLetterQueue.Metrics().TotalItems)

	return result, nil
}

// processSingleFeedWithRetry processes a single feed with retry logic
func (s *FeedsProcessorService) processSingleFeedWithRetry(ctx context.Context, url string) error {
	operation := func() error {
		// Fetch article
		article, err := s.fetchArticleWithContext(ctx, url)
		if err != nil {
			return err // Will be retried if retryable
		}

		if article == nil {
			s.logger.Info("Article was skipped", "url", url)
			return nil
		}

		// Save article
		if err := s.articleRepo.Create(ctx, article); err != nil {
			// Classify the error
			errorType := operrors.ClassifyError(err)
			retryable := errorType == operrors.ErrorTypeTransient
			return operrors.NewOperationError("save_article", err, retryable).WithContext(ctx)
		}

		return nil
	}

	return s.retryExecutor.Execute(ctx, operation)
}

// fetchArticleWithContext wraps the fetcher with proper error context
func (s *FeedsProcessorService) fetchArticleWithContext(ctx context.Context, url string) (*models.Article, error) {
	// Try to use the enhanced fetcher interface if available
	if enhancedFetcher, ok := s.fetcher.(interface {
		FetchArticleWithContext(context.Context, string) (*models.Article, error)
	}); ok {
		return enhancedFetcher.FetchArticleWithContext(ctx, url)
	}

	// Fall back to regular fetcher and wrap errors
	article, err := s.fetcher.FetchArticle(ctx, url)
	if err != nil {
		// Classify and wrap the error
		errorType := operrors.ClassifyError(err)
		retryable := errorType == operrors.ErrorTypeTransient
		return nil, operrors.NewOperationError("fetch_article", err, retryable).WithContext(ctx)
	}

	return article, nil
}

// ProcessFeeds implements the original interface for backward compatibility
func (s *FeedsProcessorService) ProcessFeeds(ctx context.Context, batchSize int) (*ProcessingResult, error) {
	return s.ProcessFeedsWithRetry(ctx, batchSize)
}

// GetProcessingStats returns current processing statistics
func (s *FeedsProcessorService) GetProcessingStats(ctx context.Context) (*ProcessingStats, error) {
	s.logger.Info("Getting processing statistics")

	repoStats, err := s.feedRepo.GetProcessingStats(ctx)
	if err != nil {
		opErr := operrors.NewOperationError("get_processing_stats", err, false).WithContext(ctx)
		s.logger.Error("Failed to get processing statistics", "error", opErr)
		return nil, opErr
	}

	return &ProcessingStats{
		TotalFeeds:     repoStats.TotalFeeds,
		ProcessedFeeds: repoStats.ProcessedFeeds,
		RemainingFeeds: repoStats.RemainingFeeds,
	}, nil
}

// ResetPagination resets the pagination cursor
func (s *FeedsProcessorService) ResetPagination() error {
	s.cursor = &repository.Cursor{}
	s.logger.Info("Pagination cursor reset")
	return nil
}

// GetDeadLetterQueueMetrics returns metrics for the dead letter queue
func (s *FeedsProcessorService) GetDeadLetterQueueMetrics() utils.DeadLetterMetrics {
	return s.deadLetterQueue.Metrics()
}

// GetCircuitBreakerMetrics returns metrics for the circuit breaker
func (s *FeedsProcessorService) GetCircuitBreakerMetrics() utils.CircuitBreakerMetrics {
	return s.circuitBreaker.Metrics()
}

// ProcessFailedItems processes items from the dead letter queue
func (s *FeedsProcessorService) ProcessFailedItems(ctx context.Context) error {
	processor := func(ctx context.Context, item *utils.DeadLetterItem) error {
		if feedURL, ok := item.Data.(*url.URL); ok {
			return s.processSingleFeedWithRetry(ctx, feedURL.String())
		}
		return fmt.Errorf("invalid item type in dead letter queue: %T", item.Data)
	}

	s.deadLetterQueue.ProcessRetriesWithExecutor(ctx, s.retryExecutor, processor)
	return nil
}
