// Package trail_thumbnail_gateway adapts alt_db's article_heads OG image
// lookup (the same table the og-image-backfill job scrapes into) to the
// trail episode thumbnail port.
package trail_thumbnail_gateway

import (
	"context"

	"alt/shared/driver/alt_db"
)

type ogImageLookupDB interface {
	FetchOgImageURLsByArticleIDs(ctx context.Context, articleIDs []string) (map[string]string, error)
}

// Gateway implements trail_thumbnail_port.GetOgImageURLsPort.
type Gateway struct {
	db ogImageLookupDB
}

// NewGateway wires the gateway to the shared alt_db repository.
func NewGateway(db *alt_db.AltDBRepository) *Gateway {
	return newGateway(db)
}

func newGateway(db ogImageLookupDB) *Gateway {
	return &Gateway{db: db}
}

// GetOgImageURLsByArticleIDs resolves OG image URLs for article ids.
func (g *Gateway) GetOgImageURLsByArticleIDs(ctx context.Context, articleIDs []string) (map[string]string, error) {
	return g.db.FetchOgImageURLsByArticleIDs(ctx, articleIDs)
}
