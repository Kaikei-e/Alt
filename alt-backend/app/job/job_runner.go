package job

import (
	"context"
	"net/url"
	"time"

	"alt/utils/logger"
)

func HourlyJobRunner(ctx context.Context) {
	feedStaticURL := url.URL{
		Scheme: "https",
		Host:   "example.com",
		Path:   "/feed.xml",
	}

	go func() {
		for {
			feed, err := CollectSingleFeed(ctx, feedStaticURL)
			if err != nil {
				logger.Logger.Error("Error collecting feed", "error", err)
				retryCount, err := exponentialBackoffAndRetry(ctx, 5)
				if err != nil {
					logger.Logger.Error("Error collecting feed", "error", err)
					continue
				}
				logger.Logger.Info("Feed collected", "feed title", feed.Title, "retry count", retryCount)
			}
			logger.Logger.Info("Feed collected", "feed title", feed.Title)
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
