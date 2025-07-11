package search_engine

import (
	"testing"
)

func TestMakeSecureSearchFilter(t *testing.T) {
	tests := []struct {
		name     string
		tags     []string
		expected string
	}{
		{
			name:     "empty tags",
			tags:     []string{},
			expected: "",
		},
		{
			name:     "single tag",
			tags:     []string{"technology"},
			expected: "tags = \"technology\"",
		},
		{
			name:     "multiple tags",
			tags:     []string{"technology", "programming"},
			expected: "tags = \"technology\" AND tags = \"programming\"",
		},
		{
			name:     "tags with quotes",
			tags:     []string{"tech\"malicious"},
			expected: "tags = \"tech\\\"malicious\"",
		},
		{
			name:     "tags with backslashes",
			tags:     []string{"tech\\malicious"},
			expected: "tags = \"tech\\\\malicious\"",
		},
		{
			name:     "SQL injection attempt",
			tags:     []string{"'; DROP TABLE articles; --"},
			expected: "tags = \"'; DROP TABLE articles; --\"",
		},
		{
			name:     "Meilisearch injection with OR",
			tags:     []string{"tag\" OR \"malicious"},
			expected: "tags = \"tag\\\" OR \\\"malicious\"",
		},
		{
			name:     "Meilisearch injection with AND",
			tags:     []string{"tag\" AND \"malicious"},
			expected: "tags = \"tag\\\" AND \\\"malicious\"",
		},
		{
			name:     "Meilisearch injection with NOT",
			tags:     []string{"tag\" NOT \"malicious"},
			expected: "tags = \"tag\\\" NOT \\\"malicious\"",
		},
		{
			name:     "Complex injection attempt",
			tags:     []string{"tag\" OR (tags = \"admin\" AND secret = \"true\")"},
			expected: "tags = \"tag\\\" OR (tags = \\\"admin\\\" AND secret = \\\"true\\\")\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MakeSecureSearchFilter(tt.tags)
			if result != tt.expected {
				t.Errorf("MakeSecureSearchFilter(%v) = %q, want %q", tt.tags, result, tt.expected)
			}
		})
	}
}

func TestEscapeMeilisearchValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no special characters",
			input:    "technology",
			expected: "technology",
		},
		{
			name:     "single quote",
			input:    "tech'nology",
			expected: "tech'nology",
		},
		{
			name:     "double quote",
			input:    "tech\"nology",
			expected: "tech\\\"nology",
		},
		{
			name:     "backslash",
			input:    "tech\\nology",
			expected: "tech\\\\nology",
		},
		{
			name:     "multiple backslashes",
			input:    "tech\\\\nology",
			expected: "tech\\\\\\\\nology",
		},
		{
			name:     "backslash and quote",
			input:    "tech\\\"nology",
			expected: "tech\\\\\\\"nology",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only quotes",
			input:    "\"\"\"",
			expected: "\\\"\\\"\\\"",
		},
		{
			name:     "only backslashes",
			input:    "\\\\\\",
			expected: "\\\\\\\\\\\\",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EscapeMeilisearchValue(tt.input)
			if result != tt.expected {
				t.Errorf("EscapeMeilisearchValue(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidateFilterTags(t *testing.T) {
	tests := []struct {
		name    string
		tags    []string
		wantErr bool
	}{
		{
			name:    "valid tags",
			tags:    []string{"technology", "programming"},
			wantErr: false,
		},
		{
			name:    "empty tags",
			tags:    []string{},
			wantErr: false,
		},
		{
			name:    "valid tag with unicode",
			tags:    []string{"テクノロジー"},
			wantErr: false,
		},
		{
			name:    "valid tag with spaces",
			tags:    []string{"machine learning"},
			wantErr: false,
		},
		{
			name:    "valid tag with hyphens",
			tags:    []string{"web-development"},
			wantErr: false,
		},
		{
			name:    "valid tag with underscores",
			tags:    []string{"data_science"},
			wantErr: false,
		},
		{
			name:    "invalid tag too long",
			tags:    []string{string(make([]byte, 101))}, // 101 characters
			wantErr: true,
		},
		{
			name:    "too many tags",
			tags:    make([]string, 11), // 11 tags
			wantErr: true,
		},
		{
			name:    "tag with invalid characters",
			tags:    []string{"tag<script>"},
			wantErr: true,
		},
		{
			name:    "empty tag",
			tags:    []string{""},
			wantErr: true,
		},
		{
			name:    "tag with only spaces",
			tags:    []string{"   "},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilterTags(tt.tags)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFilterTags(%v) error = %v, wantErr %v", tt.tags, err, tt.wantErr)
			}
		})
	}
}

func BenchmarkMakeSecureSearchFilter(b *testing.B) {
	tags := []string{"technology", "programming", "web-development", "data-science", "machine-learning"}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MakeSecureSearchFilter(tags)
	}
}

func BenchmarkEscapeMeilisearchValue(b *testing.B) {
	value := "tech\"nology\\with\\\"special\\characters"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EscapeMeilisearchValue(value)
	}
}

// Security-focused tests
func TestSecurityVulnerabilityPrevention(t *testing.T) {
	securityTests := []struct {
		name        string
		maliciousTag string
		description string
	}{
		{
			name:        "XSS attempt",
			maliciousTag: "<script>alert('xss')</script>",
			description: "Should escape HTML/JS injection attempts",
		},
		{
			name:        "SQL injection attempt",
			maliciousTag: "'; DROP TABLE articles; --",
			description: "Should escape SQL injection attempts",
		},
		{
			name:        "Meilisearch filter bypass",
			maliciousTag: "tag\" OR \"admin",
			description: "Should escape Meilisearch filter injection",
		},
		{
			name:        "Null byte injection",
			maliciousTag: "tag\x00malicious",
			description: "Should handle null byte injection",
		},
		{
			name:        "Unicode control characters",
			maliciousTag: "tag\u0000\u0001\u0002malicious",
			description: "Should handle Unicode control characters",
		},
	}

	for _, tt := range securityTests {
		t.Run(tt.name, func(t *testing.T) {
			// Test validation rejects dangerous input
			err := ValidateFilterTags([]string{tt.maliciousTag})
			if err == nil {
				t.Errorf("ValidateFilterTags should reject malicious input: %s", tt.description)
			}
		})
	}
}