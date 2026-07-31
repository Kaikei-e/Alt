package image_proxy_gateway

import (
	"alt/domain"
	"alt/shared/driver/alt_db"
	"context"
)

// CacheGateway implements ImageProxyCachePort using the AltDB repository.
//
// UNREFERENCED since ADR-000954 Wave 3 batch 1. The image proxy's cache tier
// moved to alt-data-hub (catalog §2.E), so both composition roots now wire
// datahub_gateway.ImageProxyCacheGateway here and NewCacheGateway has no
// caller. It is left in place only because Wave 3 batch 6 removes the alt_db
// drivers and everything that reaches them in one pass; deleting it here would
// spread that removal across seven commits with no way to check the total.
type CacheGateway struct {
	repo *alt_db.AltDBRepository
}

// NewCacheGateway creates a new CacheGateway.
func NewCacheGateway(repo *alt_db.AltDBRepository) *CacheGateway {
	return &CacheGateway{repo: repo}
}

func (g *CacheGateway) GetCachedImage(ctx context.Context, urlHash string) (*domain.ImageProxyCacheEntry, error) {
	return g.repo.GetImageProxyCache(ctx, urlHash)
}

func (g *CacheGateway) SaveCachedImage(ctx context.Context, entry *domain.ImageProxyCacheEntry) error {
	return g.repo.SaveImageProxyCache(ctx, entry)
}

func (g *CacheGateway) CleanupExpiredImages(ctx context.Context) (int64, error) {
	return g.repo.CleanupExpiredImageProxyCache(ctx)
}
