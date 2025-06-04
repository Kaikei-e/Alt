package job

import (
	"context"
	"time"

	"alt/driver/alt_db"
	"alt/driver/models"
	"alt/utils/logger"
)

func HourlyJobRunner(ctx context.Context, r *alt_db.AltDBRepository) {
	feedURLs, err := r.FetchRSSFeedURLs(ctx)
	if err != nil {
		logger.Logger.Error("Error fetching RSS feed URLs", "error", err)
		return
	}

	logger.Logger.Info("Found RSS feed URLs", "count", len(feedURLs))

	go func() {
		for {
			feeds, err := CollectMultipleFeeds(ctx, feedURLs)
			if err != nil {
				logger.Logger.Error("Error collecting feeds", "error", err)
			} else {
				logger.Logger.Info("Feed collection completed", "feed count", len(feeds))

				feedModels := make([]models.Feed, len(feeds))
				for i, feed := range feeds {
					feedModels[i] = models.Feed{
						Title:       feed.Title,
						Description: feed.Description,
						Link:        feed.Link,
						CreatedAt:   time.Now().UTC(),
						UpdatedAt:   time.Now().UTC(),
					}
				}

				r.RegisterMultipleFeeds(ctx, feedModels)
			}

			logger.Logger.Info("Sleeping for 1 hour until next feed collection cycle")
			time.Sleep(1 * time.Hour)
		}
	}()
}

// Remove the broken exponential backoff function since it doesn't actually retry the operation
// func exponentialBackoffAndRetry(ctx context.Context, maxRetries int) (int, error) {
// 	// This function was not working as intended - it just waited and returned
// 	// without actually retrying the feed collection operation
// }
