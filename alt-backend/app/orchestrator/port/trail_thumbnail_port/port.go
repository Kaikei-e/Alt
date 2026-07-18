// Package trail_thumbnail_port defines the lookup used to resolve an
// episode's representative OG image (D29).
package trail_thumbnail_port

import "context"

// GetOgImageURLsPort resolves OG image URLs for article ids. A missing entry
// in the returned map is not an error — the caller degrades to a text-only
// card (D29's transient-fallback rule).
type GetOgImageURLsPort interface {
	GetOgImageURLsByArticleIDs(ctx context.Context, articleIDs []string) (map[string]string, error)
}
