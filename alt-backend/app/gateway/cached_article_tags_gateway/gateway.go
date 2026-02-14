package cached_article_tags_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/port/cached_article_tags_port"
	"context"
)

// Verify interface compliance at compile time.
var _ cached_article_tags_port.CachedArticleTagsPort = (*Gateway)(nil)

// Gateway implements CachedArticleTagsPort using AltDBRepository (DB-only, no generation).
type Gateway struct {
	repo *alt_db.AltDBRepository
}

// NewGateway creates a new cached article tags gateway.
func NewGateway(repo *alt_db.AltDBRepository) *Gateway {
	return &Gateway{repo: repo}
}

// FetchCachedArticleTags retrieves tags from the database without triggering on-the-fly generation.
func (g *Gateway) FetchCachedArticleTags(ctx context.Context, articleID string) ([]*domain.FeedTag, error) {
	return g.repo.FetchArticleTags(ctx, articleID)
}
