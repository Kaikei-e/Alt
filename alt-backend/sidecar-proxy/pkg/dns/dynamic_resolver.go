// Package dns provides dynamic DNS resolution and on-memory domain management
// オンメモリDNS管理システム: 動的ドメイン解決とキャッシング
package dns

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
)

// DynamicResolver manages on-memory DNS resolution and domain learning
// オンメモリDNS管理: 動的ドメイン解決システム
type DynamicResolver struct {
	// Static domain patterns from configuration
	staticDomains []*regexp.Regexp

	// Literal domain names backing staticDomains (anchors/escaping stripped),
	// used to scope auto-learning to genuine subdomains of an already
	// statically-approved base domain.
	staticDomainNames []string

	// Dynamic learned domains (on-memory cache)
	learnedDomains map[string]*LearnedDomain
	learnedMutex   sync.RWMutex

	// DNS resolution cache
	dnsCache      map[string]*DNSEntry
	dnsCacheMutex sync.RWMutex

	// Configuration
	dnsServers        []string
	cacheTimeout      time.Duration
	maxCacheEntries   int
	maxLearnedDomains int
}

// LearnedDomain represents a dynamically learned domain
type LearnedDomain struct {
	Domain    string    `json:"domain"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	UseCount  int       `json:"use_count"`
}

// DNSEntry represents a cached DNS resolution result
type DNSEntry struct {
	Domain     string        `json:"domain"`
	IPs        []string      `json:"ips"`
	ResolvedAt time.Time     `json:"resolved_at"`
	TTL        time.Duration `json:"ttl"`
}

// NewDynamicResolver creates a new dynamic DNS resolver
func NewDynamicResolver(staticDomains []*regexp.Regexp, dnsServers []string, cacheTimeout time.Duration, maxCacheEntries int) *DynamicResolver {
	return &DynamicResolver{
		staticDomains:     staticDomains,
		staticDomainNames: extractStaticDomainNames(staticDomains),
		learnedDomains:    make(map[string]*LearnedDomain),
		dnsCache:          make(map[string]*DNSEntry),
		dnsServers:        dnsServers,
		cacheTimeout:      cacheTimeout,
		maxCacheEntries:   maxCacheEntries,
		maxLearnedDomains: 100, // Reasonable limit for memory management
	}
}

// extractStaticDomainNames recovers the literal domain (e.g. "zenn.dev")
// backing each compiled `^zenn\.dev$`-style static allowlist pattern, so
// shouldLearnDomain can test genuine subdomain relationships without
// re-parsing config.
func extractStaticDomainNames(staticDomains []*regexp.Regexp) []string {
	names := make([]string, 0, len(staticDomains))
	for _, pattern := range staticDomains {
		src := pattern.String()
		src = strings.TrimPrefix(src, "^")
		src = strings.TrimSuffix(src, "$")
		src = strings.ReplaceAll(src, "\\.", ".")
		if src != "" {
			names = append(names, src)
		}
	}
	return names
}

// IsDomainAllowed checks if domain is allowed through static patterns or dynamic learning
// オンメモリDNS管理: 動的ドメイン許可判定
func (dr *DynamicResolver) IsDomainAllowed(domain string) (allowed bool, learned bool) {
	// 1. Check static domain patterns first
	for _, pattern := range dr.staticDomains {
		if pattern.MatchString(domain) {
			return true, false
		}
	}

	// 2. Check learned domains
	dr.learnedMutex.RLock()
	_, exists := dr.learnedDomains[domain]
	dr.learnedMutex.RUnlock()

	if exists {
		// Update last seen time
		dr.updateLearnedDomain(domain)
		return true, false
	}

	// 3. Dynamic learning: Check if domain should be auto-learned
	if dr.shouldLearnDomain(domain) {
		dr.addLearnedDomain(domain)
		return true, true
	}

	return false, false
}

// shouldLearnDomain implements domain learning heuristics
// オンメモリDNS管理: ドメイン学習ヒューリスティクス
//
// Auto-learning must never widen the allowlist to arbitrary destinations:
// generic TLD suffixes (.com/.org/.../.dev) and generic prefixes
// (api./cdn./feeds.) match almost any domain and previously made the
// CONNECT/persistent-tunnel allowlist a no-op (any domain would be
// "learned" on first use). The only case learned here is a genuine
// subdomain of a domain that is already statically allowed/reviewed
// (e.g. "api.zenn.dev" when "zenn.dev" is in ALLOWED_DOMAINS).
func (dr *DynamicResolver) shouldLearnDomain(domain string) bool {
	// Basic domain validation
	if len(domain) == 0 || len(domain) > 253 {
		return false
	}

	// Must contain at least one dot
	if !strings.Contains(domain, ".") {
		return false
	}

	for _, base := range dr.staticDomainNames {
		if base == "" {
			continue
		}
		if strings.HasSuffix(domain, "."+base) {
			return true
		}
	}

	return false
}

// addLearnedDomain adds a new domain to the learned cache
func (dr *DynamicResolver) addLearnedDomain(domain string) {
	dr.learnedMutex.Lock()
	defer dr.learnedMutex.Unlock()

	// Check if we're at capacity
	if len(dr.learnedDomains) >= dr.maxLearnedDomains {
		// Remove oldest domain
		dr.evictOldestLearnedDomain()
	}

	now := time.Now()
	dr.learnedDomains[domain] = &LearnedDomain{
		Domain:    domain,
		FirstSeen: now,
		LastSeen:  now,
		UseCount:  1,
	}
}

// updateLearnedDomain updates the last seen time and use count
func (dr *DynamicResolver) updateLearnedDomain(domain string) {
	dr.learnedMutex.Lock()
	defer dr.learnedMutex.Unlock()

	if entry, exists := dr.learnedDomains[domain]; exists {
		entry.LastSeen = time.Now()
		entry.UseCount++
	}
}

// evictOldestLearnedDomain removes the least recently used domain
func (dr *DynamicResolver) evictOldestLearnedDomain() {
	var oldestDomain string
	var oldestTime time.Time = time.Now()

	for domain, entry := range dr.learnedDomains {
		if entry.LastSeen.Before(oldestTime) {
			oldestTime = entry.LastSeen
			oldestDomain = domain
		}
	}

	if oldestDomain != "" {
		delete(dr.learnedDomains, oldestDomain)
	}
}

// PreResolveDomain performs DNS pre-resolution and caches the result
// オンメモリDNS管理: DNS事前解決とキャッシング
func (dr *DynamicResolver) PreResolveDomain(domain string) error {
	// Check if already cached and not expired
	dr.dnsCacheMutex.RLock()
	if entry, exists := dr.dnsCache[domain]; exists {
		if time.Since(entry.ResolvedAt) < entry.TTL {
			dr.dnsCacheMutex.RUnlock()
			return nil // Already cached and valid
		}
	}
	dr.dnsCacheMutex.RUnlock()

	// Perform DNS resolution
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, domain)
	if err != nil {
		return fmt.Errorf("DNS resolution failed for %s: %w", domain, err)
	}

	// Convert to string slice
	ipStrings := make([]string, len(ips))
	for i, ip := range ips {
		ipStrings[i] = ip.IP.String()
	}

	// Cache the result
	dr.dnsCacheMutex.Lock()
	defer dr.dnsCacheMutex.Unlock()

	// Check if we're at capacity
	if len(dr.dnsCache) >= dr.maxCacheEntries {
		dr.evictOldestDNSEntry()
	}

	dr.dnsCache[domain] = &DNSEntry{
		Domain:     domain,
		IPs:        ipStrings,
		ResolvedAt: time.Now(),
		TTL:        dr.cacheTimeout,
	}

	return nil
}

// evictOldestDNSEntry removes the oldest DNS cache entry
func (dr *DynamicResolver) evictOldestDNSEntry() {
	var oldestDomain string
	var oldestTime time.Time = time.Now()

	for domain, entry := range dr.dnsCache {
		if entry.ResolvedAt.Before(oldestTime) {
			oldestTime = entry.ResolvedAt
			oldestDomain = domain
		}
	}

	if oldestDomain != "" {
		delete(dr.dnsCache, oldestDomain)
	}
}

// GetLearnedDomains returns current learned domains for monitoring
func (dr *DynamicResolver) GetLearnedDomains() map[string]*LearnedDomain {
	dr.learnedMutex.RLock()
	defer dr.learnedMutex.RUnlock()

	result := make(map[string]*LearnedDomain)
	for k, v := range dr.learnedDomains {
		result[k] = &LearnedDomain{
			Domain:    v.Domain,
			FirstSeen: v.FirstSeen,
			LastSeen:  v.LastSeen,
			UseCount:  v.UseCount,
		}
	}
	return result
}

// GetDNSCacheStats returns DNS cache statistics
func (dr *DynamicResolver) GetDNSCacheStats() map[string]interface{} {
	dr.dnsCacheMutex.RLock()
	defer dr.dnsCacheMutex.RUnlock()

	return map[string]interface{}{
		"cache_size":    len(dr.dnsCache),
		"max_entries":   dr.maxCacheEntries,
		"cache_timeout": dr.cacheTimeout.String(),
		"learned_count": len(dr.learnedDomains),
		"max_learned":   dr.maxLearnedDomains,
	}
}
