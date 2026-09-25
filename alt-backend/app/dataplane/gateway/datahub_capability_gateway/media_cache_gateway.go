package datahub_capability_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	"alt/shared/driver/alt_db"
)

// ---------------------------------------------------------------------------
// §2.D OG image / article_heads
// ---------------------------------------------------------------------------

type ogImageDriver interface {
	SaveArticleHead(ctx context.Context, articleID, headHTML, ogImageURL string) error
	FetchArticleHeadByArticleID(ctx context.Context, articleID string) (*domain.ArticleHead, error)
	FetchOgImageURLsByArticleIDs(ctx context.Context, articleIDs []string) (map[string]string, error)
	FetchUnwarmedOgImageURLs(ctx context.Context, limit int) ([]string, error)
	CleanupExpiredArticleHeads(ctx context.Context, ttl time.Duration) (int64, error)
	FetchFeedOgImageTargets(ctx context.Context, feedIDs []string) ([]domain.FeedOgImageTarget, error)
	SaveFeedOgImage(ctx context.Context, feedID, ogImageURL string, refusal domain.OgImageRefusal, retryAfter time.Duration) error
	CleanupExpiredFeedOgImages(ctx context.Context, ttl time.Duration) (int64, error)
}

// OgImageGateway implements datahub_capability_port.OgImagePort.
type OgImageGateway struct {
	db ogImageDriver
}

func NewOgImageGateway(db *alt_db.AltDBRepository) *OgImageGateway {
	return &OgImageGateway{db: db}
}

// SaveArticleHead upserts the scraped head (catalog §2.B W3-B2). It lives on
// the OG image gateway because it writes the table the §2.D reads read.
func (g *OgImageGateway) SaveArticleHead(ctx context.Context, articleID, headHTML, ogImageURL string) error {
	if err := g.db.SaveArticleHead(ctx, articleID, headHTML, ogImageURL); err != nil {
		return fmt.Errorf("save article head %s: %w", articleID, err)
	}
	return nil
}

func (g *OgImageGateway) GetArticleHead(ctx context.Context, articleID string) (*domain.ArticleHead, error) {
	head, err := g.db.FetchArticleHeadByArticleID(ctx, articleID)
	if err != nil {
		return nil, fmt.Errorf("get article head %s: %w", articleID, err)
	}
	return head, nil
}

func (g *OgImageGateway) BatchGetOgImageURLs(ctx context.Context, articleIDs []string) (map[string]string, error) {
	urls, err := g.db.FetchOgImageURLsByArticleIDs(ctx, articleIDs)
	if err != nil {
		return nil, fmt.Errorf("batch get og image urls: %w", err)
	}
	return urls, nil
}

func (g *OgImageGateway) ListUnwarmedOgImageURLs(ctx context.Context, limit int) ([]string, error) {
	urls, err := g.db.FetchUnwarmedOgImageURLs(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list unwarmed og image urls: %w", err)
	}
	return urls, nil
}

func (g *OgImageGateway) PurgeExpiredArticleHeads(ctx context.Context, ttl time.Duration) (int64, error) {
	purged, err := g.db.CleanupExpiredArticleHeads(ctx, ttl)
	if err != nil {
		return 0, fmt.Errorf("purge expired article heads: %w", err)
	}
	return purged, nil
}

func (g *OgImageGateway) GetFeedOgImageTargets(ctx context.Context, feedIDs []string) ([]domain.FeedOgImageTarget, error) {
	targets, err := g.db.FetchFeedOgImageTargets(ctx, feedIDs)
	if err != nil {
		return nil, fmt.Errorf("get feed og image targets (%d ids): %w", len(feedIDs), err)
	}
	return targets, nil
}

func (g *OgImageGateway) SaveFeedOgImage(
	ctx context.Context,
	feedID, ogImageURL string,
	refusal domain.OgImageRefusal,
	retryAfter time.Duration,
) error {
	if err := g.db.SaveFeedOgImage(ctx, feedID, ogImageURL, refusal, retryAfter); err != nil {
		return fmt.Errorf("save feed og image %s: %w", feedID, err)
	}
	return nil
}

func (g *OgImageGateway) PurgeExpiredFeedOgImages(ctx context.Context, ttl time.Duration) (int64, error) {
	purged, err := g.db.CleanupExpiredFeedOgImages(ctx, ttl)
	if err != nil {
		return 0, fmt.Errorf("purge expired feed og images: %w", err)
	}
	return purged, nil
}

// ---------------------------------------------------------------------------
// §2.E Image proxy cache
// ---------------------------------------------------------------------------

type imageProxyCacheDriver interface {
	GetImageProxyCache(ctx context.Context, urlHash string) (*domain.ImageProxyCacheEntry, error)
	SaveImageProxyCache(ctx context.Context, entry *domain.ImageProxyCacheEntry) error
	CleanupExpiredImageProxyCache(ctx context.Context) (int64, error)
	CleanupImageProxyCacheOlderThan(ctx context.Context, ttl time.Duration) (int64, error)
}

// ImageProxyCacheGateway implements datahub_capability_port.ImageProxyCachePort.
type ImageProxyCacheGateway struct {
	db imageProxyCacheDriver
}

func NewImageProxyCacheGateway(db *alt_db.AltDBRepository) *ImageProxyCacheGateway {
	return &ImageProxyCacheGateway{db: db}
}

func (g *ImageProxyCacheGateway) Get(ctx context.Context, urlHash string) (*domain.ImageProxyCacheEntry, error) {
	entry, err := g.db.GetImageProxyCache(ctx, urlHash)
	if err != nil {
		return nil, fmt.Errorf("get image proxy cache %s: %w", urlHash, err)
	}
	return entry, nil
}

func (g *ImageProxyCacheGateway) Put(ctx context.Context, entry *domain.ImageProxyCacheEntry) error {
	if err := g.db.SaveImageProxyCache(ctx, entry); err != nil {
		return fmt.Errorf("put image proxy cache: %w", err)
	}
	return nil
}

func (g *ImageProxyCacheGateway) EvictExpired(ctx context.Context) (int64, error) {
	evicted, err := g.db.CleanupExpiredImageProxyCache(ctx)
	if err != nil {
		return 0, fmt.Errorf("evict expired image proxy cache: %w", err)
	}
	return evicted, nil
}

func (g *ImageProxyCacheGateway) PurgeOlderThan(ctx context.Context, ttl time.Duration) (int64, error) {
	purged, err := g.db.CleanupImageProxyCacheOlderThan(ctx, ttl)
	if err != nil {
		return 0, fmt.Errorf("purge image proxy cache older than %s: %w", ttl, err)
	}
	return purged, nil
}
