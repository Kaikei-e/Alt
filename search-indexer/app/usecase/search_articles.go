package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"search-indexer/domain"
	"search-indexer/port"
	"strings"
	"unicode"
)

type SearchArticlesUsecase struct {
	searchEngine port.SearchEngine
}

type SearchResult struct {
	Query     string
	Documents []domain.SearchDocument
	Total     int
}

func NewSearchArticlesUsecase(searchEngine port.SearchEngine) *SearchArticlesUsecase {
	return &SearchArticlesUsecase{
		searchEngine: searchEngine,
	}
}

// Security validation patterns
var (
	// XSS prevention patterns
	scriptTagPattern    = regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`)
	htmlTagPattern      = regexp.MustCompile(`(?i)<[^>]*>`)
	javascriptProtocol  = regexp.MustCompile(`(?i)javascript:`)
	eventHandlerPattern = regexp.MustCompile(`(?i)on\w+\s*=`)

	// SQL injection prevention patterns
	sqlInjectionPattern = regexp.MustCompile(`(?i)(union|select|insert|update|delete|drop|create|alter|exec|execute|\-\-|\/\*|\*\/|;|'|")`)

	// Command injection prevention patterns
	commandInjectionPattern = regexp.MustCompile(`[|;&$\x60]`)

	// Control character patterns
	controlCharPattern = regexp.MustCompile(`[\x00-\x1F\x7F]`)

	// Zero-width character patterns (U+200B, U+200C, U+200D, U+FEFF)
	zeroWidthPattern = regexp.MustCompile("[\u200B\u200C\u200D\uFEFF]")
)

// validateQuerySecurity performs comprehensive security validation on search queries
func (u *SearchArticlesUsecase) validateQuerySecurity(query string) error {
	// Check for null bytes and control characters
	if controlCharPattern.MatchString(query) {
		return errors.New("query contains invalid control characters")
	}

	// Check for zero-width characters
	if zeroWidthPattern.MatchString(query) {
		return errors.New("query contains zero-width characters")
	}

	// URL decode the query multiple times to check for encoded attacks
	decoded := query
	for i := 0; i < 3; i++ { // Decode up to 3 times to catch multiple encoding levels
		newDecoded, err := url.QueryUnescape(decoded)
		if err != nil || newDecoded == decoded {
			break // Stop if error or no more changes
		}
		decoded = newDecoded

		// Check the decoded version for attacks at each level
		if scriptTagPattern.MatchString(decoded) ||
			htmlTagPattern.MatchString(decoded) ||
			javascriptProtocol.MatchString(decoded) ||
			eventHandlerPattern.MatchString(decoded) {
			return errors.New("query contains potential XSS attack vectors")
		}

		if sqlInjectionPattern.MatchString(decoded) {
			return errors.New("query contains potential SQL injection patterns")
		}

		if commandInjectionPattern.MatchString(decoded) {
			return errors.New("query contains potential command injection patterns")
		}
	}

	// Check the original query as well
	if scriptTagPattern.MatchString(query) ||
		htmlTagPattern.MatchString(query) ||
		javascriptProtocol.MatchString(query) ||
		eventHandlerPattern.MatchString(query) {
		return errors.New("query contains potential XSS attack vectors")
	}

	if sqlInjectionPattern.MatchString(query) {
		return errors.New("query contains potential SQL injection patterns")
	}

	if commandInjectionPattern.MatchString(query) {
		return errors.New("query contains potential command injection patterns")
	}

	return nil
}

// sanitizeQuery cleans and normalizes the query string
func (u *SearchArticlesUsecase) sanitizeQuery(query string) string {
	// Remove zero-width characters
	query = zeroWidthPattern.ReplaceAllString(query, "")

	// Normalize whitespace
	query = strings.TrimSpace(query)
	query = regexp.MustCompile(`\s+`).ReplaceAllString(query, " ")

	// Remove any remaining control characters except newlines and tabs
	result := ""
	for _, r := range query {
		if unicode.IsGraphic(r) || unicode.IsSpace(r) {
			result += string(r)
		}
	}

	return result
}

func (u *SearchArticlesUsecase) Execute(ctx context.Context, query string, limit int) (*SearchResult, error) {
	if query == "" {
		return nil, errors.New("query cannot be empty")
	}

	if limit <= 0 {
		return nil, errors.New("limit must be greater than 0")
	}

	if len(query) > 1000 {
		return nil, errors.New("query too long")
	}

	if limit > 1000 {
		return nil, errors.New("limit too large")
	}

	// Perform security validation
	if err := u.validateQuerySecurity(query); err != nil {
		return nil, fmt.Errorf("security validation failed: %w", err)
	}

	// Sanitize the query
	sanitizedQuery := u.sanitizeQuery(query)

	// Final check after sanitization
	if sanitizedQuery == "" {
		return nil, errors.New("query became empty after sanitization")
	}

	documents, err := u.searchEngine.Search(ctx, sanitizedQuery, limit)
	if err != nil {
		return nil, err
	}

	return &SearchResult{
		Query:     sanitizedQuery,
		Documents: documents,
		Total:     len(documents),
	}, nil
}
