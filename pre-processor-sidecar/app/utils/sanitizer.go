package utils

import (
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// Sanitizer provides HTML sanitization functionality
type Sanitizer struct {
	policy *bluemonday.Policy
}

// NewSanitizer creates a new sanitizer with a configured policy
func NewSanitizer() *Sanitizer {
	// Use bluemonday's UGCPolicy as a base, which allows a broad range of standard HTML tags
	// (p, b, i, a, img, etc.) while stripping dangerous content (script, iframe, object, etc.)
	// We can customize this policy further if needed.
	policy := bluemonday.UGCPolicy()

	// Ensure links have rel="nofollow" to prevent spam/SEO manipulation
	policy.RequireNoFollowOnLinks(true)

	// Open links in new window/tab
	policy.AddTargetBlankToFullyQualifiedLinks(true)

	return &Sanitizer{
		policy: policy,
	}
}

// SanitizeHTML sanitizes the given HTML content string
func (s *Sanitizer) SanitizeHTML(content string) string {
	if content == "" {
		return ""
	}
	return s.policy.Sanitize(content)
}

// SanitizeAndTrim processes content by sanitizing HTML and then trimming whitespace
func (s *Sanitizer) SanitizeAndTrim(content string) string {
	sanitized := s.SanitizeHTML(content)
	return strings.TrimSpace(sanitized)
}
