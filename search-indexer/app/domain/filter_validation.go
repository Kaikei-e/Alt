package domain

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// ValidateFilterTags validates input tags for security.
func ValidateFilterTags(tags []string) error {
	if len(tags) > 10 {
		return fmt.Errorf("too many filter tags: maximum 10 allowed, got %d", len(tags))
	}

	// Regex for allowed characters: alphanumeric, spaces, hyphens, underscores, and Unicode letters
	validTagRegex := regexp.MustCompile(`^[\p{L}\p{N}\s\-_]+$`)

	for _, tag := range tags {
		if strings.TrimSpace(tag) == "" {
			return fmt.Errorf("empty or whitespace-only tag not allowed")
		}

		if len(tag) > 100 {
			return fmt.Errorf("tag too long: maximum 100 characters, got %d", len(tag))
		}

		if !validTagRegex.MatchString(tag) {
			return fmt.Errorf("invalid characters in tag: %s", tag)
		}

		for _, r := range tag {
			if unicode.IsControl(r) {
				return fmt.Errorf("control characters not allowed in tag: %s", tag)
			}
		}
	}

	return nil
}
