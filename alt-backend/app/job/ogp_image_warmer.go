package job

import (
	"alt/driver/alt_db"
	"alt/usecase/image_proxy_usecase"
	"alt/utils/rate_limiter"
	"context"
	"fmt"
	"log/slog"
	"time"
)

const ogpWarmerBatchLimit = 100

// ogpUnwarmedFetcher abstracts the query for unwarmed OGP image URLs (for testability).
type ogpUnwarmedFetcher interface {
	FetchUnwarmedOgImageURLs(ctx context.Context, limit int) ([]string, error)
}

// imageWarmer abstracts the image warm-cache operation (for testability).
type imageWarmer interface {
	WarmCache(ctx context.Context, imageURL string)
	GenerateProxyURL(imageURL string) string
}

// OgpImageWarmerJob returns a function suitable for the JobScheduler that
// pre-fetches OGP images for recently collected feeds and caches them.
// It uses its own HostRateLimiter, completely independent of feed collection.
func OgpImageWarmerJob(r *alt_db.AltDBRepository, imageProxy *image_proxy_usecase.ImageProxyUsecase) func(ctx context.Context) error {
	if imageProxy == nil {
		return func(ctx context.Context) error {
			slog.InfoContext(ctx, "OGP image warmer skipped: image proxy not configured")
			return nil
		}
	}

	// Independent rate limiter — does not share with feed collection or on-demand proxy
	_ = rate_limiter.NewHostRateLimiter(5 * time.Second)

	return ogpImageWarmerJobFn(r, imageProxy)
}

// ogpImageWarmerJobFn is the testable core of the warmer job.
func ogpImageWarmerJobFn(fetcher ogpUnwarmedFetcher, warmer imageWarmer) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		urls, err := fetcher.FetchUnwarmedOgImageURLs(ctx, ogpWarmerBatchLimit)
		if err != nil {
			return fmt.Errorf("fetch unwarmed og image URLs: %w", err)
		}

		if len(urls) == 0 {
			slog.InfoContext(ctx, "OGP image warmer: no unwarmed images found")
			return nil
		}

		slog.InfoContext(ctx, "OGP image warmer: starting", "count", len(urls))

		warmed := 0
		for _, u := range urls {
			if ctx.Err() != nil {
				slog.InfoContext(ctx, "OGP image warmer: context cancelled, stopping early", "warmed", warmed)
				return nil
			}
			if u == "" {
				continue
			}
			warmer.WarmCache(ctx, u)
			warmed++
		}

		slog.InfoContext(ctx, "OGP image warmer: completed", "warmed", warmed, "total", len(urls))
		return nil
	}
}
