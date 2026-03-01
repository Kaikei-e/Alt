package image_proxy_gateway

import (
	"alt/domain"
	"context"
	"strings"
	"sync"
	"time"
)

// DomainLister is the interface for listing feed link domains.
type DomainLister interface {
	ListFeedLinkDomains(ctx context.Context) ([]domain.FeedLinkDomain, error)
}

// majorCDNDomains is the static list of well-known CDN/image hosting domains.
var majorCDNDomains = []string{
	"cdn.mos.cms.futurecdn.net",
	"images.unsplash.com",
	"img.youtube.com",
	"i.ytimg.com",
	"i.imgur.com",
	"pbs.twimg.com",
	"cdn.pixabay.com",
	"images.pexels.com",
	"cdn-images-1.medium.com",
	"miro.medium.com",
}

// majorCDNSuffixes are domain suffixes for CDN providers.
var majorCDNSuffixes = []string{
	"cloudfront.net",
	"cloudinary.com",
	"imgix.net",
	"fastly.net",
	"akamaized.net",
	"githubusercontent.com",
	"googleapis.com",
	"wp.com",
}

const domainCacheTTL = 5 * time.Minute

// DynamicDomainGateway implements DynamicDomainPort with subscription + CDN domains.
type DynamicDomainGateway struct {
	lister DomainLister

	mu              sync.RWMutex
	cachedDomains   map[string]bool
	cacheExpiry     time.Time
}

// NewDynamicDomainGateway creates a new DynamicDomainGateway.
func NewDynamicDomainGateway(lister DomainLister) *DynamicDomainGateway {
	return &DynamicDomainGateway{
		lister: lister,
	}
}

// IsAllowedImageDomain checks if the hostname is in the dynamic allowlist.
// The allowlist is: subscription domains + CDN domains + static domain list.
func (g *DynamicDomainGateway) IsAllowedImageDomain(ctx context.Context, hostname string) (bool, error) {
	hostname = strings.ToLower(hostname)

	// Check static CDN domains first (no cache needed)
	for _, cdn := range majorCDNDomains {
		if hostname == cdn {
			return true, nil
		}
	}

	// Check CDN suffixes (match exact or as subdomain)
	for _, suffix := range majorCDNSuffixes {
		if hostname == suffix || strings.HasSuffix(hostname, "."+suffix) {
			return true, nil
		}
	}

	// Check subscription domains (cached) — exact match and subdomain match.
	// Image CDN subdomains (e.g. media2.dev.to) should be allowed when the
	// parent domain (dev.to) is a subscribed feed source.
	subscriptionDomains, err := g.getSubscriptionDomains(ctx)
	if err != nil {
		return false, err
	}

	if subscriptionDomains[hostname] {
		return true, nil
	}

	// Subdomain match: walk up the hostname (media2.dev.to → dev.to)
	for h := hostname; ; {
		dot := strings.Index(h, ".")
		if dot < 0 {
			break
		}
		parent := h[dot+1:]
		if parent == "" {
			break
		}
		if subscriptionDomains[parent] {
			return true, nil
		}
		h = parent
	}

	return false, nil
}

// getSubscriptionDomains returns cached subscription domains, refreshing if expired.
func (g *DynamicDomainGateway) getSubscriptionDomains(ctx context.Context) (map[string]bool, error) {
	g.mu.RLock()
	if g.cachedDomains != nil && time.Now().Before(g.cacheExpiry) {
		domains := g.cachedDomains
		g.mu.RUnlock()
		return domains, nil
	}
	g.mu.RUnlock()

	// Refresh cache
	g.mu.Lock()
	defer g.mu.Unlock()

	// Double-check after acquiring write lock
	if g.cachedDomains != nil && time.Now().Before(g.cacheExpiry) {
		return g.cachedDomains, nil
	}

	feedDomains, err := g.lister.ListFeedLinkDomains(ctx)
	if err != nil {
		// If we have stale cache, use it
		if g.cachedDomains != nil {
			return g.cachedDomains, nil
		}
		return nil, err
	}

	domainSet := make(map[string]bool, len(feedDomains)*2)
	for _, d := range feedDomains {
		domain := strings.ToLower(d.Domain)
		domainSet[domain] = true

		// Also index parent domains so that subdomain matching works
		// across sibling subdomains (e.g. www.wired.com → wired.com
		// allows media.wired.com). Stop at 2-label domains to avoid
		// over-permissive entries like "co.uk" or "com".
		for h := domain; ; {
			dot := strings.Index(h, ".")
			if dot < 0 {
				break
			}
			parent := h[dot+1:]
			// Only add parents with 2+ labels (e.g. wired.com, bbci.co.uk)
			if strings.Contains(parent, ".") {
				domainSet[parent] = true
			}
			h = parent
		}
	}

	g.cachedDomains = domainSet
	g.cacheExpiry = time.Now().Add(domainCacheTTL)
	return domainSet, nil
}
