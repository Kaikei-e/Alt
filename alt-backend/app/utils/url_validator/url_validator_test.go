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
