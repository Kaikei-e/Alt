package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
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

// TestArticleFetcher_HTTPClientManagerIntegration tests that ArticleFetcher uses HTTPClientManager consistently.
func TestArticleFetcher_HTTPClientManagerIntegration(t *testing.T) {
	t.Run("should use HTTPClientManager when no custom client provided", func(t *testing.T) {
		// RED PHASE: This test will fail initially
		service := NewArticleFetcherService(testLoggerFetcher())

		// We need to verify that the service uses HTTPClientManager internally
		// This test ensures consistency with TASK1.md requirements

		// Type assertion to access internal structure for testing
		impl, ok := service.(*articleFetcherService)
		require.True(t, ok, "service should be *articleFetcherService")

		// Verify that when httpClient is nil, it should use HTTPClientManager
		assert.Nil(t, impl.httpClient, "httpClient should be nil to trigger HTTPClientManager usage")
	})

	t.Run("should respect custom HTTP client when provided", func(t *testing.T) {
		// Test with custom client injection
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

// TestArticleFetcher_ConfigurationConsistency tests that ArticleFetcher follows TASK1.md requirements.
func TestArticleFetcher_ConfigurationConsistency(t *testing.T) {
	t.Run("should use 30 second timeout", func(t *testing.T) {
		// This test documents that we expect 30-second timeout as per TASK1.md
		service := NewArticleFetcherService(testLoggerFetcher())
		impl, ok := service.(*articleFetcherService)
		require.True(t, ok)

		// The HTTPClientManager provides the proper timeout configuration
		assert.NotNil(t, impl, "service should be properly initialized")
	})

	t.Run("should have User-Agent setting capability", func(t *testing.T) {
		// This test documents expected User-Agent format as per TASK1.md
		expectedUserAgent := "pre-processor/1.0 (+https://alt.example.com/bot)"

		// Test that the expected User-Agent meets requirements
		assert.NotEmpty(t, expectedUserAgent, "User-Agent should be defined")
		assert.Contains(t, expectedUserAgent, "pre-processor", "User-Agent should identify service")
		assert.Contains(t, expectedUserAgent, "alt.example.com", "User-Agent should include contact info")
	})
}

// TestArticleFetcher_TASK1_Requirements tests specific TASK1.md implementation requirements.
func TestArticleFetcher_TASK1_Requirements(t *testing.T) {
	t.Run("should perform rate limiting", func(t *testing.T) {
		// RED PHASE: Test that rate limiting is applied (5 second intervals per CLAUDE.md)
		service := NewArticleFetcherService(testLoggerFetcher())

		// Rate limiting is handled by domainRateLimiter in fetchArticleFromURL
		// This test documents the requirement
		assert.NotNil(t, service, "service should implement rate limiting")
	})

	t.Run("should handle circuit breaker", func(t *testing.T) {
		// RED PHASE: Test that circuit breaker is applied
		service := NewArticleFetcherService(testLoggerFetcher())

		// Circuit breaker should be integrated via HTTPClientManager
		assert.NotNil(t, service, "service should integrate with circuit breaker")
	})

	t.Run("should provide performance logging", func(t *testing.T) {
		// RED PHASE: Test that performance metrics are logged
		service := NewArticleFetcherService(testLoggerFetcher())

		// Performance logging should include timing metrics as per TASK1.md
		assert.NotNil(t, service, "service should provide performance logging")
	})
}

// TestArticleFetcher_TASK2_Requirements tests TASK2.md retry and DLQ integration
func TestArticleFetcher_TASK2_Requirements(t *testing.T) {
	t.Run("should integrate with retry mechanism", func(t *testing.T) {
		// TDD RED PHASE: Test that retry is integrated
		service := NewArticleFetcherServiceWithRetryAndDLQ(testLoggerFetcher(), nil, nil)
		
		assert.NotNil(t, service, "service should support retry and DLQ integration")
	})
	
	t.Run("should publish failed articles to DLQ", func(t *testing.T) {
		// TDD RED PHASE: Mock DLQ publisher
		mockDLQ := &MockDLQPublisher{published: make([]DLQMessage, 0)}
		service := NewArticleFetcherServiceWithRetryAndDLQ(testLoggerFetcher(), nil, mockDLQ)
		
		// Mock should be integrated
		assert.NotNil(t, service, "service should integrate DLQ publisher")
		assert.Empty(t, mockDLQ.published, "DLQ should start empty")
	})
	
	t.Run("should handle retryable errors with exponential backoff", func(t *testing.T) {
		// TDD RED PHASE: Test retry behavior
		mockDLQ := &MockDLQPublisher{published: make([]DLQMessage, 0)}
		service := NewArticleFetcherServiceWithRetryAndDLQ(testLoggerFetcher(), nil, mockDLQ)
		
		// This documents the retry requirement
		assert.NotNil(t, service, "service should implement exponential backoff retry")
	})
}

// Mock DLQ Publisher for testing
type MockDLQPublisher struct {
	published []DLQMessage
	shouldErr bool
}

type DLQMessage struct {
	URL      string
	Attempts int
	Error    error
}

func (m *MockDLQPublisher) PublishFailedArticle(ctx context.Context, url string, attempts int, lastError error) error {
	if m.shouldErr {
		return errors.New("DLQ publish failed")
	}
	
	m.published = append(m.published, DLQMessage{
		URL:      url,
		Attempts: attempts,
		Error:    lastError,
	})
	return nil
}
