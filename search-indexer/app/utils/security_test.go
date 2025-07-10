package utils

import (
	"context"
	"testing"
)

func TestQuerySanitizer_SanitizeQuery(t *testing.T) {
	sanitizer := NewQuerySanitizer(DefaultSecurityConfig())
	ctx := context.Background()

	tests := []struct {
		name        string
		input       string
		expected    string
		shouldError bool
	}{
		{
			name:        "clean query",
			input:       "golang programming",
			expected:    "golang programming",
			shouldError: false,
		},
		{
			name:        "query with allowed special chars",
			input:       "go-lang & programming!",
			expected:    "go-lang & programming!",
			shouldError: false,
		},
		{
			name:        "query with HTML tags",
			input:       "<b>bold</b> text",
			expected:    "bold text",
			shouldError: false,
		},
		{
			name:        "query with script tags",
			input:       "<script>alert('xss')</script>search term",
			expected:    "search term",
			shouldError: false,
		},
		{
			name:        "query with javascript protocol",
			input:       "javascript:alert('xss') search term",
			expected:    " search term",
			shouldError: false,
		},
		{
			name:        "query with data protocol",
			input:       "data:text/html,<script>alert('xss')</script>",
			expected:    "text/html,",
			shouldError: false,
		},
		{
			name:        "query with event handlers",
			input:       "onload=alert('xss') search term",
			expected:    " search term",
			shouldError: false,
		},
		{
			name:        "query with multiple whitespace",
			input:       "   multiple    spaces   ",
			expected:    "multiple spaces",
			shouldError: false,
		},
		{
			name:        "empty query",
			input:       "",
			expected:    "",
			shouldError: false,
		},
		{
			name:        "query with unclosed script tag",
			input:       "<script>malicious code search term",
			expected:    "",
			shouldError: false,
		},
		{
			name:        "query with unclosed HTML tag",
			input:       "<div>content search term",
			expected:    "",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sanitizer.SanitizeQuery(ctx, tt.input)

			if tt.shouldError && err == nil {
				t.Errorf("SanitizeQuery() expected error but got none")
				return
			}

			if !tt.shouldError && err != nil {
				t.Errorf("SanitizeQuery() unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("SanitizeQuery() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestQuerySanitizer_ValidateQuery(t *testing.T) {
	sanitizer := NewQuerySanitizer(DefaultSecurityConfig())
	ctx := context.Background()

	tests := []struct {
		name        string
		input       string
		shouldError bool
		errorType   string
	}{
		{
			name:        "valid query",
			input:       "golang programming",
			shouldError: false,
		},
		{
			name:        "query with allowed special chars",
			input:       "go-lang & programming!",
			shouldError: false,
		},
		{
			name:        "query too long",
			input:       string(make([]byte, 1001)),
			shouldError: true,
			errorType:   "query_too_long",
		},
		{
			name:        "query with dangerous characters",
			input:       "search<script>alert('xss')</script>",
			shouldError: true,
			errorType:   "dangerous_character",
		},
		{
			name:        "query with single quotes",
			input:       "user's guide",
			shouldError: true,
			errorType:   "dangerous_character",
		},
		{
			name:        "query with double quotes",
			input:       "\"quoted text\"",
			shouldError: true,
			errorType:   "dangerous_character",
		},
		{
			name:        "query with semicolon",
			input:       "search; DROP TABLE",
			shouldError: true,
			errorType:   "dangerous_character",
		},
		{
			name:        "query with backslash",
			input:       "search\\term",
			shouldError: true,
			errorType:   "dangerous_character",
		},
		{
			name:        "query with forward slash",
			input:       "search/term",
			shouldError: true,
			errorType:   "dangerous_character",
		},
		{
			name:        "query with asterisk",
			input:       "search*wildcard",
			shouldError: true,
			errorType:   "dangerous_character",
		},
		{
			name:        "query at maximum length",
			input:       string(make([]byte, 1000)),
			shouldError: false,
		},
		{
			name:        "empty query",
			input:       "",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sanitizer.ValidateQuery(ctx, tt.input)

			if tt.shouldError && err == nil {
				t.Errorf("ValidateQuery() expected error but got none")
				return
			}

			if !tt.shouldError && err != nil {
				t.Errorf("ValidateQuery() unexpected error: %v", err)
				return
			}

			if tt.shouldError && err != nil {
				if secErr, ok := err.(*SecurityError); ok {
					if secErr.Type != tt.errorType {
						t.Errorf("ValidateQuery() error type = %s, want %s", secErr.Type, tt.errorType)
					}
				} else {
					t.Errorf("ValidateQuery() error is not a SecurityError: %v", err)
				}
			}
		})
	}
}

func TestQuerySanitizer_WithCustomConfig(t *testing.T) {
	config := &SecurityConfig{
		MaxQueryLength:      500,
		DisallowedPatterns:  []string{`\bdrop\b`, `\bdelete\b`},
		AllowedSpecialChars: []string{"-", "_"},
		StripHTMLTags:       true,
		NormalizeWhitespace: true,
	}

	sanitizer := NewQuerySanitizer(config)
	ctx := context.Background()

	tests := []struct {
		name        string
		input       string
		shouldError bool
		errorType   string
	}{
		{
			name:        "query with disallowed pattern",
			input:       "search drop table",
			shouldError: true,
			errorType:   "disallowed_pattern",
		},
		{
			name:        "query with delete pattern",
			input:       "delete from users",
			shouldError: true,
			errorType:   "disallowed_pattern",
		},
		{
			name:        "query exceeding custom limit",
			input:       string(make([]byte, 501)),
			shouldError: true,
			errorType:   "query_too_long",
		},
		{
			name:        "query with non-allowed special char",
			input:       "search@term",
			shouldError: true,
			errorType:   "dangerous_character",
		},
		{
			name:        "query with allowed special chars",
			input:       "go-lang_term",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test validation
			err := sanitizer.ValidateQuery(ctx, tt.input)

			if tt.shouldError && err == nil {
				t.Errorf("ValidateQuery() expected error but got none")
				return
			}

			if !tt.shouldError && err != nil {
				t.Errorf("ValidateQuery() unexpected error: %v", err)
				return
			}

			// Test sanitization for disallowed patterns
			if tt.errorType == "disallowed_pattern" {
				_, err := sanitizer.SanitizeQuery(ctx, tt.input)
				if err == nil {
					t.Errorf("SanitizeQuery() expected error for disallowed pattern but got none")
				}
			}
		})
	}
}

func TestDefaultSecurityConfig(t *testing.T) {
	config := DefaultSecurityConfig()

	if config.MaxQueryLength != 1000 {
		t.Errorf("Default MaxQueryLength = %d, want 1000", config.MaxQueryLength)
	}

	if !config.StripHTMLTags {
		t.Errorf("Default StripHTMLTags = %v, want true", config.StripHTMLTags)
	}

	if !config.NormalizeWhitespace {
		t.Errorf("Default NormalizeWhitespace = %v, want true", config.NormalizeWhitespace)
	}

	expectedAllowedChars := []string{"-", "_", ".", "!", "?", "&", "+", "@", "#"}
	if len(config.AllowedSpecialChars) != len(expectedAllowedChars) {
		t.Errorf("Default AllowedSpecialChars length = %d, want %d", len(config.AllowedSpecialChars), len(expectedAllowedChars))
	}

	for i, char := range expectedAllowedChars {
		if i >= len(config.AllowedSpecialChars) || config.AllowedSpecialChars[i] != char {
			t.Errorf("Default AllowedSpecialChars[%d] = %s, want %s", i, config.AllowedSpecialChars[i], char)
		}
	}
}

func TestSecurityError_Error(t *testing.T) {
	err := &SecurityError{
		Type:    "test_error",
		Message: "Test error message",
		Query:   "test query",
	}

	if err.Error() != "Test error message" {
		t.Errorf("SecurityError.Error() = %s, want %s", err.Error(), "Test error message")
	}
}