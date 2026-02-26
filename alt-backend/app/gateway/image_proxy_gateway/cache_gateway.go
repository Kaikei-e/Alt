package image_proxy_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"
)

// CacheGateway implements ImageProxyCachePort using the AltDB repository.
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
