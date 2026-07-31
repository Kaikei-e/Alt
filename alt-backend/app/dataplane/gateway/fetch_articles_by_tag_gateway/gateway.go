// Package fetch_articles_by_tag_gateway serves the Tag Trail from alt_db.
//
// It sits under dataplane/ since ADR-000954 Wave 3 batch 6. Until then it was
// under shared/ and both cmd/backend and cmd/datahub built it — which was the
// problem: the Wave 2 FetchArticlesByTag procedure took a tag name with no
// cursor, so alt-backend's own paged Trail could not use it and kept reading
// the database directly. Batch 6 gave the Trail its own pair of procedures
// (ListArticlesByTagID / ListArticlesByTagName) and left this implementation
// as what serves them, in the one binary that owns the pool.
package fetch_articles_by_tag_gateway

import (
	"alt/domain"
	"alt/shared/driver/alt_db"
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
