package service

import (
	"context"
	"log/slog"

	"pre-processor/repository"
)

// feedProcessorService is a stub implementation.
// Feed processing is disabled for ethical compliance.
type feedProcessorService struct {
	feedRepo repository.FeedRepository
	logger   *slog.Logger
}

// NewFeedProcessorService creates a new feed processor service (stub).
func NewFeedProcessorService(
	feedRepo repository.FeedRepository,
	logger *slog.Logger,
) FeedProcessorService {
	return &feedProcessorService{
		feedRepo: feedRepo,
		logger:   logger,
	}
}

// ProcessFeeds is disabled for ethical compliance.
func (s *feedProcessorService) ProcessFeeds(ctx context.Context, batchSize int) (*ProcessingResult, error) {
	s.logger.InfoContext(ctx, "Feed processing disabled for ethical compliance")
	return &ProcessingResult{
		ProcessedCount: 0,
		SuccessCount:   0,
		ErrorCount:     0,
		Errors:         []error{},
		HasMore:        false,
	}, nil
}

// GetProcessingStats returns current processing statistics.
func (s *feedProcessorService) GetProcessingStats(ctx context.Context) (*ProcessingStats, error) {
	repoStats, err := s.feedRepo.GetProcessingStats(ctx)
	if err != nil {
		return nil, err
	}
	return &ProcessingStats{
		TotalFeeds:     repoStats.TotalFeeds,
		ProcessedFeeds: repoStats.ProcessedFeeds,
		RemainingFeeds: repoStats.RemainingFeeds,
	}, nil
}

// ResetPagination is a no-op since feed processing is disabled.
func (s *feedProcessorService) ResetPagination() error {
	return nil
}
