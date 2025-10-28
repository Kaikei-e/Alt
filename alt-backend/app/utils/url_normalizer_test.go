package utils

import (
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "URL with UTM parameters",
			input:    "https://example.com/article?utm_source=rss&utm_medium=rss&utm_campaign=test",
			expected: "https://example.com/article",
			wantErr:  false,
		},
		{
			name:     "URL with trailing slash",
			input:    "https://example.com/article/",
			expected: "https://example.com/article",
			wantErr:  false,
		},
		{
			name:     "URL with UTM parameters and trailing slash",
			input:    "https://example.com/article/?utm_source=rss",
			expected: "https://example.com/article",
			wantErr:  false,
		},
		{
			name:     "URL with fragment",
			input:    "https://example.com/article#section1",
			expected: "https://example.com/article",
			wantErr:  false,
		},
		{
			name:     "URL with fragment and UTM parameters",
			input:    "https://example.com/article?utm_source=rss#section1",
			expected: "https://example.com/article",
			wantErr:  false,
		},
		{
			name:     "URL with multiple tracking parameters",
			input:    "https://example.com/article?utm_source=rss&fbclid=abc123&gclid=xyz789",
			expected: "https://example.com/article",
			wantErr:  false,
		},
		{
			name:     "URL with normal query parameters (should be preserved)",
			input:    "https://example.com/search?q=golang&page=2",
			expected: "https://example.com/search?page=2&q=golang",
			wantErr:  false,
		},
		{
			name:     "URL with mixed parameters",
			input:    "https://example.com/search?q=golang&utm_source=rss&page=2",
			expected: "https://example.com/search?page=2&q=golang",
			wantErr:  false,
		},
		{
			name:     "Root path with trailing slash (should keep slash)",
			input:    "https://example.com/",
			expected: "https://example.com/",
			wantErr:  false,
		},
		{
			name:     "Root path without trailing slash",
			input:    "https://example.com",
			expected: "https://example.com",
			wantErr:  false,
		},
		{
			name:     "Simple URL without parameters",
			input:    "https://example.com/article",
			expected: "https://example.com/article",
			wantErr:  false,
		},
		{
			name:     "URL with all UTM variants",
			input:    "https://example.com/article?utm_source=rss&utm_medium=feed&utm_campaign=spring&utm_term=test&utm_content=link&utm_id=123",
			expected: "https://example.com/article",
			wantErr:  false,
		},
		{
			name:     "Real-world example from logs",
			input:    "https://www.nationalelfservice.net/treatment/complementary-and-alternative/from-pills-to-people-the-rise-of-social-prescribing/?utm_source=rss&utm_medium=rss&utm_campaign=from-pills-to-people-the-rise-of-social-prescribing",
			expected: "https://www.nationalelfservice.net/treatment/complementary-and-alternative/from-pills-to-people-the-rise-of-social-prescribing",
			wantErr:  false,
		},
		{
			name:     "Invalid URL",
			input:    "://invalid-url",
			expected: "",
			wantErr:  true,
		},
		{
			name:     "URL with msclkid (Microsoft Click ID)",
			input:    "https://example.com/article?msclkid=abc123",
			expected: "https://example.com/article",
			wantErr:  false,
		},
		{
			name:     "URL with mc_eid (MailChimp Email ID)",
			input:    "https://example.com/article?mc_eid=xyz789",
			expected: "https://example.com/article",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NormalizeURL(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NormalizeURL() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("NormalizeURL() unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("NormalizeURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNormalizeURL_Consistency(t *testing.T) {
	// Test that normalizing the same URL multiple times produces the same result
	url := "https://example.com/article?utm_source=rss&utm_campaign=test/"

	result1, err1 := NormalizeURL(url)
	result2, err2 := NormalizeURL(url)

	if err1 != nil || err2 != nil {
		t.Fatalf("NormalizeURL() returned error: %v, %v", err1, err2)
	}

	if result1 != result2 {
		t.Errorf("NormalizeURL() not consistent: %v != %v", result1, result2)
	}
}

func TestNormalizeURL_Idempotent(t *testing.T) {
	// Test that normalizing an already normalized URL doesn't change it
	url := "https://example.com/article"

	result1, err1 := NormalizeURL(url)
	if err1 != nil {
		t.Fatalf("NormalizeURL() returned error: %v", err1)
	}

	result2, err2 := NormalizeURL(result1)
	if err2 != nil {
		t.Fatalf("NormalizeURL() returned error on second call: %v", err2)
	}

	if result1 != result2 {
		t.Errorf("NormalizeURL() not idempotent: %v != %v", result1, result2)
	}
}
