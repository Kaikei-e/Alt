package service

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLoggerFetcher() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Only errors in tests
	}))
}

func TestArticleFetcherService_InterfaceCompliance(t *testing.T) {
	t.Run("should implement ArticleFetcherService interface", func(t *testing.T) {
		// GREEN PHASE: Test that service implements interface
		service := NewArticleFetcherService(testLoggerFetcher())

		// Verify interface compliance at compile time
		var _ ArticleFetcherService = service

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
		expectError bool
	}{
		"should handle malformed URL": {
			input:       "://invalid",
			expectError: true,
			description: "URL parsing should fail",
		},
		"should handle empty URL": {
			input:       "",
			expectError: true,
			description: "Empty URL should be rejected",
		},
		"should reject non-HTTP schemes": {
			input:       "ftp://example.com",
			expectError: true,
			description: "Non-HTTP schemes should be rejected by article fetcher",
		},
		"should skip MP3 files": {
			input:       "https://example.com/audio.mp3",
			expectError: false,
			description: "MP3 files should be skipped and return nil without error",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			service := NewArticleFetcherService(testLoggerFetcher())

			result, err := service.FetchArticle(context.Background(), tc.input)

			if tc.expectError {
				require.Error(t, err, tc.description)
				assert.Nil(t, result)
			} else {
				// For MP3 files, expect no error but nil result (skipped)
				if strings.Contains(tc.input, ".mp3") {
					require.NoError(t, err, tc.description)
					assert.Nil(t, result, "MP3 files should return nil result")
				} else {
					require.NoError(t, err, tc.description)
					assert.NotNil(t, result)
				}
			}
		})
	}
}

// TestArticleFetcherService_PublicURLValidation tests URL validation for public domains.
func TestArticleFetcherService_PublicURLValidation(t *testing.T) {
	service := NewArticleFetcherService(testLoggerFetcher())

	// Test URL validation for common public domains (validation only, no network calls)
	publicURLs := []string{
		"https://example.com/article",
		"https://news.example.org/story",
		"http://blog.example.net/post/123",
	}

	for _, url := range publicURLs {
		// We only test URL validation, not actual fetching
		err := service.ValidateURL(url)
		assert.NoError(t, err, "URL validation should pass for %s", url)
	}
}
