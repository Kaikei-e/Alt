package domain

import "context"

// TagCloudEntry represents a single tag with its article count.
type TagCloudEntry struct {
	TagName      string
	ArticleCount int32
}

// TagCloudClient fetches the tag cloud over one user's articles.
type TagCloudClient interface {
	FetchTagCloud(ctx context.Context, userID string, limit int) ([]TagCloudEntry, error)
}
