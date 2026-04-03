package domain

import "context"

// TagCloudEntry represents a single tag with its article count.
type TagCloudEntry struct {
	TagName      string
	ArticleCount int32
}

// TagCloudClient fetches tag cloud data from alt-backend.
type TagCloudClient interface {
	FetchTagCloud(ctx context.Context, limit int) ([]TagCloudEntry, error)
}
