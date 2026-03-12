package feed_page_cache_gateway

import (
	"alt/driver/alt_db"
	"alt/port/feed_page_cache_port"
	"alt/utils/cache"
	"alt/utils/sanitize"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type feedPageDB interface {
	FetchFeedsByFeedLinkID(ctx context.Context, feedLinkID uuid.UUID) ([]*alt_db.FeedPageRow, error)
}

type Gateway struct {
	db    feedPageDB
	cache *cache.SharedCache[uuid.UUID, []*feed_page_cache_port.FeedPageEntry]
}

func NewGateway(db *alt_db.AltDBRepository) *Gateway {
	g := &Gateway{db: db}
	g.cache = cache.NewSharedCache(2*time.Minute, time.Minute, g.loadFeedPage)
	return g
}

func newGateway(db feedPageDB) *Gateway {
	g := &Gateway{db: db}
	g.cache = cache.NewSharedCache(2*time.Minute, time.Minute, g.loadFeedPage)
	return g
}

func (g *Gateway) GetFeedPage(ctx context.Context, feedLinkID uuid.UUID) ([]*feed_page_cache_port.FeedPageEntry, error) {
	return g.cache.Get(ctx, feedLinkID)
}

func (g *Gateway) InvalidateFeedPage(ctx context.Context, feedLinkID uuid.UUID) error {
	g.cache.Invalidate(feedLinkID)
	return nil
}

func (g *Gateway) loadFeedPage(ctx context.Context, feedLinkID uuid.UUID) ([]*feed_page_cache_port.FeedPageEntry, error) {
	rows, err := g.db.FetchFeedsByFeedLinkID(ctx, feedLinkID)
	if err != nil {
		return nil, fmt.Errorf("fetch feeds by feed_link_id: %w", err)
	}

	result := make([]*feed_page_cache_port.FeedPageEntry, 0, len(rows))
	for _, row := range rows {
		result = append(result, &feed_page_cache_port.FeedPageEntry{
			FeedID:               row.FeedID,
			Title:                row.Title,
			Description:          row.Description,
			Link:                 row.Link,
			PubDate:              row.PubDate,
			CreatedAt:            row.CreatedAt,
			UpdatedAt:            row.UpdatedAt,
			ArticleID:            row.ArticleID,
			OgImageURL:           row.OgImageURL,
			SanitizedDescription: sanitize.SanitizeDescription(row.Description),
			FeedIDStr:            row.FeedID.String(),
			PublishedStr:         row.CreatedAt.Format(time.RFC3339),
		})
	}
	return result, nil
}
