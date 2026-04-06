package driver

import (
	"strings"
	"testing"
)

func TestMeilisearchDriver_SearchWithFilters(t *testing.T) {
	// Mock test - since we can't easily test against real Meilisearch in unit tests
	// This test verifies the method exists and handles basic scenarios

	// For now, we'll test the buildSecureFilter method directly
	driver := &MeilisearchDriver{}

	tests := []struct {
		name     string
		filters  []string
		expected string
	}{
		{
			name:     "empty filters",
			filters:  []string{},
			expected: "",
		},
		{
			name:     "single filter",
			filters:  []string{"technology"},
			expected: "tags = \"technology\"",
		},
		{
			name:     "multiple filters",
			filters:  []string{"technology", "programming"},
			expected: "tags = \"technology\" AND tags = \"programming\"",
		},
		{
			name:     "filters with quotes",
			filters:  []string{"tech\"malicious"},
			expected: "tags = \"tech\\\"malicious\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := driver.buildSecureFilter(tt.filters)
			if result != tt.expected {
				t.Errorf("buildSecureFilter(%v) = %q, want %q", tt.filters, result, tt.expected)
			}
		})
	}
}

func TestMeilisearchDriver_SearchWithFilters_SecurityValidation(t *testing.T) {
	driver := &MeilisearchDriver{}

	securityTests := []struct {
		name         string
		maliciousTag string
		description  string
	}{
		{
			name:         "SQL injection attempt",
			maliciousTag: "'; DROP TABLE articles; --",
			description:  "Should escape SQL injection attempts",
		},
		{
			name:         "Meilisearch filter bypass",
			maliciousTag: "tag\" OR \"admin",
			description:  "Should escape Meilisearch filter injection",
		},
		{
			name:         "Complex injection",
			maliciousTag: "tag\" OR (tags = \"admin\" AND secret = \"true\")",
			description:  "Should escape complex injection attempts",
		},
	}

	for _, tt := range securityTests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that malicious input is properly escaped
			result := driver.buildSecureFilter([]string{tt.maliciousTag})

			// Verify the result is properly escaped and wrapped in quotes
			if result != "" && !containsQuotedValue(result) {
				t.Errorf("buildSecureFilter should properly quote and escape malicious input: %s", tt.description)
			}

			// Verify no injection characters remain unescaped
			if result != "" && containsUnescapedQuotes(result) {
				t.Errorf("buildSecureFilter should escape all quotes in malicious input: %s", tt.description)
			}
		})
	}
}

// Helper function to check if the result contains properly quoted values
func containsQuotedValue(filter string) bool {
	// A properly formatted filter should look like: tags = "..."
	return strings.HasPrefix(filter, "tags = \"") && strings.HasSuffix(filter, "\"")
}

// Helper function to check for unescaped quotes inside the filter value
func containsUnescapedQuotes(filter string) bool {
	if !containsQuotedValue(filter) {
		// If not properly quoted, it's considered insecure for this test's purpose.
		return true
	}

	// Extract value from inside `tags = "..."`
	value := filter[len("tags = \"") : len(filter)-1]

	// Look for quotes that are not escaped with backslashes.
	// A quote is escaped if it is preceded by an odd number of backslashes.
	for i := 0; i < len(value); i++ {
		if value[i] == '"' {
			// This is a quote. Check if it's escaped.
			backslashes := 0
			for j := i - 1; j >= 0; j-- {
				if value[j] == '\\' {
					backslashes++
				} else {
					break
				}
			}
			if backslashes%2 == 0 {
				return true // Unescaped quote (preceded by an even number of backslashes)
			}
		}
	}
	return false
}

func TestContainsCJK_Japanese(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"イラン 石油", true},
		{"石油危機 原因", true},
		{"量子コンピュータ 実用化", true},
		{"Iran oil crisis", false},
		{"hello world", false},
		{"", false},
		{"AI関連ニュース", true},
		{"GPT-4o と Claude", true},              // と is Hiragana → CJK
		{"GPT-4oとClaude 3.5の違いは？", true},    // contains と, の, は (Hiragana)
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := containsCJK(tt.input)
			if got != tt.expected {
				t.Errorf("containsCJK(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func BenchmarkMeilisearchDriver_BuildSecureFilter(b *testing.B) {
	driver := &MeilisearchDriver{}
	filters := []string{"technology", "programming", "web-development", "data-science", "machine-learning"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		driver.buildSecureFilter(filters)
	}
}
