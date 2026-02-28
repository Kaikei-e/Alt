package job

import (
	"context"
	"time"

	"alt/driver/alt_db"
	"alt/driver/models"
	"alt/utils"
	"alt/utils/logger"
	"alt/utils/rate_limiter"
)

// CollectFeedsJob returns a function suitable for the JobScheduler that
// collects feeds from all registered RSS URLs and upserts them into the DB.
func CollectFeedsJob(r *alt_db.AltDBRepository) func(ctx context.Context) error {
	// Create rate limiter with 5-second minimum interval for external API calls
	rateLimiter := rate_limiter.NewHostRateLimiter(5 * time.Second)

	return func(ctx context.Context) error {
		feedLinks, err := r.FetchRSSFeedURLs(ctx)
		if err != nil {
			return err
		}

		logger.Logger.InfoContext(ctx, "Found RSS feed URLs", "count", len(feedLinks))

		feedItems, err := CollectMultipleFeeds(ctx, feedLinks, rateLimiter, r)
		if err != nil {
			return err
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
				Link:        normalizedLink,
				PubDate:     pubDate,
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
				FeedLinkID:  feedItem.FeedLinkID,
			}
		}

		if err := r.RegisterMultipleFeeds(ctx, feedModels); err != nil {
			return err
		}

		return nil
	}
}

// HourlyJobRunner is kept for backward compatibility but delegates to CollectFeedsJob.
// Deprecated: Use CollectFeedsJob with JobScheduler instead.
func HourlyJobRunner(ctx context.Context, r *alt_db.AltDBRepository) {
	if err := CollectFeedsJob(r)(ctx); err != nil {
		logger.Logger.ErrorContext(ctx, "Error in feed collection", "error", err)
	}
}
