package image_proxy_gateway

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"testing"
)

func TestProcessingGateway_ProcessImage_JPEG(t *testing.T) {
	gw := NewProcessingGateway()

	// Create a test JPEG image (800x400)
	img := createTestImage(800, 400)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}

	result, err := gw.ProcessImage(context.Background(), buf.Bytes(), "image/jpeg", 600, 80)
	if err != nil {
		t.Fatalf("ProcessImage failed: %v", err)
	}

	if result.ContentType != "image/jpeg" {
		t.Errorf("expected content type image/jpeg, got %s", result.ContentType)
	}
	if result.Width > 600 {
		t.Errorf("expected width <= 600, got %d", result.Width)
	}
	if result.Width != 600 {
		t.Errorf("expected width 600 (downscaled from 800), got %d", result.Width)
	}
	// Aspect ratio: 800:400 = 2:1, so at 600px wide, height should be 300
	if result.Height != 300 {
		t.Errorf("expected height 300 (aspect ratio preserved), got %d", result.Height)
	}
	if result.SizeBytes != len(result.Data) {
		t.Errorf("SizeBytes mismatch: %d vs len %d", result.SizeBytes, len(result.Data))
	}
	if result.ETag == "" {
		t.Error("expected non-empty ETag")
	}
}

func TestProcessingGateway_ProcessImage_PNG(t *testing.T) {
	gw := NewProcessingGateway()

	img := createTestImage(400, 300)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	result, err := gw.ProcessImage(context.Background(), buf.Bytes(), "image/png", 600, 80)
	if err != nil {
		t.Fatalf("ProcessImage failed: %v", err)
	}

	// Image is 400px wide, smaller than maxWidth 600 - should not be upscaled
	if result.Width != 400 {
		t.Errorf("expected width 400 (no upscale), got %d", result.Width)
	}
	if result.Height != 300 {
		t.Errorf("expected height 300, got %d", result.Height)
	}
}

func TestProcessingGateway_ProcessImage_InvalidData(t *testing.T) {
	gw := NewProcessingGateway()

	_, err := gw.ProcessImage(context.Background(), []byte("not an image"), "image/jpeg", 600, 80)
	if err == nil {
		t.Fatal("expected error for invalid image data")
	}
}

func TestProcessingGateway_ProcessImage_WebP(t *testing.T) {
	gw := NewProcessingGateway()

	// Load a real WebP test file (lossy VP8 format, from golang.org/x/image testdata)
	webpData, err := os.ReadFile("testdata/test.webp")
	if err != nil {
		t.Fatalf("failed to read WebP test file: %v", err)
	}

	result, err := gw.ProcessImage(context.Background(), webpData, "image/webp", 600, 80)
	if err != nil {
		t.Fatalf("ProcessImage failed for WebP input: %v", err)
	}

	if result.ContentType != "image/jpeg" {
		t.Errorf("expected output content type image/jpeg, got %s", result.ContentType)
	}
	if result.Width == 0 || result.Height == 0 {
		t.Errorf("expected non-zero dimensions, got %dx%d", result.Width, result.Height)
	}
	if result.SizeBytes != len(result.Data) {
		t.Errorf("SizeBytes mismatch: %d vs len %d", result.SizeBytes, len(result.Data))
	}
	if result.ETag == "" {
		t.Error("expected non-empty ETag")
	}
}

func TestProcessingGateway_ProcessImage_EmptyData(t *testing.T) {
	gw := NewProcessingGateway()

	_, err := gw.ProcessImage(context.Background(), nil, "image/jpeg", 600, 80)
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestProcessingGateway_ProcessImage_DecodeErrorIncludesDiagnostics(t *testing.T) {
	gw := NewProcessingGateway()

	// HTML error page bytes — valid content-type check could pass upstream if
	// the server lies about the MIME, but image.Decode will reject it.
	htmlBytes := []byte("<!DOCTYPE html><html><body>404 Not Found</body></html>")

	_, err := gw.ProcessImage(context.Background(), htmlBytes, "text/html", 600, 80)
	if err == nil {
		t.Fatal("expected error for HTML input")
	}
	msg := err.Error()
	if !strings.Contains(msg, "text/html") {
		t.Errorf("expected error to contain upstream content-type, got: %s", msg)
	}
	// Expect magic-byte hex prefix to be surfaced for triage.
	if !strings.Contains(msg, "magic=") {
		t.Errorf("expected error to contain magic byte prefix, got: %s", msg)
	}
}

func TestProcessingGateway_ProcessImage_DecodeErrorNamesAVIF(t *testing.T) {
	gw := NewProcessingGateway()

	// Synthetic ISO-BMFF header with 'ftypavif' brand — enough for format
	// detection to classify as AVIF even though image.Decode will fail.
	avifHeader := []byte{
		0x00, 0x00, 0x00, 0x20, // box size
		'f', 't', 'y', 'p',
		'a', 'v', 'i', 'f', // major brand = avif
		0x00, 0x00, 0x00, 0x00, // minor version
		'a', 'v', 'i', 'f', // compatible brand
		'm', 'i', 'f', '1',
		'm', 'i', 'a', 'f',
		'M', 'A', '1', 'B',
	}

	_, err := gw.ProcessImage(context.Background(), avifHeader, "image/avif", 600, 80)
	if err == nil {
		t.Fatal("expected error for AVIF input (decoder not registered)")
	}
	msg := err.Error()
	if !strings.Contains(msg, "avif") && !strings.Contains(msg, "AVIF") {
		t.Errorf("expected error to identify AVIF format, got: %s", msg)
	}
}

func createTestImage(width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(x % 256),
				G: uint8(y % 256),
				B: uint8((x + y) % 256),
				A: 255,
			})
		}
	}
	return img
}
