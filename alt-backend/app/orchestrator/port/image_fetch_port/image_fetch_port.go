package image_fetch_port

import (
	"alt/domain"
	"context"
	"net/url"
)

// ImageFetchPort defines the interface for external image fetching operations
type ImageFetchPort interface {
	// FetchImage fetches an image from the given URL with options
	FetchImage(ctx context.Context, imageURL *url.URL, options *domain.ImageFetchOptions) (*domain.ImageFetchResult, error)
}
