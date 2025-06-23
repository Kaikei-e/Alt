package service

import (
	"context"
	"fmt"
	"log/slog"
	"pre-processor/repository"
)

// FeedProcessorService implementation
type feedProcessorService struct {
	feedRepo    repository.FeedRepository
	articleRepo repository.ArticleRepository
	fetcher     ArticleFetcherService
	logger      *slog.Logger
	cursor      *repository.Cursor
}

// NewFeedProcessorService creates a new feed processor service
func NewFeedProcessorService(
	feedRepo repository.FeedRepository,
	articleRepo repository.ArticleRepository,
	fetcher ArticleFetcherService,
	logger *slog.Logger,
) FeedProcessorService {
	return &feedProcessorService{
		feedRepo:    feedRepo,
		articleRepo: articleRepo,
		fetcher:     fetcher,
		logger:      logger,
		cursor:      &repository.Cursor{},
	}
}

// ProcessFeeds processes a batch of feeds
func (s *feedProcessorService) ProcessFeeds(ctx context.Context, batchSize int) (*ProcessingResult, error) {
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

	// Process each URL
	var successCount, errorCount int
	var errors []error

	for _, url := range urls {
		s.logger.Info("Processing feed", "url", url.String())

		// Fetch article
		article, err := s.fetcher.FetchArticle(ctx, url.String())
		if err != nil {
			s.logger.Error("Failed to fetch article", "url", url.String(), "error", err)
			errorCount++
			errors = append(errors, err)
			continue
		}

		if article == nil {
			s.logger.Info("Article was skipped", "url", url.String())
			continue
		}

		// Save article
		if err := s.articleRepo.Create(ctx, article); err != nil {
			s.logger.Error("Failed to save article", "url", url.String(), "error", err)
			errorCount++
			errors = append(errors, err)
			continue
		}

		successCount++
		s.logger.Info("Successfully processed article", "url", url.String())
	}

	result := &ProcessingResult{
		ProcessedCount: len(urls),
		SuccessCount:   successCount,
		ErrorCount:     errorCount,
		Errors:         errors,
		HasMore:        len(urls) == batchSize, // Has more if we got a full batch
	}

	s.logger.Info("Feed processing completed",
		"processed", result.ProcessedCount,
		"success", result.SuccessCount,
		"errors", result.ErrorCount,
		"has_more", result.HasMore)

	return result, nil
}

// GetProcessingStats returns current processing statistics
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

// ResetPagination resets the pagination cursor
func (s *feedProcessorService) ResetPagination() error {
	s.logger.Info("Resetting pagination cursor")
	s.cursor = &repository.Cursor{}
	s.logger.Info("Pagination cursor reset")
	return nil
}
