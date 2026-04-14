package security

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// M-002: URLSecurityValidator must offer an HTTPS-only mode for callers like
// the image proxy and RAG fetcher where plaintext HTTP is unacceptable.
func TestURLSecurityValidator_HTTPSOnly_RejectsHTTP(t *testing.T) {
	v := NewURLSecurityValidator()
	v.RequireHTTPS(true)
	err := v.ValidateRSSURL("http://example.com/feed.xml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "HTTPS")
}

func TestURLSecurityValidator_HTTPSOnly_AllowsHTTPS(t *testing.T) {
	v := NewURLSecurityValidator()
	v.RequireHTTPS(true)
	err := v.ValidateRSSURL("https://example.com/feed.xml")
	require.NoError(t, err)
}

// Regression: by default HTTP is still allowed (RSS feed registration).
func TestURLSecurityValidator_DefaultAllowsHTTP(t *testing.T) {
	v := NewURLSecurityValidator()
	err := v.ValidateRSSURL("http://example.com/feed.xml")
	require.NoError(t, err)
}

// M-003: NewSSRFValidator must NOT seed the allow-list with a placeholder
// "example.com". Caller code that wanted an allow-list opted in explicitly,
// and a misleading default risks giving a false sense of safety.
func TestNewSSRFValidator_NoPlaceholderAllowedDomains(t *testing.T) {
	v := NewSSRFValidator()
	require.Empty(t, v.allowedDomains, "allowedDomains must start empty; callers add entries explicitly")
}

// M-004: metadata host detection must match exactly, not by substring.
// `not-metadata.example.com` is a perfectly valid hostname and must not be
// blocked, while `metadata.google.internal` (an actual metadata endpoint)
// must still be blocked.
func TestURLSecurityValidator_MetadataMatchesExactly(t *testing.T) {
	v := NewURLSecurityValidator()
	require.NoError(t, v.ValidateRSSURL("https://not-metadata.example.com/rss"),
		"unrelated hostname containing 'metadata' must NOT be blocked")
}

func TestURLSecurityValidator_MetadataExactHostBlocked(t *testing.T) {
	v := NewURLSecurityValidator()
	err := v.ValidateRSSURL("http://169.254.169.254/latest/meta-data/")
	require.Error(t, err, "AWS/GCP metadata IP must be blocked (handled via private/link-local)")
}
