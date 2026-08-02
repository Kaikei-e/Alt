package datahub_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"

	"connectrpc.com/connect"
)

// ImageProxyCacheGateway is the image proxy's cache tier (catalog §2.E).
//
// Fetching, resizing and re-encoding the image stay on the calling side —
// they are an outbound HTTP call and a CPU-bound transform, neither of which
// ADR-000954 D4 moves. What crosses is the row.
type ImageProxyCacheGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewImageProxyCacheGateway(client datahubv1connect.DataHubServiceClient) *ImageProxyCacheGateway {
	if client == nil {
		panic("datahub_gateway: ImageProxyCacheGateway requires a DataHubService client (see .claude/rules/di-wiring.md)")
	}
	return &ImageProxyCacheGateway{client: client}
}

// GetCachedImage returns a live entry, or (nil, nil) on a miss.
//
// A miss and an expired entry are the same answer here, as they were in the
// driver: the WHERE clause filtered on expires_at, and the caller re-fetches
// either way.
func (g *ImageProxyCacheGateway) GetCachedImage(ctx context.Context, urlHash string) (*domain.ImageProxyCacheEntry, error) {
	resp, err := g.client.GetImageProxyCache(ctx, connect.NewRequest(&datahubv1.GetImageProxyCacheRequest{
		UrlHash: urlHash,
	}))
	if err != nil {
		return nil, fmt.Errorf("get image proxy cache %s: %w", urlHash, err)
	}
	return imageProxyCacheEntryFromProto(resp.Msg.GetEntry()), nil
}

// SaveCachedImage upserts an entry by url_hash.
func (g *ImageProxyCacheGateway) SaveCachedImage(ctx context.Context, entry *domain.ImageProxyCacheEntry) error {
	if entry == nil {
		return fmt.Errorf("save image proxy cache: entry is nil")
	}

	_, err := g.client.PutImageProxyCache(ctx, connect.NewRequest(&datahubv1.PutImageProxyCacheRequest{
		Entry: imageProxyCacheEntryToProto(entry),
	}))
	if err != nil {
		return fmt.Errorf("save image proxy cache %s: %w", entry.URLHash, err)
	}
	return nil
}

// CleanupExpiredImages deletes entries past their own TTL.
func (g *ImageProxyCacheGateway) CleanupExpiredImages(ctx context.Context) (int64, error) {
	resp, err := g.client.EvictExpiredImageProxyCache(ctx, connect.NewRequest(&datahubv1.EvictExpiredImageProxyCacheRequest{}))
	if err != nil {
		return 0, fmt.Errorf("evict expired image proxy cache: %w", err)
	}
	return resp.Msg.GetEvictedCount(), nil
}

// CleanupImageProxyCacheOlderThan enforces the copyright retention cap by
// first-acquisition time, independent of each entry's TTL.
func (g *ImageProxyCacheGateway) CleanupImageProxyCacheOlderThan(ctx context.Context, ttl time.Duration) (int64, error) {
	resp, err := g.client.PurgeImageProxyCacheOlderThan(ctx, connect.NewRequest(&datahubv1.PurgeImageProxyCacheOlderThanRequest{
		TtlSeconds: int64(ttl.Seconds()),
	}))
	if err != nil {
		return 0, fmt.Errorf("purge image proxy cache older than %s: %w", ttl, err)
	}
	return resp.Msg.GetPurgedCount(), nil
}

// CleanupExpiredImageProxyCache is the name the og-image-retention job's port
// uses for the TTL eviction. Same call as CleanupExpiredImages, which is the
// name the image proxy usecase's port uses; the two ports were written years
// apart against the same driver method.
func (g *ImageProxyCacheGateway) CleanupExpiredImageProxyCache(ctx context.Context) (int64, error) {
	return g.CleanupExpiredImages(ctx)
}
