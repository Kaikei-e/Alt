package config

import "testing"

// TestParseAllowedDomains_DefaultValueIsAnchored guards against the
// regression where pre-escaped patterns (like the default ALLOWED_DOMAINS
// value, e.g. "zenn\.dev") skipped the anchoring branch and compiled
// unanchored, letting "zenn.dev.evil.com" match as a substring.
func TestParseAllowedDomains_DefaultValueIsAnchored(t *testing.T) {
	cfg := &ProxyConfig{}
	if err := cfg.parseAllowedDomains(); err != nil {
		t.Fatalf("parseAllowedDomains() error = %v", err)
	}

	cases := []struct {
		domain string
		want   bool
	}{
		{"zenn.dev", true},
		{"github.com", true},
		{"feeds.bbci.co.uk", true},
		// Exact-match allowlist: attacker-controlled suffix/prefix around an
		// allowed domain must never match.
		{"zenn.dev.evil.com", false},
		{"evil-zenn.dev", false},
		{"evilgithub.com", false},
		{"github.com.attacker.net", false},
		// No implicit subdomain wildcarding: each hostname must be listed
		// explicitly (matches the existing "feeds.bbci.co.uk" style entries).
		{"sub.zenn.dev", false},
		{"api.github.com", false},
	}

	for _, tc := range cases {
		if got := cfg.IsDomainAllowed(tc.domain); got != tc.want {
			t.Errorf("IsDomainAllowed(%q) = %v, want %v", tc.domain, got, tc.want)
		}
	}
}

// TestParseAllowedDomains_UnescapedInputIsAnchored covers the branch where
// ALLOWED_DOMAINS is supplied as plain (non pre-escaped) literals, which
// must also be fully anchored.
func TestParseAllowedDomains_UnescapedInputIsAnchored(t *testing.T) {
	t.Setenv("ALLOWED_DOMAINS", "example.com,news.example.org")

	cfg := &ProxyConfig{}
	if err := cfg.parseAllowedDomains(); err != nil {
		t.Fatalf("parseAllowedDomains() error = %v", err)
	}

	cases := []struct {
		domain string
		want   bool
	}{
		{"example.com", true},
		{"news.example.org", true},
		{"example.com.evil.com", false},
		{"evil-example.com", false},
		{"sub.example.com", false},
	}

	for _, tc := range cases {
		if got := cfg.IsDomainAllowed(tc.domain); got != tc.want {
			t.Errorf("IsDomainAllowed(%q) = %v, want %v", tc.domain, got, tc.want)
		}
	}
}
