package image_fetch_gateway

import (
	"alt/domain"
	"alt/utils/errors"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageFetchGateway_FetchImage(t *testing.T) {
	tests := []struct {
		name           string
		imageURL       string
		options        *domain.ImageFetchOptions
		serverResponse func(w http.ResponseWriter, r *http.Request)
		want           *domain.ImageFetchResult
		wantErr        bool
		errCode        string
	}{
		{
			name:     "successful image fetch",
			imageURL: "https://example.com/image.jpg",
			options:  domain.NewImageFetchOptions(),
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				// Verify request headers
				assert.Contains(t, r.Header.Get("User-Agent"), "ALT-RSS-Reader")
				assert.Equal(t, "image/*", r.Header.Get("Accept"))
				
				w.Header().Set("Content-Type", "image/jpeg")
				w.Header().Set("Content-Length", "13")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("fake-jpg-data"))
			},
			want: &domain.ImageFetchResult{
				URL:         "https://example.com/image.jpg",
				ContentType: "image/jpeg",
				Data:        []byte("fake-jpg-data"),
				Size:        13, // Length of "fake-jpg-data"
			},
			wantErr: false,
		},
		{
			name:     "image too large",
			imageURL: "https://example.com/large-image.jpg",
			options: &domain.ImageFetchOptions{
				MaxSize: 10, // Very small limit for testing
				Timeout: 30 * time.Second,
			},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "image/jpeg")
				w.Header().Set("Content-Length", "20") // Larger than maxSize
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("this-is-larger-than-10-bytes"))
			},
			want:    nil,
			wantErr: true,
			errCode: "VALIDATION_ERROR",
		},
		{
			name:     "invalid content type",
			imageURL: "https://example.com/not-image.txt",
			options:  domain.NewImageFetchOptions(),
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("This is not an image"))
			},
			want:    nil,
			wantErr: true,
			errCode: "VALIDATION_ERROR",
		},
		{
			name:     "server returns 404",
			imageURL: "https://example.com/missing-image.jpg",
			options:  domain.NewImageFetchOptions(),
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("Image not found"))
			},
			want:    nil,
			wantErr: true,
			errCode: "EXTERNAL_API_ERROR",
		},
		{
			name:     "server returns 500",
			imageURL: "https://example.com/server-error.jpg",
			options:  domain.NewImageFetchOptions(),
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal Server Error"))
			},
			want:    nil,
			wantErr: true,
			errCode: "EXTERNAL_API_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			// Parse the test server URL and replace it with our test URL
			testURL, err := url.Parse(server.URL + strings.TrimPrefix(tt.imageURL, "https://example.com"))
			require.NoError(t, err)

			// Create gateway
			gateway := NewImageFetchGateway(&http.Client{Timeout: 10 * time.Second})

			// Execute
			got, err := gateway.FetchImage(context.Background(), testURL, tt.options)

			// Assertions
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if appErr, ok := err.(*errors.AppContextError); ok {
						assert.Equal(t, tt.errCode, appErr.Code)
					}
				}
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, testURL.String(), got.URL) // Use actual test server URL
				assert.Equal(t, tt.want.ContentType, got.ContentType)
				assert.Equal(t, tt.want.Data, got.Data)
				assert.Equal(t, tt.want.Size, got.Size)
				assert.NotZero(t, got.FetchedAt) // Should be set to current time
			}
		})
	}
}

func TestImageFetchGateway_FetchImage_Timeout(t *testing.T) {
	// Create a slow server that takes longer than timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // Simulate slow response
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("slow-image-data"))
	}))
	defer server.Close()

	testURL, err := url.Parse(server.URL + "/slow-image.jpg")
	require.NoError(t, err)

	// Create gateway with very short timeout
	gateway := NewImageFetchGateway(&http.Client{Timeout: 50 * time.Millisecond})

	options := &domain.ImageFetchOptions{
		MaxSize: 5 * 1024 * 1024,
		Timeout: 50 * time.Millisecond,
	}

	got, err := gateway.FetchImage(context.Background(), testURL, options)

	assert.Error(t, err)
	assert.Nil(t, got)
	if appErr, ok := err.(*errors.AppContextError); ok {
		assert.Equal(t, "TIMEOUT_ERROR", appErr.Code)
	}
}

func TestImageFetchGateway_FetchImage_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("image-data"))
	}))
	defer server.Close()

	testURL, err := url.Parse(server.URL + "/image.jpg")
	require.NoError(t, err)

	// Create context that cancels quickly
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	gateway := NewImageFetchGateway(&http.Client{Timeout: 10 * time.Second})
	got, err := gateway.FetchImage(ctx, testURL, domain.NewImageFetchOptions())

	assert.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "context canceled")
}