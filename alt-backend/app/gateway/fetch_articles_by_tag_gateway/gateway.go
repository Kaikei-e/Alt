package fetch_articles_by_tag_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"
	"time"
)

// FetchArticlesByTagGateway implements the port for fetching articles by tag.
type FetchArticlesByTagGateway struct {
	altDB *alt_db.AltDBRepository
}

// NewFetchArticlesByTagGateway creates a new gateway instance.
func NewFetchArticlesByTagGateway(altDB *alt_db.AltDBRepository) *FetchArticlesByTagGateway {
	return &FetchArticlesByTagGateway{
		altDB: altDB,
	}
}

// FetchArticlesByTag retrieves articles associated with a specific tag.
func (g *FetchArticlesByTagGateway) FetchArticlesByTag(ctx context.Context, tagID string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	return g.altDB.FetchArticlesByTag(ctx, tagID, cursor, limit)
}

// FetchArticlesByTagName retrieves articles associated with a specific tag name across all feeds.
func (g *FetchArticlesByTagGateway) FetchArticlesByTagName(ctx context.Context, tagName string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	return g.altDB.FetchArticlesByTagName(ctx, tagName, cursor, limit)
}
