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
				logger.Logger.Error("Error collecting feed", "error", err)
				retryCount, err := exponentialBackoffAndRetry(ctx, 5)
				if err != nil {
					logger.Logger.Error("Error collecting feed", "error", err)
					continue
				}
				logger.Logger.Info("Feed collected", "feed length", len(feeds), "retry count", retryCount)
			}

			logger.Logger.Info("Feed collected", "feed length", len(feeds))
			// err = WriteFeedsToFile(feeds)
			// if err != nil {
			// 	logger.Logger.Error("Error writing feeds to file", "error", err)
			// }
			time.Sleep(1 * time.Hour)
		}
	}()
}

func exponentialBackoffAndRetry(ctx context.Context, maxRetries int) (int, error) {
	backoff := 5 * time.Second
	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			logger.Logger.Error("Context done", "error", ctx.Err())
			return 0, ctx.Err()
		default:
			logger.Logger.Info("Exponential backoff and retry", "retry", i, "backoff", backoff)
			// add retry count to the context
			ctx = context.WithValue(ctx, "retryCount", i)
			backoff *= 2
			time.Sleep(backoff)
		}
	}
	logger.Logger.Error("Exponential backoff and retry failed", "maxRetries", maxRetries)
	return 0, nil
}
