package url_validator

import (
	"net/url"
	"testing"
)

func TestIsAllowedURL_AllowsConfiguredFeedHosts(t *testing.T) {
	t.Setenv("FEED_ALLOWED_HOSTS", "mock-rss-001,mock-rss-002")

	u, err := url.Parse("http://mock-rss-001/feed.xml")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	if err := IsAllowedURL(u); err != nil {
		t.Fatalf("expected configured feed host to be allowed, got error: %v", err)
	}
}

// SSRF finding [2]: IsAllowedURL resolved the hostname but only rejected
// loopback/private IPs, missing link-local (169.254.0.0/16 — cloud metadata)
// addresses. A feed host whose A record points at 169.254.169.254 must be
// rejected the same way a 127.0.0.1/10.0.0.0 host is.
func TestIsAllowedURL_BlocksLinkLocalIPLiteral(t *testing.T) {
	u, err := url.Parse("http://169.254.169.254/feed.xml")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	if err := IsAllowedURL(u); err == nil {
		t.Fatalf("expected link-local metadata IP to be rejected, got nil error")
	}
}

func TestIsAllowedURL_RejectsDisallowedSchemes(t *testing.T) {
	for _, raw := range []string{"ftp://example.com/feed.xml", "file:///etc/passwd", "gopher://example.com"} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if err := IsAllowedURL(u); err == nil {
			t.Errorf("expected scheme %s to be rejected", u.Scheme)
		}
	}
}

func TestIsAllowedURL_RejectsUserinfo(t *testing.T) {
	u, err := url.Parse("http://admin:secret@example.com/feed.xml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := IsAllowedURL(u); err == nil {
		t.Error("expected URL with userinfo to be rejected")
	}
}

func TestIsAllowedURL_PortRestrictions(t *testing.T) {
	// Disallowed ports
	for _, raw := range []string{
		"http://example.com:8080/feed.xml",
		"http://example.com:22/feed.xml",
		"https://example.com:8443/feed.xml",
	} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if err := IsAllowedURL(u); err == nil {
			t.Errorf("expected URL with port %s to be rejected", u.Port())
		}
	}

	// Allowed ports (empty, 80, 443) for public domain (mock via allowlist to avoid external DNS)
	t.Setenv("FEED_ALLOWED_HOSTS", "example.com")
	for _, raw := range []string{
		"http://example.com/feed.xml",
		"http://example.com:80/feed.xml",
		"https://example.com:443/feed.xml",
	} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if err := IsAllowedURL(u); err != nil {
			t.Errorf("expected URL %s to be allowed, got error: %v", raw, err)
		}
	}
}

func TestIsAllowedURL_BlocksBareIPLiterals(t *testing.T) {
	for _, raw := range []string{
		"http://0.0.0.0/feed.xml",
		"http://0.0.0.1/feed.xml",
		"http://[::]/feed.xml",
		"http://100.64.0.1/feed.xml",
		"http://100.64.0.1:80/feed.xml",
		"http://224.0.0.1/feed.xml",
		"http://240.0.0.1/feed.xml",
		"http://255.255.255.255/feed.xml",
		"http://198.18.0.1/feed.xml",
	} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if err := IsAllowedURL(u); err == nil {
			t.Errorf("expected bare IP literal %s to be rejected, got nil", raw)
		}
	}
}
