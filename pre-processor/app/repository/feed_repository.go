package repository

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"pre-processor/driver"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FeedRepository implementation.
type feedRepository struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

// NewFeedRepository creates a new feed repository.
func NewFeedRepository(db *pgxpool.Pool, logger *slog.Logger) FeedRepository {
	return &feedRepository{
		db:     db,
		logger: logger,
	}
}

// GetUnprocessedFeeds gets unprocessed feeds using cursor-based pagination.
func (r *feedRepository) GetUnprocessedFeeds(ctx context.Context, cursor *Cursor, limit int) ([]*url.URL, *Cursor, error) {
	r.logger.Info("getting unprocessed feeds", "limit", limit)

	var lastCreatedAt *time.Time

	var lastID string

	if cursor != nil {
		lastCreatedAt = cursor.LastCreatedAt
		lastID = cursor.LastID
	}

	// Use existing driver function
	urls, finalCreatedAt, finalID, err := driver.GetSourceURLs(lastCreatedAt, lastID, ctx, r.db)
	if err != nil {
		r.logger.Error("failed to get unprocessed feeds", "error", err)
		return nil, nil, fmt.Errorf("failed to get unprocessed feeds: %w", err)
	}

	// Convert []url.URL to []*url.URL
	urlPtrs := make([]*url.URL, len(urls))
	for i, u := range urls {
		urlPtrs[i] = &u
	}

	// Create new cursor
	newCursor := &Cursor{
		LastCreatedAt: finalCreatedAt,
		LastID:        finalID,
	}

	r.logger.Info("got unprocessed feeds", "count", len(urlPtrs))

	return urlPtrs, newCursor, nil
}

// GetProcessingStats returns feed processing statistics.
func (r *feedRepository) GetProcessingStats(ctx context.Context) (*ProcessingStats, error) {
	r.logger.Info("getting processing statistics")

	// Use existing driver function
	totalFeeds, processedFeeds, err := driver.GetFeedStatistics(ctx, r.db)
	if err != nil {
		r.logger.Error("failed to get processing statistics", "error", err)
		return nil, fmt.Errorf("failed to get processing statistics: %w", err)
	}

	stats := &ProcessingStats{
		TotalFeeds:     totalFeeds,
		ProcessedFeeds: processedFeeds,
		RemainingFeeds: totalFeeds - processedFeeds,
	}

	r.logger.Info("got processing statistics", "total", stats.TotalFeeds, "processed", stats.ProcessedFeeds, "remaining", stats.RemainingFeeds)

	return stats, nil
}
