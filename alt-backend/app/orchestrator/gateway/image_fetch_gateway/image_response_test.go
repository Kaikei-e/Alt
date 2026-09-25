package image_fetch_gateway

import (
	"errors"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCheckContentEncoding(t *testing.T) {
	tests := []struct {
		name     string
		encoding string
		wantErr  bool
	}{
		{"empty", "", false},
		{"identity", "identity", false},
		{"gzip", "gzip", true},
		{"brotli", "br", true},
		{"zstd", "zstd", true},
		{"deflate", "deflate", true},
		{"multiple", "gzip, br", true},
		{"uppercase", "GZIP", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkContentEncoding(tt.encoding)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateImageHeaders(t *testing.T) {
	maxSize := 1024 * 1024 // 1MB

	t.Run("valid image headers with content length", func(t *testing.T) {
		len, err := validateImageHeaders("image/jpeg", "50000", maxSize)
		require.NoError(t, err)
		require.Equal(t, int64(50000), len)
	})

	t.Run("valid image headers without content length", func(t *testing.T) {
		len, err := validateImageHeaders("image/png", "", maxSize)
		require.NoError(t, err)
		require.Equal(t, int64(0), len)
	})

	t.Run("invalid content type", func(t *testing.T) {
		_, err := validateImageHeaders("text/html", "100", maxSize)
		require.Error(t, err)
		require.True(t, errors.Is(err, errInvalidContentType))
	})

	t.Run("content length exceeds max size", func(t *testing.T) {
		_, err := validateImageHeaders("image/webp", strconv.Itoa(maxSize+1), maxSize)
		require.Error(t, err)
		require.True(t, errors.Is(err, errImageTooLarge))
	})

	t.Run("content length exceeds int32 max", func(t *testing.T) {
		tooBig := strconv.FormatInt(int64(math.MaxInt32)+1, 10)
		_, err := validateImageHeaders("image/gif", tooBig, math.MaxInt32)
		require.Error(t, err)
		require.True(t, errors.Is(err, errImageTooLarge))
	})

	t.Run("invalid content length format ignored", func(t *testing.T) {
		len, err := validateImageHeaders("image/jpeg", "not-a-number", maxSize)
		require.NoError(t, err)
		require.Equal(t, int64(0), len)
	})
}

func TestBuildImageFetchResult(t *testing.T) {
	data := []byte{0x89, 0x50, 0x4E, 0x47} // PNG magic bytes
	now := time.Now()
	res := buildImageFetchResult("https://example.com/test.png", "image/png", data, now)

	require.NotNil(t, res)
	require.Equal(t, "https://example.com/test.png", res.URL)
	require.Equal(t, "image/png", res.ContentType)
	require.Equal(t, data, res.Data)
	require.Equal(t, 4, res.Size)
	require.Equal(t, now, res.FetchedAt)
}
