package job

import (
	"alt/driver/alt_db"
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ogImageRetentionWindow bounds how long OG image artifacts (scraped <head>
// metadata and cached image bytes) are retained, for copyright compliance.
// Artifacts older than this are purged; reloading the feed/article re-acquires
// them on demand.
const ogImageRetentionWindow = 7 * 24 * time.Hour

// ogImageRetentionPurger abstracts the retention deletes (for testability).
type ogImageRetentionPurger interface {
	CleanupExpiredArticleHeads(ctx context.Context, ttl time.Duration) (int64, error)
	CleanupImageProxyCacheOlderThan(ctx context.Context, ttl time.Duration) (int64, error)
	CleanupExpiredImageProxyCache(ctx context.Context) (int64, error)
}

// OgImageRetentionJob returns a JobScheduler function that enforces the OG image
// copyright retention window: it purges article_heads and cached image bytes
// older than ogImageRetentionWindow, and evicts TTL-expired cache entries.
func OgImageRetentionJob(r *alt_db.AltDBRepository) func(ctx context.Context) error {
	return ogImageRetentionJobFn(r)
}

func ogImageRetentionJobFn(p ogImageRetentionPurger) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		heads, err := p.CleanupExpiredArticleHeads(ctx, ogImageRetentionWindow)
		if err != nil {
			return fmt.Errorf("purge article heads past retention: %w", err)
		}

		images, err := p.CleanupImageProxyCacheOlderThan(ctx, ogImageRetentionWindow)
		if err != nil {
			return fmt.Errorf("purge image cache past retention: %w", err)
		}

		expired, err := p.CleanupExpiredImageProxyCache(ctx)
		if err != nil {
			return fmt.Errorf("evict expired image cache: %w", err)
		}

		slog.InfoContext(ctx, "OG image retention completed",
			"article_heads_purged", heads,
			"image_cache_purged", images,
			"image_cache_expired", expired,
			"retention_window", ogImageRetentionWindow.String(),
		)
		return nil
	}
}
