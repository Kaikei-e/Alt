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
			{Domain: "news.custom.org", Scheme: "https"},
		},
	})

	tests := []struct {
		hostname string
		allowed  bool
	}{
		{"blog.example.com", true},
		{"news.custom.org", true},
		{"other.example.com", false},
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
