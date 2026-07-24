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

// SSRF finding [2]: isPrivateNetwork must not silently allow domain-name
// hosts that cannot be resolved. A hostname whose DNS resolution fails must
// fail closed (treated as private/blocked), matching the fail-closed
// contract already documented on IsPrivateHost in ssrf_validator.go.
// Before the fix, isPrivateNetwork only inspected IP literals and the
// ".local"/".localhost" suffix, so any other domain name (including one an
// attacker fully controls, e.g. pointing its A record at 169.254.169.254)
// was reported as "not private" without ever being resolved.
func TestURLSecurityValidator_IsAllowedDomain_BlocksUnresolvableDomain(t *testing.T) {
	v := NewURLSecurityValidator()
	// RFC 2606 reserved TLD: guaranteed to never resolve.
	allowed := v.IsAllowedDomain("definitely-does-not-exist-ssrf-probe.invalid")
	require.False(t, allowed, "unresolvable domain must fail closed (blocked), not be treated as public")
}

func TestURLSecurityValidator_ValidateRSSURL_BlocksUnresolvableDomain(t *testing.T) {
	v := NewURLSecurityValidator()
	err := v.ValidateRSSURL("http://definitely-does-not-exist-ssrf-probe.invalid/feed.xml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "private network access denied")
}

// M-004: metadata host detection must match exactly, not by substring.
// `not-metadata.example.com` is a perfectly valid hostname and must not be
// blocked by the metadata check, while `metadata.google.internal` (an actual
// metadata endpoint) must still be blocked.
//
// Asserted directly against the metadataHosts allow-list (rather than via
// ValidateRSSURL end-to-end) because ValidateRSSURL now also performs real
// DNS resolution as part of the SSRF domain-name fix (finding [2]), and
// "not-metadata.example.com" is a synthetic subdomain that does not actually
// resolve — that would make this substring-matching regression test flaky on
// an unrelated concern (DNS resolvability of a made-up hostname).
func TestURLSecurityValidator_MetadataMatchesExactly(t *testing.T) {
	_, isMetadata := metadataHosts["not-metadata.example.com"]
	require.False(t, isMetadata, "unrelated hostname containing 'metadata' must NOT match the metadata allow-list via substring")
}

func TestURLSecurityValidator_MetadataExactHostBlocked(t *testing.T) {
	v := NewURLSecurityValidator()
	err := v.ValidateRSSURL("http://169.254.169.254/latest/meta-data/")
	require.Error(t, err, "AWS/GCP metadata IP must be blocked (handled via private/link-local)")
}
