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
			name:     "URL with normal query parameters (all removed)",
			input:    "https://example.com/search?q=golang&page=2",
			expected: "https://example.com/search",
			wantErr:  false,
		},
		{
			name:     "URL with mixed parameters (all removed)",
			input:    "https://example.com/search?q=golang&utm_source=rss&page=2",
			expected: "https://example.com/search",
			wantErr:  false,
		},
		{
			name:     "Root path with trailing slash (should remove slash)",
			input:    "https://example.com/",
			expected: "https://example.com",
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
		{
			name:     "Japanese URL with lowercase percent-encoding and trailing slash",
			input:    "https://tech.example.com/ai%e3%81%af%e3%80%8c%e3%82%b9%e3%82%ad%e3%83%ab%e3%81%ae%e5%85%a8%e7%b5%84%e3%81%bf%e5%90%88%e3%82%8f%e3%81%9b%e8%a9%95%e4%be%a1%e3%80%8d%e3%81%ae%e5%a4%a2%e3%82%92%e8%a6%8b%e3%82%8b%e3%81%8b%ef%bc%9f/",
			expected: "https://tech.example.com/ai%E3%81%AF%E3%80%8C%E3%82%B9%E3%82%AD%E3%83%AB%E3%81%AE%E5%85%A8%E7%B5%84%E3%81%BF%E5%90%88%E3%82%8F%E3%81%9B%E8%A9%95%E4%BE%A1%E3%80%8D%E3%81%AE%E5%A4%A2%E3%82%92%E8%A6%8B%E3%82%8B%E3%81%8B%EF%BC%9F",
			wantErr:  false,
		},
		{
			name:     "Japanese URL with uppercase percent-encoding",
			input:    "https://tech.example.com/ai%E3%81%AF%E3%80%8C%E3%82%B9%E3%82%AD%E3%83%AB%E3%81%AE%E5%85%A8%E7%B5%84%E3%81%BF%E5%90%88%E3%82%8F%E3%81%9B%E8%A9%95%E4%BE%A1%E3%80%8D%E3%81%AE%E5%A4%A2%E3%82%92%E8%A6%8B%E3%82%8B%E3%81%8B%EF%BC%9F",
			expected: "https://tech.example.com/ai%E3%81%AF%E3%80%8C%E3%82%B9%E3%82%AD%E3%83%AB%E3%81%AE%E5%85%A8%E7%B5%84%E3%81%BF%E5%90%88%E3%82%8F%E3%81%9B%E8%A9%95%E4%BE%A1%E3%80%8D%E3%81%AE%E5%A4%A2%E3%82%92%E8%A6%8B%E3%82%8B%E3%81%8B%EF%BC%9F",
			wantErr:  false,
		},
		{
			name:     "Japanese URL with UTM parameters",
			input:    "https://tech.example.com/article%e3%81%82/?utm_source=rss",
			expected: "https://tech.example.com/article%E3%81%82",
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

func TestURLsEqual(t *testing.T) {
	tests := []struct {
		name     string
		url1     string
		url2     string
		expected bool
	}{
		{
			name:     "Identical URLs",
			url1:     "https://example.com/article",
			url2:     "https://example.com/article",
			expected: true,
		},
		{
			name:     "Different URLs",
			url1:     "https://example.com/article1",
			url2:     "https://example.com/article2",
			expected: false,
		},
		{
			name:     "Percent-encoding case difference (lowercase vs uppercase)",
			url1:     "https://example.com/path%e3%81%82",
			url2:     "https://example.com/path%E3%81%82",
			expected: true,
		},
		{
			name:     "Japanese URL - lowercase encoding",
			url1:     "https://tech.drecom.co.jp/ai%e3%81%af%e3%80%8c%e3%82%b9%e3%82%ad%e3%83%ab%e3%81%ae%e5%85%a8%e7%b5%84%e3%81%bf%e5%90%88%e3%82%8f%e3%81%9b%e8%a9%95%e4%be%a1%e3%80%8d%e3%81%ae%e5%a4%a2%e3%82%92%e8%a6%8b%e3%82%8b%e3%81%8b%ef%bc%9f",
			url2:     "https://tech.drecom.co.jp/ai%E3%81%AF%E3%80%8C%E3%82%B9%E3%82%AD%E3%83%AB%E3%81%AE%E5%85%A8%E7%B5%84%E3%81%BF%E5%90%88%E3%82%8F%E3%81%9B%E8%A9%95%E4%BE%A1%E3%80%8D%E3%81%AE%E5%A4%A2%E3%82%92%E8%A6%8B%E3%82%8B%E3%81%8B%EF%BC%9F",
			expected: true,
		},
		{
			name:     "Mixed case in domain (should be case-insensitive)",
			url1:     "https://Example.COM/article",
			url2:     "https://example.com/article",
			expected: true,
		},
		{
			name:     "Different domains",
			url1:     "https://example.com/article",
			url2:     "https://different.com/article",
			expected: false,
		},
		{
			name:     "Same content different schemes",
			url1:     "http://example.com/article",
			url2:     "https://example.com/article",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := URLsEqual(tt.url1, tt.url2)
			if result != tt.expected {
				t.Errorf("URLsEqual(%q, %q) = %v, want %v", tt.url1, tt.url2, result, tt.expected)
			}
		})
	}
}

func TestURLsEqual_WithNormalization(t *testing.T) {
	// Test that URLsEqual works correctly with normalized URLs
	tests := []struct {
		name     string
		url1     string
		url2     string
		expected bool
	}{
		{
			name:     "Both URLs normalized - Japanese lowercase encoding",
			url1:     "https://tech.drecom.co.jp/ai%e3%81%af%e3%80%8c%e3%82%b9%e3%82%ad%e3%83%ab%e3%81%ae%e5%85%a8%e7%b5%84%e3%81%bf%e5%90%88%e3%82%8f%e3%81%9b%e8%a9%95%e4%be%a1%e3%80%8d%e3%81%ae%e5%a4%a2%e3%82%92%e8%a6%8b%e3%82%8b%e3%81%8b%ef%bc%9f/",
			url2:     "https://tech.drecom.co.jp/ai%e3%81%af%e3%80%8c%e3%82%b9%e3%82%ad%e3%83%ab%e3%81%ae%e5%85%a8%e7%b5%84%e3%81%bf%e5%90%88%e3%82%8f%e3%81%9b%e8%a9%95%e4%be%a1%e3%80%8d%e3%81%ae%e5%a4%a2%e3%82%92%e8%a6%8b%e3%82%8b%e3%81%8b%ef%bc%9f",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Normalize both URLs first
			normalized1, err1 := NormalizeURL(tt.url1)
			if err1 != nil {
				t.Fatalf("Failed to normalize url1: %v", err1)
			}

			normalized2, err2 := NormalizeURL(tt.url2)
			if err2 != nil {
				t.Fatalf("Failed to normalize url2: %v", err2)
			}

			// Compare normalized URLs
			result := URLsEqual(normalized1, normalized2)
			if result != tt.expected {
				t.Errorf("URLsEqual(NormalizeURL(%q), NormalizeURL(%q)) = %v, want %v\nnormalized1: %s\nnormalized2: %s",
					tt.url1, tt.url2, result, tt.expected, normalized1, normalized2)
			}
		})
	}
}
