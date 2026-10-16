package driver

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// sanitizeFilterValue strips control characters (< 0x20, DEL, and Unicode
// control runes) and escapes backslashes, double quotes, and single quotes
// in Meilisearch filter values.
func sanitizeFilterValue(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if r < 0x20 || r == 0x7f || unicode.IsControl(r) {
			continue
		}
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\'':
			b.WriteString(`\'`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// escapeMeilisearchValue escapes special characters in Meilisearch filter values.
func escapeMeilisearchValue(value string) string {
	return sanitizeFilterValue(value)
}

// makeSecureSearchFilter creates a secure Meilisearch filter from tags,
// quoting and escaping each tag value.
func makeSecureSearchFilter(tags []string) string {
	if len(tags) == 0 {
		return ""
	}

	filters := make([]string, 0, len(tags))
	for _, tag := range tags {
		escapedTag := sanitizeFilterValue(tag)
		if strings.TrimSpace(escapedTag) == "" {
			continue
		}
		filters = append(filters, fmt.Sprintf("tags = \"%s\"", escapedTag))
	}

	return strings.Join(filters, " AND ")
}

// BuildUserFilter creates a secure Meilisearch filter for a user ID.
// userID must not be empty or whitespace.
func BuildUserFilter(userID string) string {
	sanitized := sanitizeFilterValue(userID)
	if strings.TrimSpace(sanitized) == "" {
		panic("driver: BuildUserFilter called with empty userID")
	}
	return fmt.Sprintf("user_id = \"%s\"", sanitized)
}

// BuildUserDateFilter creates a secure Meilisearch filter combining user_id
// and optional published_at lower and upper bounds.
// userID must not be empty or whitespace.
func BuildUserDateFilter(userID string, publishedAfter, publishedBefore *time.Time) string {
	userFilter := BuildUserFilter(userID)
	clauses := []string{userFilter}
	if publishedAfter != nil {
		clauses = append(clauses, "published_at >= "+strconv.FormatInt(publishedAfter.Unix(), 10))
	}
	if publishedBefore != nil {
		clauses = append(clauses, "published_at <= "+strconv.FormatInt(publishedBefore.Unix(), 10))
	}
	return strings.Join(clauses, " AND ")
}
