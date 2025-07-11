package utils

import (
	"net/url"
	"sync"
	"time"
)

// DomainRateLimiter manages rate limiting per domain
type DomainRateLimiter struct {
	limiters  map[string]*domainLimiter
	mu        sync.RWMutex
	rateLimit time.Duration
	burst     int
}

type domainLimiter struct {
	lastRequest time.Time
	mu          sync.Mutex
}

// NewDomainRateLimiter creates a new domain-based rate limiter
func NewDomainRateLimiter(rateLimit time.Duration, burst int) *DomainRateLimiter {
	return &DomainRateLimiter{
		limiters:  make(map[string]*domainLimiter),
		rateLimit: rateLimit,
		burst:     burst,
	}
}

// Wait blocks until the domain can make another request
func (d *DomainRateLimiter) Wait(domain string) {
	d.mu.RLock()
	limiter, exists := d.limiters[domain]
	d.mu.RUnlock()

	if !exists {
		d.mu.Lock()
		// Double check after acquiring write lock
		if limiter, exists = d.limiters[domain]; !exists {
			limiter = &domainLimiter{}
			d.limiters[domain] = limiter
		}
		d.mu.Unlock()
	}

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	if !limiter.lastRequest.IsZero() {
		elapsed := time.Since(limiter.lastRequest)
		if elapsed < d.rateLimit {
			waitTime := d.rateLimit - elapsed
			time.Sleep(waitTime)
		}
	}

	limiter.lastRequest = time.Now()
}

// extractDomain extracts domain from URL string
func extractDomain(urlStr string) string {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "unknown"
	}

	hostname := parsedURL.Hostname()
	if hostname == "" {
		return "unknown"
	}

	return hostname
}
