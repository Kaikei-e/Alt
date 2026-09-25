package image_fetch_gateway

import (
	"alt/domain"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

var (
	errInvalidContentType = errors.New("response is not an image")
	errImageTooLarge      = errors.New("image too large")
)

// checkContentEncoding validates that Content-Encoding is uncompressed or identity.
func checkContentEncoding(contentEncoding string) error {
	enc := strings.ToLower(strings.TrimSpace(contentEncoding))
	if enc == "" || enc == "identity" {
		return nil
	}
	return fmt.Errorf("unhandled response Content-Encoding %q: body is still compressed and cannot be decoded safely", enc)
}

// validateImageHeaders checks that contentType is an image and Content-Length does not exceed maxSize.
func validateImageHeaders(contentType, contentLengthHeader string, maxSize int) (int64, error) {
	if !domain.IsValidImageContentType(contentType) {
		return 0, errInvalidContentType
	}

	if contentLengthHeader != "" {
		if contentLength, err := strconv.ParseInt(contentLengthHeader, 10, 64); err == nil {
			// Safe comparison with bounds checking to prevent integer overflow
			maxSizeInt64 := int64(maxSize)

			// Check if content length exceeds int32 bounds or the configured max size
			if contentLength > math.MaxInt32 || contentLength > maxSizeInt64 {
				return contentLength, errImageTooLarge
			}
			return contentLength, nil
		}
	}

	return 0, nil
}

// buildImageFetchResult constructs an ImageFetchResult from fetched components.
func buildImageFetchResult(imageURL, contentType string, data []byte, fetchedAt time.Time) *domain.ImageFetchResult {
	return &domain.ImageFetchResult{
		URL:         imageURL,
		ContentType: contentType,
		Data:        data,
		Size:        len(data),
		FetchedAt:   fetchedAt,
	}
}
