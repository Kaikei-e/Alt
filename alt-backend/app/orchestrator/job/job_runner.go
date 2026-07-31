package job

import (
	"context"
	"fmt"
	"time"

	"alt/domain"

	"alt/orchestrator/driver/models"
	"alt/orchestrator/port/feed_link_availability_port"
	"alt/utils"
	"alt/utils/logger"
	"alt/utils/rate_limiter"
)

// FeedCollectorStore is the slice of alt-data-hub the hourly collector needs:
// the poll work list and the upsert of what polling produced.
//
// Declared here rather than taking a repository handle because the job stopped
// owning a database with ADR-000954 Wave 3 batch 3. Naming the three calls it
// makes means a later edit cannot reach a fourth one it has no capability for.
type FeedCollectorStore interface {
	feed_link_availability_port.FeedLinkAvailabilityPort

	// FetchRSSFeedURLs returns the links worth polling — active, or never
	// assessed. Auto-disabled links are absent, which is the whole mechanism
	// by which the collector stops hammering dead feeds.
	FetchRSSFeedURLs(ctx context.Context) ([]domain.FeedLink, error)
	// RegisterMultipleFeeds upserts one poll's items in a single transaction.
	RegisterMultipleFeeds(ctx context.Context, feeds []models.Feed) ([]string, error)
}

// CollectFeedsJob returns a function suitable for the JobScheduler that
// collects feeds from all registered RSS URLs and upserts them into the DB.
//
// The limiter is a parameter rather than a local because this job polls the
// same publishers cmd/backend polls when a user registers a feed
// (ADR-000954 review, weakness 5). A limiter built here would be reachable
// from no composition root, so no shared arbiter could ever be attached to
// it — the job would keep its own 5-second promise while the process next
// door kept a second one, and the publisher would see both.
func CollectFeedsJob(r FeedCollectorStore, rateLimiter *rate_limiter.HostRateLimiter) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		feedLinks, err := r.FetchRSSFeedURLs(ctx)
		if err != nil {
			return fmt.Errorf("fetch rss feed urls: %w", err)
		}

		logger.Logger.InfoContext(ctx, "Found RSS feed URLs", "count", len(feedLinks))

		feedItems, err := CollectMultipleFeeds(ctx, feedLinks, rateLimiter, r)
		if err != nil {
			return fmt.Errorf("collect multiple feeds: %w", err)
		}

		logger.Logger.InfoContext(ctx, "Feed collection completed", "feed_count", len(feedItems))

		feedModels := make([]models.Feed, len(feedItems))
		for i, feedItem := range feedItems {
			pubDate := feedItem.PublishedParsed
			if pubDate.IsZero() {
				pubDate = time.Now().UTC()
			}
			normalizedLink, err := utils.NormalizeURL(feedItem.Link)
			if err != nil {
				logger.Logger.WarnContext(ctx, "Failed to normalize feed link, using original",
					"link", feedItem.Link,
					"error", err)
				normalizedLink = feedItem.Link
			}
			feedModels[i] = models.Feed{
				Title:       feedItem.Title,
				Description: feedItem.Description,
				WebsiteURL:  normalizedLink,
				PubDate:     pubDate,
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
				FeedLinkID:  feedItem.FeedLinkID,
				OgImageURL:  stringPtrOrNil(feedItem.OgImageURL),
			}
		}

		if _, err := r.RegisterMultipleFeeds(ctx, feedModels); err != nil {
			return fmt.Errorf("register multiple feeds: %w", err)
		}

		return nil
	}
}

// stringPtrOrNil returns a pointer to s if non-empty, or nil otherwise.
func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// HourlyJobRunner is kept for backward compatibility but delegates to CollectFeedsJob.
// Deprecated: Use CollectFeedsJob with JobScheduler instead.
func HourlyJobRunner(ctx context.Context, r FeedCollectorStore, rateLimiter *rate_limiter.HostRateLimiter) {
	if err := CollectFeedsJob(r, rateLimiter)(ctx); err != nil {
		logger.Logger.ErrorContext(ctx, "Error in feed collection", "error", err)
	}
}
