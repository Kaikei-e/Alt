package domain

import "time"

// ImageProxyResult represents a processed image ready for serving.
type ImageProxyResult struct {
	Data        []byte
	ContentType string
	Width       int
	Height      int
	SizeBytes   int
	ETag        string
	ExpiresAt   time.Time
}

// ImageProxyCacheEntry represents a cached image in the database.
type ImageProxyCacheEntry struct {
	URLHash     string
	OriginalURL string
	Data        []byte
	ContentType string
	Width       int
	Height      int
	SizeBytes   int
	ETag        string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

const (
	// ImageProxyCacheTTL is the default TTL for cached images.
	ImageProxyCacheTTL = 12 * time.Hour

	// ImageProxyMaxWidth is the maximum width for resized images.
	ImageProxyMaxWidth = 600

	// ImageProxyWebPQuality is the WebP encoding quality (0-100).
	ImageProxyWebPQuality = 80

	// ImageProxyMaxSize is the maximum size of compressed image data (1MB).
	ImageProxyMaxSize = 1 * 1024 * 1024
)
