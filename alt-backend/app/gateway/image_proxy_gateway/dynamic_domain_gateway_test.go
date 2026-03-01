package image_proxy_gateway

import (
	"alt/domain"
	"context"
	"testing"
)

// mockDomainLister implements the domain listing interface for tests.
type mockDomainLister struct {
	domains []domain.FeedLinkDomain
	err     error
}

func (m *mockDomainLister) ListFeedLinkDomains(ctx context.Context) ([]domain.FeedLinkDomain, error) {
	return m.domains, m.err
}

func TestDynamicDomainGateway_StaticCDNDomains(t *testing.T) {
	gw := NewDynamicDomainGateway(&mockDomainLister{})

	tests := []struct {
		hostname string
		allowed  bool
	}{
		{"img.youtube.com", true},
		{"i.imgur.com", true},
		{"pbs.twimg.com", true},
		{"images.unsplash.com", true},
		{"cdn-images-1.medium.com", true},
		{"miro.medium.com", true},
		{"d1234.cloudfront.net", true},
		{"res.cloudinary.com", true},
		{"example.imgix.net", true},
		{"user-content.githubusercontent.com", true},
		{"storage.googleapis.com", true},
		{"evil.example.com", false},
		{"localhost", false},
	}

	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			allowed, err := gw.IsAllowedImageDomain(context.Background(), tt.hostname)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if allowed != tt.allowed {
				t.Errorf("IsAllowedImageDomain(%q) = %v, want %v", tt.hostname, allowed, tt.allowed)
			}
		})
	}
}

func TestDynamicDomainGateway_SubscriptionDomains(t *testing.T) {
	gw := NewDynamicDomainGateway(&mockDomainLister{
		domains: []domain.FeedLinkDomain{
			{Domain: "blog.example.com", Scheme: "https"},
			{Domain: "news.example.org", Scheme: "https"},
		},
	})

	tests := []struct {
		hostname string
		allowed  bool
	}{
		{"blog.example.com", true},
		{"news.example.org", true},
		// Sibling subdomains are allowed (share parent example.com)
		{"other.example.com", true},
		// Completely unrelated domains are not
		{"evil.unrelated.net", false},
	}

	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			allowed, err := gw.IsAllowedImageDomain(context.Background(), tt.hostname)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if allowed != tt.allowed {
				t.Errorf("IsAllowedImageDomain(%q) = %v, want %v", tt.hostname, allowed, tt.allowed)
			}
		})
	}
}

func TestDynamicDomainGateway_OGPImageDomains(t *testing.T) {
	// OGP image domains differ from feed subscription domains.
	// The domain lister now returns both feed_links and article_heads.og_image_url domains.
	gw := NewDynamicDomainGateway(&mockDomainLister{
		domains: []domain.FeedLinkDomain{
			{Domain: "feed.example.com", Scheme: "https"},     // feed domain
			{Domain: "images.example.com", Scheme: "https"},   // OGP image domain (different from feed)
			{Domain: "rss.example.org", Scheme: "https"},      // feed domain
			{Domain: "cdn.example.org", Scheme: "https"},      // OGP image domain (different from feed)
			{Domain: "ogp.example.net", Scheme: "https"},      // OGP image domain only
		},
	})

	tests := []struct {
		hostname string
		allowed  bool
	}{
		{"feed.example.com", true},
		{"images.example.com", true},
		{"rss.example.org", true},
		{"cdn.example.org", true},
		{"ogp.example.net", true},
		{"evil.attacker.example.invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			allowed, err := gw.IsAllowedImageDomain(context.Background(), tt.hostname)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if allowed != tt.allowed {
				t.Errorf("IsAllowedImageDomain(%q) = %v, want %v", tt.hostname, allowed, tt.allowed)
			}
		})
	}
}

func TestDynamicDomainGateway_SubdomainMatching(t *testing.T) {
	// Image CDN subdomains (e.g. media2.dev.to, cdn.hackernoon.com)
	// should be allowed when their parent domain (dev.to, hackernoon.com)
	// is in the subscription list.
	gw := NewDynamicDomainGateway(&mockDomainLister{
		domains: []domain.FeedLinkDomain{
			{Domain: "dev.to", Scheme: "https"},
			{Domain: "hackernoon.com", Scheme: "https"},
			{Domain: "www.wired.com", Scheme: "https"},
			{Domain: "feeds.bbci.co.uk", Scheme: "https"},
		},
	})

	tests := []struct {
		hostname string
		allowed  bool
	}{
		// Exact match
		{"dev.to", true},
		{"hackernoon.com", true},
		// Subdomains of subscription domains
		{"media2.dev.to", true},
		{"cdn.hackernoon.com", true},
		{"media.wired.com", true},
		{"ichef.bbci.co.uk", true},
		// Not a subdomain of any subscription domain
		{"evil.example.com", false},
		{"dev.to.evil.com", false},
		// Bare TLD should not match
		{"com", false},
	}

	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			allowed, err := gw.IsAllowedImageDomain(context.Background(), tt.hostname)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if allowed != tt.allowed {
				t.Errorf("IsAllowedImageDomain(%q) = %v, want %v", tt.hostname, allowed, tt.allowed)
			}
		})
	}
}

func TestDynamicDomainGateway_CachesResult(t *testing.T) {
	callCount := 0
	lister := &mockDomainLister{
		domains: []domain.FeedLinkDomain{
			{Domain: "cached.example.com", Scheme: "https"},
		},
	}

	gw := NewDynamicDomainGateway(lister)

	// First call loads cache
	allowed, err := gw.IsAllowedImageDomain(context.Background(), "cached.example.com")
	if err != nil || !allowed {
		t.Fatalf("first call failed: allowed=%v, err=%v", allowed, err)
	}

	// Mutate the lister to return empty (simulating change)
	lister.domains = nil
	_ = callCount

	// Second call should still use cached result (within TTL)
	allowed, err = gw.IsAllowedImageDomain(context.Background(), "cached.example.com")
	if err != nil || !allowed {
		t.Fatalf("cached call failed: allowed=%v, err=%v", allowed, err)
	}
}
