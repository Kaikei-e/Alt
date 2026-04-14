// Package sanitize provides a centralized HTML sanitization utility.
// The bluemonday policy is created once at package init and is goroutine-safe.
package sanitize

import (
	"html"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

var (
	strictPolicy    = bluemonday.StrictPolicy()
	spaceCollapseRe = regexp.MustCompile(`\s+`)
)

// SanitizeDescription removes HTML tags, decodes HTML entities, and returns plain text.
func SanitizeDescription(rawHTML string) string {
	if rawHTML == "" {
		return ""
	}

	text := strictPolicy.Sanitize(rawHTML)
	text = html.UnescapeString(text)
	text = strings.TrimSpace(text)
	text = spaceCollapseRe.ReplaceAllString(text, " ")
	return text
}
