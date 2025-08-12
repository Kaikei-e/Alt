package domain

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// ImageFetchResult represents the result of fetching an image
type ImageFetchResult struct {
	URL         string
	ContentType string
	Data        []byte
	Size        int
	FetchedAt   time.Time
}

// ImageFetchOptions represents options for fetching an image
type ImageFetchOptions struct {
	MaxSize int           // Maximum size in bytes (default: 5MB)
	Timeout time.Duration // Request timeout (default: 30s)
}

// NewImageFetchOptions creates default ImageFetchOptions
func NewImageFetchOptions() *ImageFetchOptions {
	return &ImageFetchOptions{
		MaxSize: 5 * 1024 * 1024, // 5MB
		Timeout: 30 * time.Second, // 30 seconds
	}
}

// ValidateImageURL validates if the URL is suitable for image fetching
func ValidateImageURL(rawURL string) (*url.URL, error) {
	if strings.TrimSpace(rawURL) == "" {
		return nil, fmt.Errorf("URL cannot be empty")
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL format: %w", err)
	}

	// Only allow HTTPS
	if parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("only HTTPS URLs are allowed")
	}

	return parsedURL, nil
}

// IsAllowedImageDomain checks if the domain is in the whitelist for image fetching
func IsAllowedImageDomain(hostname string) bool {
	allowedDomains := []string{
		"9to5mac.com",
		"techcrunch.com",
		"arstechnica.com",
		"theverge.com",
		"engadget.com",
		"wired.com",
		"cdn.mos.cms.futurecdn.net",
		"images.unsplash.com",
		"img.youtube.com",
		"i.imgur.com",
		"pbs.twimg.com",
		"images.pexels.com",
		"cdn.pixabay.com",
	}

	hostname = strings.ToLower(hostname)

	for _, allowedDomain := range allowedDomains {
		if hostname == allowedDomain || strings.HasSuffix(hostname, "."+allowedDomain) {
			return true
		}
	}

	return false
}

// IsValidImagePath checks if the URL path is likely to contain an image
func IsValidImagePath(pathname string) bool {
	pathname = strings.ToLower(pathname)

	// Check for common image file extensions
	imageExtensions := []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".bmp", ".ico", ".tiff"}
	for _, ext := range imageExtensions {
		if strings.Contains(pathname, ext) {
			return true
		}
	}

	// Check for image-related path patterns
	imagePatterns := []string{
		"/photo-",
		"/image",
		"/img/",
		"/thumb",
		"/avatar",
		"/logo",
		"/wp-content/uploads", // WordPress images
	}

	for _, pattern := range imagePatterns {
		if strings.Contains(pathname, pattern) {
			return true
		}
	}

	return true // Default to allowing since some dynamic image URLs don't have obvious patterns
}

// IsValidImageContentType validates if the content type is an allowed image type
func IsValidImageContentType(contentType string) bool {
	if contentType == "" {
		return false
	}

	contentType = strings.ToLower(strings.TrimSpace(contentType))
	return strings.HasPrefix(contentType, "image/")
}