package feed_url_to_id_port

import (
	"context"
)

// FeedURLToIDPort defines the contract for converting feed URLs to feed IDs
type FeedURLToIDPort interface {
	// GetFeedIDByURL retrieves the feed ID for a given feed URL
	// Returns the feed ID as string, or error if not found
	GetFeedIDByURL(ctx context.Context, feedURL string) (string, error)
}