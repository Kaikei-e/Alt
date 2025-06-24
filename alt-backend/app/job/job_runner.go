package job

import (
	"context"
	"time"

	"alt/driver/alt_db"
	"alt/driver/models"
	"alt/utils/logger"
	"alt/utils/rate_limiter"
)

func HourlyJobRunner(ctx context.Context, r *alt_db.AltDBRepository) {
	feedURLs, err := r.FetchRSSFeedURLs(ctx)
	if err != nil {
		logger.Logger.Error("Error fetching RSS feed URLs", "error", err)
		return
	}

	logger.Logger.Info("Found RSS feed URLs", "count", len(feedURLs))

	// Create rate limiter with 5-second minimum interval for external API calls
	rateLimiter := rate_limiter.NewHostRateLimiter(5 * time.Second)

	go func() {
		for {
			feedItems, err := CollectMultipleFeeds(ctx, feedURLs, rateLimiter)
			if err != nil {
				logger.Logger.Error("Error collecting feeds", "error", err)
			} else {
				logger.Logger.Info("Feed collection completed", "feed count", len(feedItems))

				feedModels := make([]models.Feed, len(feedItems))
				for i, feedItem := range feedItems {
					feedModel := models.Feed{
						Title:       feedItem.Title,
						Description: feedItem.Description,
						Link:        feedItem.Link,
						CreatedAt:   time.Now().UTC(),
						UpdatedAt:   time.Now().UTC(),
					}
					feedModels[i] = feedModel
				}

				r.RegisterMultipleFeeds(ctx, feedModels)
			}

			logger.Logger.Info("Sleeping for 1 hour until next feed collection cycle")
			time.Sleep(1 * time.Hour)
		}
	}()
}
