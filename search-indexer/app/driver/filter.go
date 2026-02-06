package driver

import (
	"fmt"
	"strings"
)

// escapeMeilisearchValue escapes special characters in Meilisearch filter values.
func escapeMeilisearchValue(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return value
}

// makeSecureSearchFilter creates a secure Meilisearch filter from tags.
func makeSecureSearchFilter(tags []string) string {
	if len(tags) == 0 {
		return ""
	}

	var filters []string
	for _, tag := range tags {
		escapedTag := escapeMeilisearchValue(tag)
		filters = append(filters, fmt.Sprintf("tags = \"%s\"", escapedTag))
	}

	return strings.Join(filters, " AND ")
}

// BuildUserFilter creates a secure Meilisearch filter for a user ID.
func BuildUserFilter(userID string) string {
	return fmt.Sprintf("user_id = \"%s\"", escapeMeilisearchValue(userID))
}
