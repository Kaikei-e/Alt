package integration_tests

import (
	"alt/validation"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationIntegration(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		validatorType string
		input         interface{}
		expectValid   bool
		expectedErr   string
	}{
		// Feed URL validation tests
		{
			name:          "valid HTTP URL",
			validatorType: "feed_url",
			input:         "http://example.com/feed.xml",
			expectValid:   true,
		},
		{
			name:          "valid HTTPS URL",
			validatorType: "feed_url",
			input:         "https://example.com/feed.xml",
			expectValid:   true,
		},
		{
			name:          "invalid URL scheme",
			validatorType: "feed_url",
			input:         "ftp://example.com/feed.xml",
			expectValid:   false,
			expectedErr:   "URL must use HTTP or HTTPS scheme",
		},
		{
			name:          "private IP address blocked",
			validatorType: "feed_url",
			input:         "http://192.168.1.1/feed.xml",
			expectValid:   false,
			expectedErr:   "Access to private networks not allowed for security reasons",
		},
		{
			name:          "localhost blocked",
			validatorType: "feed_url",
			input:         "http://localhost/feed.xml",
			expectValid:   false,
			expectedErr:   "Access to localhost not allowed for security reasons",
		},
		{
			name:          "cloud metadata endpoint blocked",
			validatorType: "feed_url",
			input:         "http://169.254.169.254/metadata",
			expectValid:   false,
			expectedErr:   "Access to metadata endpoints not allowed for security reasons",
		},

		// Search query validation tests
		{
			name:          "valid search query",
			validatorType: "search_query",
			input:         "technology news",
			expectValid:   true,
		},
		{
			name:          "empty search query",
			validatorType: "search_query",
			input:         "",
			expectValid:   false,
			expectedErr:   "Search query cannot be empty",
		},
		{
			name:          "search query too long",
			validatorType: "search_query",
			input:         string(make([]byte, 1001)), // 1001 characters
			expectValid:   false,
			expectedErr:   "Search query too long (maximum 1000 characters)",
		},
		{
			name:          "search query with special characters",
			validatorType: "search_query",
			input:         "search with @#$%^&*() characters",
			expectValid:   true,
		},

		// Pagination validation tests
		{
			name:          "valid pagination limit",
			validatorType: "pagination",
			input: map[string]interface{}{
				"limit": 50,
				"page":  0,
			},
			expectValid: true,
		},
		{
			name:          "pagination limit too high",
			validatorType: "pagination",
			input: map[string]interface{}{
				"limit": 2000,
				"page":  0,
			},
			expectValid: false,
			expectedErr: "limit exceeds maximum",
		},
		{
			name:          "negative pagination limit",
			validatorType: "pagination",
			input: map[string]interface{}{
				"limit": -1,
				"page":  0,
			},
			expectValid: false,
			expectedErr: "limit must be positive",
		},
		{
			name:          "negative page number",
			validatorType: "pagination",
			input: map[string]interface{}{
				"limit": 10,
				"page":  -1,
			},
			expectValid: false,
			expectedErr: "page must be non-negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error

			switch tt.validatorType {
			case "feed_url":
				url, ok := tt.input.(string)
				require.True(t, ok, "Input should be string for feed URL validation")
				err = validation.ValidateFeedURL(ctx, url)

			case "search_query":
				query, ok := tt.input.(string)
				require.True(t, ok, "Input should be string for search query validation")
				err = validation.ValidateSearchQuery(ctx, query)

			case "pagination":
				params, ok := tt.input.(map[string]interface{})
				require.True(t, ok, "Input should be map for pagination validation")

				limit, limitOk := params["limit"].(int)
				page, pageOk := params["page"].(int)
				require.True(t, limitOk && pageOk, "Pagination params should be integers")

				err = validation.ValidatePagination(ctx, limit, page)

			default:
				t.Fatalf("Unknown validator type: %s", tt.validatorType)
			}

			if tt.expectValid {
				assert.NoError(t, err, "Validation should pass")
			} else {
				require.Error(t, err, "Validation should fail")
				if tt.expectedErr != "" {
					assert.Contains(t, err.Error(), tt.expectedErr, "Error message should contain expected text")
				}
			}
		})
	}
}

func TestSSRFProtectionIntegration(t *testing.T) {
	// Test comprehensive SSRF protection
	tests := []struct {
		name        string
		url         string
		expectValid bool
		reason      string
	}{
		{
			name:        "legitimate public URL",
			url:         "https://feeds.example.com/rss.xml",
			expectValid: true,
		},
		{
			name:        "private IP 10.x.x.x",
			url:         "http://10.0.0.1/feed.xml",
			expectValid: false,
			reason:      "private IP range",
		},
		{
			name:        "private IP 172.16-31.x.x",
			url:         "http://172.20.0.1/feed.xml",
			expectValid: false,
			reason:      "private IP range",
		},
		{
			name:        "private IP 192.168.x.x",
			url:         "http://192.168.0.1/feed.xml",
			expectValid: false,
			reason:      "private IP range",
		},
		{
			name:        "localhost",
			url:         "http://localhost/feed.xml",
			expectValid: false,
			reason:      "localhost blocked",
		},
		{
			name:        "127.0.0.1",
			url:         "http://127.0.0.1/feed.xml",
			expectValid: false,
			reason:      "loopback address",
		},
		{
			name:        "0.0.0.0",
			url:         "http://0.0.0.0/feed.xml",
			expectValid: false,
			reason:      "unspecified address",
		},
		{
			name:        "cloud metadata service AWS",
			url:         "http://169.254.169.254/latest/meta-data/",
			expectValid: false,
			reason:      "cloud metadata endpoint",
		},
		{
			name:        "internal domain .local",
			url:         "http://server.local/feed.xml",
			expectValid: false,
			reason:      "internal domain suffix",
		},
		{
			name:        "internal domain .internal",
			url:         "http://api.internal/feed.xml",
			expectValid: false,
			reason:      "internal domain suffix",
		},
		{
			name:        "internal domain .corp",
			url:         "http://intranet.corp/feed.xml",
			expectValid: false,
			reason:      "internal domain suffix",
		},
		{
			name:        "internal domain .lan",
			url:         "http://router.lan/feed.xml",
			expectValid: false,
			reason:      "internal domain suffix",
		},
		{
			name:        "non-HTTP scheme file://",
			url:         "file:///etc/passwd",
			expectValid: false,
			reason:      "unsupported scheme",
		},
		{
			name:        "non-HTTP scheme ftp://",
			url:         "ftp://example.com/file.xml",
			expectValid: false,
			reason:      "unsupported scheme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := validation.ValidateFeedURL(ctx, tt.url)

			if tt.expectValid {
				assert.NoError(t, err, "URL should be valid: %s", tt.reason)
			} else {
				assert.Error(t, err, "URL should be invalid: %s", tt.reason)
			}
		})
	}
}

func TestInputSanitizationIntegration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal text unchanged",
			input:    "technology news",
			expected: "technology news",
		},
		{
			name:     "HTML tags removed",
			input:    "<script>alert('xss')</script>news",
			expected: "news",
		},
		{
			name:     "SQL injection attempts neutralized",
			input:    "'; DROP TABLE feeds; --",
			expected: "'; DROP TABLE feeds; --", // Should be escaped, not executed
		},
		{
			name:     "unicode characters preserved",
			input:    "технологии новости",
			expected: "технологии новости",
		},
		{
			name:     "emoji preserved",
			input:    "tech news 📰🚀",
			expected: "tech news 📰🚀",
		},
		{
			name:     "excessive whitespace normalized",
			input:    "  multiple    spaces   ",
			expected: "multiple spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			sanitized := validation.SanitizeInput(ctx, tt.input)
			assert.Equal(t, tt.expected, sanitized)
		})
	}
}

func TestValidationErrorHandling(t *testing.T) {
	ctx := context.Background()

	// Test that validation errors contain proper context
	tests := []struct {
		name           string
		operation      func() error
		expectedFields []string
	}{
		{
			name: "feed URL validation error",
			operation: func() error {
				return validation.ValidateFeedURL(ctx, "invalid-url")
			},
			expectedFields: []string{"url", "validation_type"},
		},
		{
			name: "search query validation error",
			operation: func() error {
				return validation.ValidateSearchQuery(ctx, "")
			},
			expectedFields: []string{"query", "validation_type"},
		},
		{
			name: "pagination validation error",
			operation: func() error {
				return validation.ValidatePagination(ctx, -1, 0)
			},
			expectedFields: []string{"limit", "page", "validation_type"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.operation()
			require.Error(t, err, "Operation should return error")

			// Check if error has proper context
			validationErr, ok := validation.AsValidationError(err)
			require.True(t, ok, "Error should be a validation error")

			// Verify expected fields are present in error context
			for _, field := range tt.expectedFields {
				assert.Contains(t, validationErr.Fields, field,
					"Validation error should contain field: %s", field)
			}
		})
	}
}

func TestConcurrentValidation(t *testing.T) {
	// Test that validation is thread-safe
	ctx := context.Background()

	// Test multiple goroutines validating different URLs concurrently
	urls := []string{
		"https://example1.com/feed.xml",
		"https://example2.com/feed.xml",
		"https://example3.com/feed.xml",
		"http://192.168.1.1/feed.xml", // Invalid
		"https://example4.com/feed.xml",
	}

	results := make(chan error, len(urls))

	// Start concurrent validations
	for _, url := range urls {
		go func(u string) {
			err := validation.ValidateFeedURL(ctx, u)
			results <- err
		}(url)
	}

	// Collect results
	validCount := 0
	invalidCount := 0

	for i := 0; i < len(urls); i++ {
		err := <-results
		if err == nil {
			validCount++
		} else {
			invalidCount++
		}
	}

	// Verify expected results
	assert.Equal(t, 4, validCount, "Should have 4 valid URLs")
	assert.Equal(t, 1, invalidCount, "Should have 1 invalid URL")
}
