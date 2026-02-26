package image_proxy_gateway

import (
	"alt/domain"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"time"

	"golang.org/x/image/draw"
)

// ProcessingGateway implements ImageProcessingPort for resizing and JPEG compression.
// Uses pure Go (no CGo) for compatibility with CGO_ENABLED=0 builds.
type ProcessingGateway struct{}

// NewProcessingGateway creates a new ProcessingGateway.
func NewProcessingGateway() *ProcessingGateway {
	return &ProcessingGateway{}
}

// ProcessImage decodes, resizes (maintaining aspect ratio), and re-encodes as optimized JPEG.
func (g *ProcessingGateway) ProcessImage(ctx context.Context, data []byte, contentType string, maxWidth int, quality int) (*domain.ImageProxyResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty image data")
	}

	// Decode the image
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	bounds := img.Bounds()
	origWidth := bounds.Dx()
	origHeight := bounds.Dy()

	// Resize only if wider than maxWidth (never upscale)
	resized := img
	newWidth := origWidth
	newHeight := origHeight
	if origWidth > maxWidth {
		newWidth = maxWidth
		newHeight = origHeight * maxWidth / origWidth
		dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
		resized = dst
	}

	// Encode as JPEG (pure Go, no CGo dependency)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("encode JPEG: %w", err)
	}

	encoded := buf.Bytes()

	// Check size limit
	if len(encoded) > domain.ImageProxyMaxSize {
		return nil, fmt.Errorf("processed image exceeds size limit: %d > %d", len(encoded), domain.ImageProxyMaxSize)
	}

	// Generate ETag from content hash
	hash := sha256.Sum256(encoded)
	etag := hex.EncodeToString(hash[:16])

	return &domain.ImageProxyResult{
		Data:        encoded,
		ContentType: "image/jpeg",
		Width:       newWidth,
		Height:      newHeight,
		SizeBytes:   len(encoded),
		ETag:        etag,
		ExpiresAt:   time.Now().Add(domain.ImageProxyCacheTTL),
	}, nil
}
