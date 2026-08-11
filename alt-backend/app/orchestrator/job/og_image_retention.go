package job

import (
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

// articleHeadPurger and imageCachePurger are the two halves of the retention
// sweep.
//
// They used to be one interface satisfied by *alt_db.AltDBRepository, because
// one struct happened to carry all three queries. Since ADR-000954 Wave 3 the
// deletes are two capability groups on alt-data-hub — article_heads (catalog
// §2.D) and image_proxy_cache (§2.E) — and splitting the interfaces keeps the
// job honest about which artifact each call removes.
type articleHeadPurger interface {
	CleanupExpiredArticleHeads(ctx context.Context, ttl time.Duration) (int64, error)
	// CleanupExpiredFeedOgImages purges the on-demand resolutions. It sits on
	// the same interface as article_heads because it is the same kind of
	// artifact under the same window: a scraped third-party image URL. A
	// resolver that acquired these without a sweep to remove them would be
	// worse than not resolving at all.
	CleanupExpiredFeedOgImages(ctx context.Context, ttl time.Duration) (int64, error)
}

type imageCachePurger interface {
	CleanupImageProxyCacheOlderThan(ctx context.Context, ttl time.Duration) (int64, error)
	CleanupExpiredImageProxyCache(ctx context.Context) (int64, error)
}

// OgImageRetentionJob returns a JobScheduler function that enforces the OG image
// copyright retention window: it purges article_heads and cached image bytes
// older than ogImageRetentionWindow, and evicts TTL-expired cache entries.
//
// Both dependencies are required. This job is the only thing that enforces a
// copyright retention window, so a nil one would leave scraped third-party
// artifacts on disk indefinitely while the job logged a successful sweep every
// six hours (CLAUDE.md rule 8).
func OgImageRetentionJob(heads articleHeadPurger, cache imageCachePurger) func(ctx context.Context) error {
	switch {
	case heads == nil:
		panic("og-image-retention: article head purger is nil — the article_heads retention window would never be enforced (see .claude/rules/di-wiring.md)")
	case cache == nil:
		panic("og-image-retention: image cache purger is nil — the cached-image retention window would never be enforced (see .claude/rules/di-wiring.md)")
	}
	return ogImageRetentionJobFn(heads, cache)
}

func ogImageRetentionJobFn(headPurger articleHeadPurger, cachePurger imageCachePurger) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		heads, err := headPurger.CleanupExpiredArticleHeads(ctx, ogImageRetentionWindow)
		if err != nil {
			return fmt.Errorf("purge article heads past retention: %w", err)
		}

		feedImages, err := headPurger.CleanupExpiredFeedOgImages(ctx, ogImageRetentionWindow)
		if err != nil {
			return fmt.Errorf("purge feed og images past retention: %w", err)
		}

		images, err := cachePurger.CleanupImageProxyCacheOlderThan(ctx, ogImageRetentionWindow)
		if err != nil {
			return fmt.Errorf("purge image cache past retention: %w", err)
		}

		expired, err := cachePurger.CleanupExpiredImageProxyCache(ctx)
		if err != nil {
			return fmt.Errorf("evict expired image cache: %w", err)
		}

		slog.InfoContext(ctx, "OG image retention completed",
			"article_heads_purged", heads,
			"feed_og_images_purged", feedImages,
			"image_cache_purged", images,
			"image_cache_expired", expired,
			"retention_window", ogImageRetentionWindow.String(),
		)
		return nil
	}
}
