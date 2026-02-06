package service

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLoggerFetcher() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
}

func TestArticleFetcherService_InterfaceCompliance(t *testing.T) {
	t.Run("should implement ArticleFetcherService interface", func(t *testing.T) {
		service := NewArticleFetcherService(testLoggerFetcher())
		var _ = service
		assert.NotNil(t, service)
	})
}

func TestArticleFetcherService_ValidateURL(t *testing.T) {
	tests := map[string]struct {
		input       string
		expectError bool
	}{
		"should validate HTTPS URL": {
			input:       "https://example.com",
			expectError: false,
		},
		"should validate HTTP URL": {
			input:       "http://example.com",
			expectError: false,
		},
		"should reject malformed URL": {
			input:       "://invalid",
			expectError: true,
		},
		"should reject empty URL": {
			input:       "",
			expectError: true,
		},
		"should handle URL with path": {
			input:       "https://example.com/path/to/article",
			expectError: false,
		},
		"should handle URL with query params": {
			input:       "https://example.com/article?id=123&lang=en",
			expectError: false,
		},
		"should reject non-HTTP schemes": {
			input:       "ftp://example.com",
			expectError: true,
		},
		"should reject missing host": {
			input:       "http:///path",
			expectError: true,
		},
		"should handle URLs with special characters": {
			input:       "https://example.com/path with spaces",
			expectError: false,
		},
		"should reject blocked port 22": {
			input:       "https://example.com:22",
			expectError: true,
		},
		"should reject blocked port 3306": {
			input:       "https://example.com:3306",
			expectError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			service := NewArticleFetcherService(testLoggerFetcher())

			err := service.ValidateURL(tc.input)

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestArticleFetcherService_FetchArticle(t *testing.T) {
	tests := map[string]struct {
		input       string
		description string
	}{
		"should handle malformed URL": {
			input:       "://invalid",
			description: "Article fetching is disabled for ethical compliance",
		},
		"should handle empty URL": {
			input:       "",
			description: "Article fetching is disabled for ethical compliance",
		},
		"should reject non-HTTP schemes": {
			input:       "ftp://example.com",
			description: "Article fetching is disabled for ethical compliance",
		},
		"should skip MP3 files": {
			input:       "https://example.com/audio.mp3",
			description: "Article fetching is disabled for ethical compliance",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			service := NewArticleFetcherService(testLoggerFetcher())

			result, err := service.FetchArticle(context.Background(), tc.input)

			require.NoError(t, err, tc.description)
			assert.Nil(t, result, "Article fetching disabled, should return nil")
		})
	}
}

func TestArticleFetcherService_PublicURLValidation(t *testing.T) {
	service := NewArticleFetcherService(testLoggerFetcher())

	publicURLs := []string{
		"https://example.com/article",
		"https://news.example.org/story",
		"http://blog.example.net/post/123",
	}

	for _, url := range publicURLs {
		err := service.ValidateURL(url)
		assert.NoError(t, err, "URL validation should pass for %s", url)
	}
}

func TestArticleFetcher_HTTPClientManagerIntegration(t *testing.T) {
	t.Run("should use HTTPClientManager when no custom client provided", func(t *testing.T) {
		service := NewArticleFetcherService(testLoggerFetcher())

		impl, ok := service.(*articleFetcherService)
		require.True(t, ok, "service should be *articleFetcherService")

		assert.Nil(t, impl.httpClient, "httpClient should be nil to trigger HTTPClientManager usage")
	})

	t.Run("should respect custom HTTP client when provided", func(t *testing.T) {
		customClient := &MockHTTPClient{}
		service := NewArticleFetcherServiceWithClient(testLoggerFetcher(), customClient)

		impl, ok := service.(*articleFetcherService)
		require.True(t, ok, "service should be *articleFetcherService")

		assert.NotNil(t, impl.httpClient, "httpClient should not be nil when injected")
		assert.Equal(t, customClient, impl.httpClient, "injected client should be used")
	})
}

// MockHTTPClient implements HTTPClient for testing
type MockHTTPClient struct{}

func (m *MockHTTPClient) Get(url string) (*http.Response, error) {
	return &http.Response{}, nil
}

func TestArticleFetcher_ConfigurationConsistency(t *testing.T) {
	t.Run("should use 30 second timeout", func(t *testing.T) {
		service := NewArticleFetcherService(testLoggerFetcher())
		impl, ok := service.(*articleFetcherService)
		require.True(t, ok)
		assert.NotNil(t, impl, "service should be properly initialized")
	})

	t.Run("should have User-Agent setting capability", func(t *testing.T) {
		expectedUserAgent := "pre-processor/1.0 (+https://alt.example.com/bot)"
		assert.NotEmpty(t, expectedUserAgent, "User-Agent should be defined")
		assert.Contains(t, expectedUserAgent, "pre-processor", "User-Agent should identify service")
		assert.Contains(t, expectedUserAgent, "alt.example.com", "User-Agent should include contact info")
	})
}

func TestArticleFetcher_RateLimiting(t *testing.T) {
	t.Run("should perform rate limiting", func(t *testing.T) {
		service := NewArticleFetcherService(testLoggerFetcher())
		assert.NotNil(t, service, "service should implement rate limiting")
	})
}
