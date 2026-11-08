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

	// ErrInvalidSearchQuery is returned when a search request fails input validation.
	ErrInvalidSearchQuery = errors.New("invalid search query")
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
		return "", fmt.Errorf("%w: query cannot be empty", ErrInvalidSearchQuery)
	}

	if limit <= 0 {
		return "", fmt.Errorf("%w: limit must be greater than 0", ErrInvalidSearchQuery)
	}

	if len(query) > 1000 {
		return "", fmt.Errorf("%w: query too long", ErrInvalidSearchQuery)
	}

	if limit > 1000 {
		return "", fmt.Errorf("%w: limit too large", ErrInvalidSearchQuery)
	}

	// Perform security validation
	if err := validateQuerySecurity(query); err != nil {
		return "", fmt.Errorf("%w: security validation failed: %w", ErrInvalidSearchQuery, err)
	}

	// Sanitize the query
	sanitizedQuery := sanitizeQuery(query)

	// Final check after sanitization
	if sanitizedQuery == "" {
		return "", fmt.Errorf("%w: query became empty after sanitization", ErrInvalidSearchQuery)
	}

	return sanitizedQuery, nil
}
