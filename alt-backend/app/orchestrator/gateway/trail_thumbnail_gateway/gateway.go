// Package trail_thumbnail_gateway adapts the article_heads OG image lookup
// (the same table the og-image-backfill job scrapes into) to the trail episode
// thumbnail port.
//
// The lookup itself moved to alt-data-hub in ADR-000954 Wave 3
// (catalog §2.D / W3-D2), so what this wraps is a Connect-RPC client rather
// than a database driver. Nothing about the port changed, which is the point:
// the trail rendering code cannot tell the difference.
package trail_thumbnail_gateway

import (
	"context"
)

// ogImageLookup is the one capability this gateway needs. Declared here as an
// interface rather than taking the concrete data-hub gateway so the trail
// tests can substitute it without a Connect client.
type ogImageLookup interface {
	FetchOgImageURLsByArticleIDs(ctx context.Context, articleIDs []string) (map[string]string, error)
}

// Gateway implements trail_thumbnail_port.GetOgImageURLsPort.
type Gateway struct {
	db ogImageLookup
}

// NewGateway wires the gateway to the data-hub OG image capability.
func NewGateway(db ogImageLookup) *Gateway {
	if db == nil {
		panic("trail_thumbnail_gateway: og image lookup is required — a nil one would render every trail episode without a thumbnail and report success (see .claude/rules/di-wiring.md)")
	}
	return &Gateway{db: db}
}

// GetOgImageURLsByArticleIDs resolves OG image URLs for article ids.
func (g *Gateway) GetOgImageURLsByArticleIDs(ctx context.Context, articleIDs []string) (map[string]string, error) {
	return g.db.FetchOgImageURLsByArticleIDs(ctx, articleIDs)
}
