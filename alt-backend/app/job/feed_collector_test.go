package job

import (
	"strings"
	"testing"
	"time"

	rssFeed "github.com/mmcdole/gofeed"
)

func TestConvertFeedToFeedItem(t *testing.T) {
	tests := []struct {
		name           string
		feeds          []*rssFeed.Feed
		expectedCount  int
		expectedTitles []string
		description    string
	}{
		{
			name: "valid feed items",
			feeds: []*rssFeed.Feed{
				{
					Items: []*rssFeed.Item{
						{
							Title:           "Valid Article Title",
							Description:     "Valid article description",
							Link:            "https://example.com/article1",
							PublishedParsed: &time.Time{},
						},
						{
							Title:           "Another Valid Title",
							Description:     "Another valid description",
							Link:            "https://example.com/article2",
							PublishedParsed: &time.Time{},
						},
					},
				},
			},
			expectedCount:  2,
			expectedTitles: []string{"Valid Article Title", "Another Valid Title"},
			description:    "Should convert all valid feed items",
		},
		{
			name: "empty title feed items should be skipped",
			feeds: []*rssFeed.Feed{
				{
					Items: []*rssFeed.Item{
						{
							Title:           "",
							Description:     "Article with empty title",
							Link:            "https://example.com/empty-title",
							PublishedParsed: &time.Time{},
						},
						{
							Title:           "Valid Title",
							Description:     "Valid description",
							Link:            "https://example.com/valid",
							PublishedParsed: &time.Time{},
						},
					},
				},
			},
			expectedCount:  1,
			expectedTitles: []string{"Valid Title"},
			description:    "Should skip items with empty titles",
		},
		{
			name: "whitespace-only title feed items should be skipped",
			feeds: []*rssFeed.Feed{
				{
					Items: []*rssFeed.Item{
						{
							Title:           "   \t\n  ",
							Description:     "Article with whitespace-only title",
							Link:            "https://example.com/whitespace-title",
							PublishedParsed: &time.Time{},
						},
						{
							Title:           "Valid Title",
							Description:     "Valid description",
							Link:            "https://example.com/valid",
							PublishedParsed: &time.Time{},
						},
					},
				},
			},
			expectedCount:  1,
			expectedTitles: []string{"Valid Title"},
			description:    "Should skip items with whitespace-only titles",
		},
		{
			name: "404 page not found in description should be skipped",
			feeds: []*rssFeed.Feed{
				{
					Items: []*rssFeed.Item{
						{
							Title:           "Some Article Title",
							Description:     "404 page not found",
							Link:            "https://example.com/404-article",
							PublishedParsed: &time.Time{},
						},
						{
							Title:           "Valid Title",
							Description:     "Valid description",
							Link:            "https://example.com/valid",
							PublishedParsed: &time.Time{},
						},
					},
				},
			},
			expectedCount:  1,
			expectedTitles: []string{"Valid Title"},
			description:    "Should skip items with '404 page not found' in description",
		},
		{
			name: "404 in title should be skipped",
			feeds: []*rssFeed.Feed{
				{
					Items: []*rssFeed.Item{
						{
							Title:           "Error 404 - Page Not Found",
							Description:     "Some description",
							Link:            "https://example.com/404-article",
							PublishedParsed: &time.Time{},
						},
						{
							Title:           "Valid Title",
							Description:     "Valid description",
							Link:            "https://example.com/valid",
							PublishedParsed: &time.Time{},
						},
					},
				},
			},
			expectedCount:  1,
			expectedTitles: []string{"Valid Title"},
			description:    "Should skip items with '404' in title",
		},
		{
			name: "not found in title should be skipped",
			feeds: []*rssFeed.Feed{
				{
					Items: []*rssFeed.Item{
						{
							Title:           "Article Not Found",
							Description:     "Some description",
							Link:            "https://example.com/not-found-article",
							PublishedParsed: &time.Time{},
						},
						{
							Title:           "Valid Title",
							Description:     "Valid description",
							Link:            "https://example.com/valid",
							PublishedParsed: &time.Time{},
						},
					},
				},
			},
			expectedCount:  1,
			expectedTitles: []string{"Valid Title"},
			description:    "Should skip items with 'not found' in title",
		},
		{
			name: "title with leading/trailing whitespace should be trimmed",
			feeds: []*rssFeed.Feed{
				{
					Items: []*rssFeed.Item{
						{
							Title:           "  Valid Title With Whitespace  ",
							Description:     "Valid description",
							Link:            "https://example.com/whitespace",
							PublishedParsed: &time.Time{},
						},
					},
				},
			},
			expectedCount:  1,
			expectedTitles: []string{"Valid Title With Whitespace"},
			description:    "Should trim whitespace from titles",
		},
		{
			name: "mixed valid and invalid items",
			feeds: []*rssFeed.Feed{
				{
					Items: []*rssFeed.Item{
						{
							Title:           "",
							Description:     "Empty title item",
							Link:            "https://example.com/empty",
							PublishedParsed: &time.Time{},
						},
						{
							Title:           "Valid Article",
							Description:     "Valid description",
							Link:            "https://example.com/valid1",
							PublishedParsed: &time.Time{},
						},
						{
							Title:           "404 Error",
							Description:     "Some description",
							Link:            "https://example.com/404",
							PublishedParsed: &time.Time{},
						},
						{
							Title:           "Another Valid Article",
							Description:     "404 page not found",
							Link:            "https://example.com/valid2-but-404-desc",
							PublishedParsed: &time.Time{},
						},
						{
							Title:           "  Trimmed Title  ",
							Description:     "Valid description",
							Link:            "https://example.com/trimmed",
							PublishedParsed: &time.Time{},
						},
					},
				},
			},
			expectedCount:  2,
			expectedTitles: []string{"Valid Article", "Trimmed Title"},
			description:    "Should handle mixed valid and invalid items correctly",
		},
		{
			name: "empty feeds should return empty result",
			feeds: []*rssFeed.Feed{
				{
					Items: []*rssFeed.Item{},
				},
			},
			expectedCount:  0,
			expectedTitles: []string{},
			description:    "Should return empty slice for empty feeds",
		},
		{
			name: "multiple feeds with mixed content",
			feeds: []*rssFeed.Feed{
				{
					Items: []*rssFeed.Item{
						{
							Title:           "Feed 1 Article",
							Description:     "Valid description",
							Link:            "https://feed1.com/article",
							PublishedParsed: &time.Time{},
						},
					},
				},
				{
					Items: []*rssFeed.Item{
						{
							Title:           "",
							Description:     "Empty title from feed 2",
							Link:            "https://feed2.com/empty",
							PublishedParsed: &time.Time{},
						},
						{
							Title:           "Feed 2 Valid Article",
							Description:     "Valid description",
							Link:            "https://feed2.com/valid",
							PublishedParsed: &time.Time{},
						},
					},
				},
			},
			expectedCount:  2,
			expectedTitles: []string{"Feed 1 Article", "Feed 2 Valid Article"},
			description:    "Should handle multiple feeds with mixed content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			result := ConvertFeedToFeedItem(tt.feeds)

			// Assert
			if len(result) != tt.expectedCount {
				t.Errorf("Expected %d items, got %d items", tt.expectedCount, len(result))
			}

			// Check that all expected titles are present and correctly trimmed
			if len(tt.expectedTitles) > 0 {
				resultTitles := make([]string, len(result))
				for i, item := range result {
					resultTitles[i] = item.Title
				}

				for i, expectedTitle := range tt.expectedTitles {
					if i >= len(resultTitles) {
						t.Errorf("Expected title '%s' at index %d, but result has only %d items", expectedTitle, i, len(resultTitles))
						continue
					}
					if resultTitles[i] != expectedTitle {
						t.Errorf("Expected title '%s' at index %d, got '%s'", expectedTitle, i, resultTitles[i])
					}
				}
			}

			// Verify all returned items have non-empty titles
			for i, item := range result {
				if item.Title == "" {
					t.Errorf("Item at index %d has empty title", i)
				}
				// Verify titles don't contain 404 or suspicious content
				if containsSuspiciousContent(item.Title, item.Description) {
					t.Errorf("Item at index %d contains suspicious content: title='%s', desc='%s'", i, item.Title, item.Description)
				}
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		maxLength int
		expected  string
	}{
		{
			name:      "string shorter than max length",
			input:     "short",
			maxLength: 10,
			expected:  "short",
		},
		{
			name:      "string equal to max length",
			input:     "exact",
			maxLength: 5,
			expected:  "exact",
		},
		{
			name:      "string longer than max length",
			input:     "this is a very long string that needs truncation",
			maxLength: 10,
			expected:  "this is a ...",
		},
		{
			name:      "empty string",
			input:     "",
			maxLength: 5,
			expected:  "",
		},
		{
			name:      "max length zero",
			input:     "test",
			maxLength: 0,
			expected:  "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLength)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// Helper function to check if content contains suspicious patterns
func containsSuspiciousContent(title, description string) bool {
	titleLower := strings.ToLower(title)
	descLower := strings.ToLower(description)

	return strings.Contains(descLower, "404 page not found") ||
		   strings.Contains(titleLower, "404") ||
		   strings.Contains(titleLower, "not found")
}