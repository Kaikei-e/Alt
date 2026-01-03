package utils

import "testing"

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "trailing slash should be removed",
			input:    "https://example.com/article/",
			expected: "https://example.com/article",
		},
		{
			name:     "URL without trailing slash should remain unchanged",
			input:    "https://example.com/article",
			expected: "https://example.com/article",
		},
		{
			name:     "root path should keep trailing slash",
			input:    "https://example.com/",
			expected: "https://example.com/",
		},
		{
			name:     "UTM parameters should be removed",
			input:    "https://example.com/article?utm_source=rss&utm_medium=feed",
			expected: "https://example.com/article",
		},
		{
			name:     "trailing slash with UTM parameters",
			input:    "https://example.com/article/?utm_source=rss",
			expected: "https://example.com/article",
		},
		{
			name:     "fragment should be removed",
			input:    "https://example.com/article#section",
			expected: "https://example.com/article",
		},
		{
			name:     "fbclid should be removed",
			input:    "https://example.com/article?fbclid=abc123",
			expected: "https://example.com/article",
		},
		{
			name:     "non-tracking params should be preserved",
			input:    "https://example.com/search?q=test&page=1",
			expected: "https://example.com/search?page=1&q=test",
		},
		{
			name:     "complex URL with mixed params",
			input:    "https://example.com/article?id=123&utm_source=rss&ref=homepage",
			expected: "https://example.com/article?id=123&ref=homepage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NormalizeURL(tt.input)
			if err != nil {
				t.Fatalf("NormalizeURL failed: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
