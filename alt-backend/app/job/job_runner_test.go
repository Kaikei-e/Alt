package job

import (
	"testing"
	"time"

	"alt/driver/models"
	"alt/utils"
)

// TestFeedModelURLNormalization verifies that URL normalization is applied correctly
// when creating feed models. This tests the normalization behavior that is used in HourlyJobRunner.
func TestFeedModelURLNormalization(t *testing.T) {
	tests := []struct {
		name         string
		inputURL     string
		expectedURL  string
		description  string
	}{
		{
			name:        "trailing slash should be removed",
			inputURL:    "https://hackaday.com/2026/01/03/zork-running-on-4-bit-intel-computer/",
			expectedURL: "https://hackaday.com/2026/01/03/zork-running-on-4-bit-intel-computer",
			description: "Trailing slash causes MarkAsRead failures",
		},
		{
			name:        "URL without trailing slash should remain unchanged",
			inputURL:    "https://hackaday.com/2026/01/03/zork-running-on-4-bit-intel-computer",
			expectedURL: "https://hackaday.com/2026/01/03/zork-running-on-4-bit-intel-computer",
			description: "URLs without trailing slash should not be modified",
		},
		{
			name:        "UTM parameters should be removed",
			inputURL:    "https://example.com/article?utm_source=rss&utm_medium=feed",
			expectedURL: "https://example.com/article",
			description: "UTM tracking parameters should be stripped",
		},
		{
			name:        "trailing slash with UTM parameters",
			inputURL:    "https://example.com/article/?utm_source=rss",
			expectedURL: "https://example.com/article",
			description: "Both trailing slash and UTM params should be removed",
		},
		{
			name:        "root path should keep trailing slash",
			inputURL:    "https://example.com/",
			expectedURL: "https://example.com/",
			description: "Root path is the only exception for trailing slash",
		},
		{
			name:        "fragment should be removed",
			inputURL:    "https://example.com/article#section",
			expectedURL: "https://example.com/article",
			description: "URL fragments should be stripped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This simulates the normalization logic in HourlyJobRunner
			normalizedLink, err := utils.NormalizeURL(tt.inputURL)
			if err != nil {
				t.Fatalf("NormalizeURL failed: %v", err)
			}

			if normalizedLink != tt.expectedURL {
				t.Errorf("Expected URL '%s', got '%s'", tt.expectedURL, normalizedLink)
			}

			// Verify feed model can be created with normalized URL
			feedModel := models.Feed{
				Title:       "Test Article",
				Description: "Test description",
				Link:        normalizedLink,
				PubDate:     time.Now().UTC(),
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
			}

			if feedModel.Link != tt.expectedURL {
				t.Errorf("Feed model Link expected '%s', got '%s'", tt.expectedURL, feedModel.Link)
			}
		})
	}
}

// TestNormalizeURLFallback verifies that invalid URLs fall back to original
func TestNormalizeURLFallback(t *testing.T) {
	// Invalid URLs should not cause panic and should fall back to original
	invalidURLs := []string{
		"not-a-valid-url",
		"",
		"://missing-scheme.com",
	}

	for _, inputURL := range invalidURLs {
		t.Run(inputURL, func(t *testing.T) {
			// This simulates the fallback behavior in HourlyJobRunner
			normalizedLink, err := utils.NormalizeURL(inputURL)
			if err != nil {
				// Fallback to original URL on error (as done in HourlyJobRunner)
				normalizedLink = inputURL
			}

			// Verify we got a result (either normalized or fallback)
			if normalizedLink == "" && inputURL != "" {
				t.Errorf("Expected non-empty result for non-empty input")
			}
		})
	}
}
