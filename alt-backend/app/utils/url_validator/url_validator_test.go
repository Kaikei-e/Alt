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
