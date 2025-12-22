package utils

import (
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// Sanitizer provides HTML sanitization functionality.
type Sanitizer struct {
	policy *bluemonday.Policy
}

// NewSanitizer creates a new Sanitizer instance with configured policy.
func NewSanitizer() *Sanitizer {
	// Use UGCPolicy as base (allows p, a, strong, em, etc.)
	p := bluemonday.UGCPolicy()

	// Enforce nofollow and target=_blank on links
	p.RequireNoFollowOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)

	return &Sanitizer{
		policy: p,
	}
}

// SanitizeHTML sanitizes the given HTML string.
func (s *Sanitizer) SanitizeHTML(html string) string {
	return s.policy.Sanitize(html)
}

// SanitizeHTMLAndTrim sanitizes the HTML and trims surrounding whitespace.
func (s *Sanitizer) SanitizeHTMLAndTrim(html string) string {
	sanitized := s.SanitizeHTML(html)
	return strings.TrimSpace(sanitized)
}
