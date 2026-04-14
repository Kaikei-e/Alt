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
	_ "golang.org/x/image/webp" // Register WebP decoder for image.Decode()
)

// detectFormatFromMagic identifies common image container formats from their
// leading bytes. Used for error diagnostics when image.Decode fails — tells
// operators whether the upstream served AVIF/HEIC/JXL (decoder not registered)
// vs. an HTML error page the Content-Type check let through.
func detectFormatFromMagic(data []byte) string {
	if len(data) < 4 {
		return "unknown"
	}
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return "jpeg"
	case bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G'}):
		return "png"
	case bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")):
		return "gif"
	case len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "webp"
	case bytes.HasPrefix(data, []byte("<!DOC")) || bytes.HasPrefix(data, []byte("<html")) || bytes.HasPrefix(data, []byte("<HTML")):
		return "html"
	case bytes.HasPrefix(data, []byte{0xFF, 0x0A}) || (len(data) >= 12 && bytes.Equal(data[0:12], []byte{0x00, 0x00, 0x00, 0x0C, 'J', 'X', 'L', ' ', 0x0D, 0x0A, 0x87, 0x0A})):
		return "jxl"
	}
	// ISO-BMFF family: size(4) + "ftyp" + major_brand(4)
	if len(data) >= 12 && bytes.Equal(data[4:8], []byte("ftyp")) {
		brand := string(data[8:12])
		switch brand {
		case "avif", "avis":
			return "avif"
		case "heic", "heix", "heim", "heis", "hevc", "hevx", "mif1", "msf1":
			return "heic"
		default:
			return "iso-bmff:" + brand
		}
	}
	return "unknown"
}

// magicPrefix returns a short hex string of the first n bytes for log triage.
func magicPrefix(data []byte, n int) string {
	if len(data) < n {
		n = len(data)
	}
	return hex.EncodeToString(data[:n])
}

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
		return nil, fmt.Errorf(
			"decode image (upstream_content_type=%q detected_format=%s magic=%s size=%d): %w",
			contentType,
			detectFormatFromMagic(data),
			magicPrefix(data, 16),
			len(data),
			err,
		)
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
