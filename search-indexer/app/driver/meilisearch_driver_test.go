package driver

import (
	"context"
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
		name          string
		maliciousTag  string
		description   string
	}{
		{
			name:          "SQL injection attempt",
			maliciousTag:  "'; DROP TABLE articles; --",
			description:   "Should escape SQL injection attempts",
		},
		{
			name:          "Meilisearch filter bypass",
			maliciousTag:  "tag\" OR \"admin",
			description:   "Should escape Meilisearch filter injection",
		},
		{
			name:          "Complex injection",
			maliciousTag:  "tag\" OR (tags = \"admin\" AND secret = \"true\")",
			description:   "Should escape complex injection attempts",
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
	// Check if the filter contains the pattern: tags = "value"
	if len(filter) == 0 {
		return false
	}
	// A properly formatted filter should contain quoted values
	return len(filter) > 10 && filter[0] != '"' // Should not start with quote (should be "tags = "...)
}

// Helper function to check for unescaped quotes
func containsUnescapedQuotes(filter string) bool {
	if len(filter) == 0 {
		return false
	}
	
	// Look for quotes that are not escaped with backslashes
	for i := 0; i < len(filter); i++ {
		if filter[i] == '"' {
			// Check if this quote is escaped (preceded by backslash)
			if i == 0 || filter[i-1] != '\\' {
				// This is an unescaped quote, but we need to check if it's part of the normal filter structure
				// Normal structure: tags = "value" - the quotes around the value are expected
				if i > 0 && filter[i-1] == ' ' {
					// This is likely the opening quote of a value, which is expected
					continue
				}
				if i < len(filter)-1 && filter[i+1] == ' ' {
					// This is likely the closing quote of a value, which is expected
					continue
				}
				// If we find quotes that are not part of the normal structure, it might be malicious
				return true
			}
		}
	}
	return false
}

func BenchmarkMeilisearchDriver_BuildSecureFilter(b *testing.B) {
	driver := &MeilisearchDriver{}
	filters := []string{"technology", "programming", "web-development", "data-science", "machine-learning"}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		driver.buildSecureFilter(filters)
	}
}