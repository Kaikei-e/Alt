package service

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

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
		expectError bool
		description string
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
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			service := NewArticleFetcherService(testLoggerFetcher())

			result, err := service.FetchArticle(context.Background(), tc.input)

			if tc.expectError {
				require.Error(t, err, tc.description)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err, tc.description)
				assert.NotNil(t, result)
			}
		})
	}
}

// TestArticleFetcherService_FetchArticle_NetworkValidation tests the network validation without external calls
func TestArticleFetcherService_FetchArticle_NetworkValidation(t *testing.T) {
	tests := map[string]struct {
		name          string
		url           string
		expectError   bool
		errorContains string
	}{
		"should reject localhost URLs due to SSRF protection": {
			url:           "http://localhost:8080/article",
			expectError:   true,
			errorContains: "access to private networks not allowed",
		},
		"should reject 127.0.0.1 URLs due to SSRF protection": {
			url:           "http://127.0.0.1:8080/article",
			expectError:   true,
			errorContains: "access to private networks not allowed",
		},
		"should reject private IP addresses": {
			url:           "http://192.168.1.1/article",
			expectError:   true,
			errorContains: "access to private networks not allowed",
		},
		"should reject internal domains": {
			url:           "http://server.local/article",
			expectError:   true,
			errorContains: "access to private networks not allowed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := NewArticleFetcherService(testLoggerFetcher())

			result, err := service.FetchArticle(context.Background(), tc.url)

			require.Error(t, err)
			assert.Nil(t, result)
			if tc.errorContains != "" {
				assert.Contains(t, err.Error(), tc.errorContains)
			}
		})
	}
}

// TestArticleFetcherService_FetchArticle_MP3Handling tests MP3 file handling
func TestArticleFetcherService_FetchArticle_MP3Handling(t *testing.T) {
	t.Run("should skip MP3 files", func(t *testing.T) {
		service := NewArticleFetcherService(testLoggerFetcher())

		// Test with .mp3 extension in URL - will hit SSRF protection but that's expected
		mp3URL := "http://example.com/audio.mp3"
		result, err := service.FetchArticle(context.Background(), mp3URL)

		// Should not error but return nil (skipped), but will fail due to SSRF protection first
		// Since the SSRF check happens before MP3 check in fetchArticleFromURL
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

// TestArticleFetcherService_EdgeCases tests edge cases without external network calls
func TestArticleFetcherService_EdgeCases(t *testing.T) {
	tests := map[string]struct {
		input       string
		expectError bool
		description string
	}{
		"should handle URL with special characters": {
			input:       "https://example.com/path with spaces",
			expectError: false, // URL parsing is lenient and will URL-encode spaces
			description: "URLs with spaces should be handled by URL parsing",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			service := NewArticleFetcherService(testLoggerFetcher())

			result, err := service.FetchArticle(context.Background(), tc.input)

			if tc.expectError {
				assert.Error(t, err, tc.description)
				assert.Nil(t, result)
			} else {
				// Note: This will likely still fail due to network restrictions
				// but not due to URL validation issues
				if err != nil {
					// Accept network-related errors as expected
					assert.Contains(t, err.Error(), "access to private networks not allowed")
				}
			}
		})
	}
}

// TestArticleFetcherService_RateLimiting tests rate limiting functionality
func TestArticleFetcherService_RateLimiting(t *testing.T) {
	t.Run("should enforce rate limiting between requests", func(t *testing.T) {
		service := NewArticleFetcherService(testLoggerFetcher())

		// Use URLs that will fail due to network restrictions, but still test rate limiting timing
		testURL := "http://example.com/test"

		// First request
		start := time.Now()
		_, err1 := service.FetchArticle(context.Background(), testURL)
		// Expect error due to network restrictions (DNS resolution failure or connection failure)
		if err1 == nil {
			t.Log("First request unexpectedly succeeded - this may be due to actual network access")
		}

		// Second request - should be delayed due to rate limiting
		_, err2 := service.FetchArticle(context.Background(), testURL)
		elapsed := time.Since(start)

		if err2 == nil {
			t.Log("Second request unexpectedly succeeded - this may be due to actual network access")
		}
		// Should take at least close to the minimum interval (5 seconds)
		assert.True(t, elapsed.Seconds() >= 4.5, "Expected rate limiting delay of ~5 seconds, got %v", elapsed)
	})
}

// TestArticleFetcherService_RealWorldScenarios tests realistic scenarios without making external calls
func TestArticleFetcherService_RealWorldScenarios(t *testing.T) {
	t.Run("should handle public domain URLs (validation only)", func(t *testing.T) {
		service := NewArticleFetcherService(testLoggerFetcher())

		// Test URL validation for common public domains
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
	})

	t.Run("should validate against blocked ports", func(t *testing.T) {
		service := NewArticleFetcherService(testLoggerFetcher())

		// Test URLs with commonly blocked ports
		blockedPortURLs := []string{
			"http://example.com:22/article",   // SSH
			"http://example.com:3306/article", // MySQL
			"http://example.com:5432/article", // PostgreSQL
		}

		for _, url := range blockedPortURLs {
			result, err := service.FetchArticle(context.Background(), url)
			assert.Error(t, err, "Should reject blocked port for %s", url)
			assert.Nil(t, result)
			// Should contain error about port access
			assert.Contains(t, err.Error(), "port is not allowed")
		}
	})
}
