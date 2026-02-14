// Package internal_feed_port defines interfaces for internal feed API operations.
package internal_feed_port

import "context"

// FeedURL represents a feed with its ID and URL.
type FeedURL struct {
	FeedID string
	URL    string
}

// GetFeedIDPort returns the feed ID for a given feed URL.
type GetFeedIDPort interface {
	GetFeedID(ctx context.Context, feedURL string) (feedID string, err error)
}

// ListFeedURLsPort returns feed URLs with cursor pagination.
type ListFeedURLsPort interface {
	ListFeedURLs(ctx context.Context, cursor string, limit int) (feeds []FeedURL, nextCursor string, hasMore bool, err error)
}
