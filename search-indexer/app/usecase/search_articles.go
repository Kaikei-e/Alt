package usecase

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Structural validation patterns. Meilisearch does not execute SQL, shell
// commands, or render HTML, so SQLi/XSS/cmd denylists only generate false
// positives against legitimate searches. We keep validation limited to
// characters that break downstream parsers (control chars, zero-width
// invisibles) and defer filter-value escaping to driver/filter.go.
var (
	controlCharPattern = regexp.MustCompile(`[\x00-\x1F\x7F]`)
	zeroWidthPattern   = regexp.MustCompile("[\u200B\u200C\u200D\uFEFF]")
	whitespaceRun      = regexp.MustCompile(`\s+`)
)

// validateQuerySecurity rejects queries whose raw bytes would confuse parsers
// or log sinks downstream. Application-level injection concerns are handled
// at the persistence boundary (Meilisearch filter escaping).
func validateQuerySecurity(query string) error {
	if controlCharPattern.MatchString(query) {
		return errors.New("query contains invalid control characters")
	}
	if zeroWidthPattern.MatchString(query) {
		return errors.New("query contains zero-width characters")
	}
	return nil
}

// sanitizeQuery applies Unicode NFC normalization and whitespace folding so
// equivalent Unicode representations produce the same Meilisearch query.
func sanitizeQuery(query string) string {
	query = norm.NFC.String(query)
	query = zeroWidthPattern.ReplaceAllString(query, "")
	query = strings.TrimSpace(query)
	query = whitespaceRun.ReplaceAllString(query, " ")

	var b strings.Builder
	b.Grow(len(query))
	for _, r := range query {
		if unicode.IsGraphic(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// validateAndSanitizeQuery runs the length/control-character/zero-width
// checks and NFC sanitization shared by every search entry point so which
// operation a request happens to go through never changes what input is accepted.
func validateAndSanitizeQuery(query string, limit int) (string, error) {
	if query == "" {
		return "", errors.New("query cannot be empty")
	}

	if limit <= 0 {
		return "", errors.New("limit must be greater than 0")
	}

	if len(query) > 1000 {
		return "", errors.New("query too long")
	}

	if limit > 1000 {
		return "", errors.New("limit too large")
	}

	// Perform security validation
	if err := validateQuerySecurity(query); err != nil {
		return "", fmt.Errorf("security validation failed: %w", err)
	}

	// Sanitize the query
	sanitizedQuery := sanitizeQuery(query)

	// Final check after sanitization
	if sanitizedQuery == "" {
		return "", errors.New("query became empty after sanitization")
	}

	return sanitizedQuery, nil
}
