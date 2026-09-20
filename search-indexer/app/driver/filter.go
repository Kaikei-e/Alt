package driver

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// escapeMeilisearchValue escapes special characters in Meilisearch filter values.
func escapeMeilisearchValue(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return value
}

// BuildUserFilter creates a secure Meilisearch filter for a user ID.
// userID must not be empty or whitespace.
func BuildUserFilter(userID string) string {
	if strings.TrimSpace(userID) == "" {
		panic("driver: BuildUserFilter called with empty userID")
	}
	return fmt.Sprintf("user_id = \"%s\"", escapeMeilisearchValue(userID))
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
