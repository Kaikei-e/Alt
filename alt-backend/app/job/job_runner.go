package job

import (
	"context"
	"time"

	"alt/driver/alt_db"
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
				// Log but don't retry - wait for next cycle
			} else {
				logger.Logger.Info("Feed collection completed", "feed count", len(feeds))

				// Uncomment when ready to write feeds to file
				// err = WriteFeedsToFile(feeds)
				// if err != nil {
				// 	logger.Logger.Error("Error writing feeds to file", "error", err)
				// }
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
