// Package utils provides security utilities for search query sanitization and validation.
// This package implements comprehensive protection against query injection attacks,
// cross-site scripting (XSS), and other security vulnerabilities in search functionality.
package utils

import (
	"context"
	"net/url"
	"regexp"
	"strings"
)

// SecurityConfig holds security-related configuration for query sanitization.
// It defines the security policies and constraints to be applied during query processing.
type SecurityConfig struct {
	// MaxQueryLength defines the maximum allowed length for search queries
	MaxQueryLength int
	
	// DisallowedPatterns contains regex patterns that are not allowed in queries
	DisallowedPatterns []string
	
	// AllowedSpecialChars contains special characters that are permitted in queries
	AllowedSpecialChars []string
	
	// StripHTMLTags enables removal of HTML tags from queries
	StripHTMLTags bool
	
	// NormalizeWhitespace enables whitespace normalization (tabs, newlines, excessive spaces)
	NormalizeWhitespace bool
}

const (
	// DefaultMaxQueryLength is the default maximum query length
	DefaultMaxQueryLength = 1000
)

// DefaultSecurityConfig returns a secure default configuration for query sanitization.
// This configuration provides comprehensive protection against common attack vectors
// while allowing legitimate search functionality.
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		MaxQueryLength:      DefaultMaxQueryLength,
		DisallowedPatterns:  []string{},
		AllowedSpecialChars: []string{"-", "_", ".", "!", "?", "&", "+", "@", "#"},
		StripHTMLTags:       true,
		NormalizeWhitespace: true,
	}
}

// QuerySanitizer provides comprehensive sanitization and validation of search queries.
// It protects against various attack vectors including XSS, script injection,
// SQL injection patterns, and other security vulnerabilities.
type QuerySanitizer struct {
	config *SecurityConfig
}

// Common dangerous characters that may be used in injection attacks
var dangerousChars = []string{"<", ">", "'", "\"", ";", "\\", "/", "*"}

// NewQuerySanitizer creates a new query sanitizer
func NewQuerySanitizer(config *SecurityConfig) *QuerySanitizer {
	if config == nil {
		config = DefaultSecurityConfig()
	}
	return &QuerySanitizer{config: config}
}

// NewQuerySanitizerWithCustomConfig creates a new query sanitizer with custom configuration
func NewQuerySanitizerWithCustomConfig(maxLength int, allowedChars []string, disallowedPatterns []string) *QuerySanitizer {
	config := &SecurityConfig{
		MaxQueryLength:         maxLength,
		DisallowedPatterns:     disallowedPatterns,
		AllowedSpecialChars:    allowedChars,
		StripHTMLTags:         true,
		NormalizeWhitespace:   true,
	}
	return &QuerySanitizer{config: config}
}

// SanitizeQuery sanitizes a search query to prevent injection attacks.
// It performs the following operations:
// 1. URL decodes the query to handle encoded attack vectors
// 2. Removes zero-width characters
// 3. Strips HTML tags (if configured)
// 4. Removes script content and dangerous protocols
// 5. Checks for disallowed patterns
// 6. Normalizes whitespace (if configured)
func (s *QuerySanitizer) SanitizeQuery(ctx context.Context, query string) (string, error) {
	if query == "" {
		return "", nil
	}

	// URL decode the query to handle encoded attack vectors
	if decoded, err := url.QueryUnescape(query); err == nil {
		query = decoded
	}

	// Remove zero-width characters
	query = s.removeZeroWidthChars(query)

	// Remove HTML tags if configured
	if s.config.StripHTMLTags {
		query = s.stripHTMLTags(query)
	}

	// Remove script content
	query = s.removeScriptContent(query)

	// Check for disallowed patterns
	for _, pattern := range s.config.DisallowedPatterns {
		if matched, _ := regexp.MatchString(pattern, strings.ToLower(query)); matched {
			return "", &SecurityError{
				Type:    "disallowed_pattern",
				Message: "Query contains disallowed pattern",
				Query:   query,
			}
		}
	}

	// Normalize whitespace if configured
	if s.config.NormalizeWhitespace {
		query = s.normalizeWhitespace(query)
	}

	return query, nil
}

// stripHTMLTags removes HTML tags from the query
func (s *QuerySanitizer) stripHTMLTags(input string) string {
	// Remove script tags and their content
	for {
		start := strings.Index(strings.ToLower(input), "<script")
		if start == -1 {
			break
		}
		end := strings.Index(strings.ToLower(input[start:]), "</script>")
		if end == -1 {
			// No closing tag, remove from start to end
			input = input[:start]
			break
		}
		end += start + len("</script>")
		input = input[:start] + input[end:]
	}

	// Remove any remaining HTML tags
	for {
		start := strings.Index(input, "<")
		if start == -1 {
			break
		}
		end := strings.Index(input[start:], ">")
		if end == -1 {
			// No closing bracket, remove from start to end
			input = input[:start]
			break
		}
		end += start + 1
		input = input[:start] + input[end:]
	}

	return input
}

// removeScriptContent removes script content from the query
func (s *QuerySanitizer) removeScriptContent(input string) string {
	// Remove common script patterns
	patterns := []string{
		"javascript:",
		"data:",
		"vbscript:",
		"onload=",
		"onerror=",
		"onclick=",
		"onmouseover=",
	}

	for _, pattern := range patterns {
		input = strings.ReplaceAll(strings.ToLower(input), pattern, "")
	}

	return input
}

// normalizeWhitespace normalizes whitespace in the query
func (s *QuerySanitizer) normalizeWhitespace(input string) string {
	// Replace tabs and newlines with spaces
	input = strings.ReplaceAll(input, "\t", " ")
	input = strings.ReplaceAll(input, "\r", " ")
	input = strings.ReplaceAll(input, "\n", " ")
	
	// Remove excessive whitespace
	input = strings.TrimSpace(input)
	return strings.Join(strings.Fields(input), " ")
}

// removeZeroWidthChars removes zero-width characters from the query
func (s *QuerySanitizer) removeZeroWidthChars(input string) string {
	// Remove common zero-width characters
	zeroWidthChars := []rune{
		'\u200B', // Zero width space
		'\u200C', // Zero width non-joiner
		'\u200D', // Zero width joiner
		'\uFEFF', // Zero width no-break space (BOM)
		'\u200E', // Left-to-right mark
		'\u200F', // Right-to-left mark
	}
	
	for _, char := range zeroWidthChars {
		input = strings.ReplaceAll(input, string(char), "")
	}
	
	return input
}

// ValidateQuery validates a search query for security concerns.
// It checks for:
// 1. Query length limits
// 2. Null bytes and control characters
// 3. Potentially dangerous characters
// This method should be called before sanitization to catch malicious input early.
func (s *QuerySanitizer) ValidateQuery(ctx context.Context, query string) error {
	if len(query) > s.config.MaxQueryLength {
		return &SecurityError{
			Type:    "query_too_long",
			Message: "Query exceeds maximum length",
			Query:   query,
		}
	}

	// Check for null bytes and control characters
	for i, r := range query {
		if r == 0 || (r < 32 && r != 9 && r != 10 && r != 13) {
			return &SecurityError{
				Type:    "dangerous_character",
				Message: "Query contains null byte or control character",
				Query:   query,
			}
		}
		_ = i // Avoid unused variable warning
	}

	// Check for potentially dangerous characters
	for _, char := range dangerousChars {
		if strings.Contains(query, char) {
			allowed := false
			for _, allowedChar := range s.config.AllowedSpecialChars {
				if char == allowedChar {
					allowed = true
					break
				}
			}
			if !allowed {
				return &SecurityError{
					Type:    "dangerous_character",
					Message: "Query contains potentially dangerous character: " + char,
					Query:   query,
				}
			}
		}
	}

	return nil
}

// SecurityError represents a security-related error
type SecurityError struct {
	Type    string
	Message string
	Query   string
}

func (e *SecurityError) Error() string {
	return e.Message
}