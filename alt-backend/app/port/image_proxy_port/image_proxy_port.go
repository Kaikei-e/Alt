package image_proxy_port

import (
	"alt/domain"
	"context"
)

// ImageProxyCachePort defines the interface for caching proxy images.
type ImageProxyCachePort interface {
	GetCachedImage(ctx context.Context, urlHash string) (*domain.ImageProxyCacheEntry, error)
	SaveCachedImage(ctx context.Context, entry *domain.ImageProxyCacheEntry) error
	CleanupExpiredImages(ctx context.Context) (int64, error)
}

// ImageProcessingPort defines the interface for image processing (resize + WebP).
type ImageProcessingPort interface {
	ProcessImage(ctx context.Context, data []byte, contentType string, maxWidth int, quality int) (*domain.ImageProxyResult, error)
}

// ImageProxySignerPort defines the interface for HMAC URL signing.
type ImageProxySignerPort interface {
	GenerateProxyURL(imageURL string) string
	VerifyAndDecode(signature, encodedURL string) (string, error)
}
