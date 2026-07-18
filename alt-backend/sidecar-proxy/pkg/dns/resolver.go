// Package dns provides external DNS resolution functionality for the proxy sidecar
// This package implements the critical DNS resolution bypass described in ISSUE_RESOLVE_PLAN.md
// to avoid Kubernetes internal DNS resolution and ensure true external domain resolution.
package dns

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// ExternalDNSResolver manages external DNS resolution with caching and failover
// This resolver bypasses Kubernetes internal DNS to ensure proper upstream resolution
type ExternalDNSResolver struct {
	// upstreamServers contains the list of external DNS servers to use
	upstreamServers []string

	// cache stores DNS resolution results with TTL-based expiration
	cache map[string]*CachedRecord

	// cacheMutex protects concurrent access to the cache
	cacheMutex sync.RWMutex

	// cacheTTL defines how long DNS records are cached
	cacheTTL time.Duration

	// timeout for individual DNS queries
	timeout time.Duration

	// maxCacheEntries limits cache size to prevent memory exhaustion
	maxCacheEntries int

	// client for DNS queries
	client *dns.Client

	totalQueries  atomic.Int64
	cacheHits     atomic.Int64
	cacheMisses   atomic.Int64
	failures      atomic.Int64
	lastQueryUnix atomic.Int64 // unix nanos
	latencySumNs  atomic.Int64
	latencyCount  atomic.Int64
}

// CachedRecord represents a cached DNS resolution result
type CachedRecord struct {
	IPs       []net.IP  `json:"ips"`
	ExpiresAt time.Time `json:"expires_at"`
	Domain    string    `json:"domain"`
	CreatedAt time.Time `json:"created_at"`
}

// DNSMetrics tracks DNS resolution performance and statistics
type DNSMetrics struct {
	TotalQueries   int64         `json:"total_queries"`
	CacheHits      int64         `json:"cache_hits"`
	CacheMisses    int64         `json:"cache_misses"`
	Failures       int64         `json:"failures"`
	AverageLatency time.Duration `json:"average_latency"`
	LastQueryTime  time.Time     `json:"last_query_time"`
	CacheSize      int           `json:"cache_size"`
}

// NewExternalDNSResolver creates a new DNS resolver with external server configuration
// This is the primary constructor for the DNS resolution system described in the plan
func NewExternalDNSResolver(servers []string, cacheTTL time.Duration, maxCacheEntries int) *ExternalDNSResolver {
	return &ExternalDNSResolver{
		upstreamServers: servers,
		cache:           make(map[string]*CachedRecord),
		cacheTTL:        cacheTTL,
		timeout:         5 * time.Second, // Conservative timeout for external DNS
		maxCacheEntries: maxCacheEntries,
		client: &dns.Client{
			Net:     "udp",
			Timeout: 5 * time.Second,
		},
	}
}

// ResolveExternal performs external DNS resolution with caching and failover
// This is the core function that bypasses Kubernetes DNS as specified in ISSUE_RESOLVE_PLAN.md
func (r *ExternalDNSResolver) ResolveExternal(ctx context.Context, domain string) ([]net.IP, error) {
	r.totalQueries.Add(1)
	r.lastQueryUnix.Store(time.Now().UnixNano())
	start := time.Now()

	// Check cache first
	if ips := r.getCachedIPs(domain); ips != nil {
		r.cacheHits.Add(1)
		r.recordLatency(start)
		return ips, nil
	}
	r.cacheMisses.Add(1)

	// Perform external DNS resolution with failover across multiple servers
	ips, err := r.performExternalQuery(ctx, domain)
	r.recordLatency(start)
	if err != nil {
		r.failures.Add(1)
		return nil, fmt.Errorf("external DNS resolution failed for %s: %w", domain, err)
	}

	// Cache the successful result
	r.cacheResult(domain, ips)

	return ips, nil
}

func (r *ExternalDNSResolver) recordLatency(start time.Time) {
	r.latencySumNs.Add(time.Since(start).Nanoseconds())
	r.latencyCount.Add(1)
}

// performExternalQuery executes DNS queries against external servers with failover
func (r *ExternalDNSResolver) performExternalQuery(ctx context.Context, domain string) ([]net.IP, error) {
	var lastErr error

	// Try each upstream server until one succeeds
	for _, server := range r.upstreamServers {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			// Attempt resolution with current server
			if ips, err := r.queryServer(ctx, domain, server); err == nil && len(ips) > 0 {
				return ips, nil
			} else if err != nil {
				lastErr = err
			}
		}
	}

	return nil, fmt.Errorf("all DNS servers failed, last error: %w", lastErr)
}

// queryServer performs a DNS query against a specific server.
// Tries TypeA first, then TypeAAAA so IPv6-only domains still resolve.
func (r *ExternalDNSResolver) queryServer(ctx context.Context, domain, server string) ([]net.IP, error) {
	ips, err := r.queryServerType(ctx, domain, server, dns.TypeA)
	if err == nil && len(ips) > 0 {
		return ips, nil
	}
	aaaa, aaaaErr := r.queryServerType(ctx, domain, server, dns.TypeAAAA)
	if aaaaErr == nil && len(aaaa) > 0 {
		return aaaa, nil
	}
	if err != nil {
		return nil, err
	}
	if aaaaErr != nil {
		return nil, aaaaErr
	}
	return nil, fmt.Errorf("no A/AAAA records found for %s", domain)
}

func (r *ExternalDNSResolver) queryServerType(ctx context.Context, domain, server string, qtype uint16) ([]net.IP, error) {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(domain), qtype)
	msg.RecursionDesired = true

	response, _, err := r.client.ExchangeContext(ctx, msg, server)
	if err != nil {
		return nil, fmt.Errorf("DNS query to %s failed: %w", server, err)
	}

	if response.Rcode != dns.RcodeSuccess {
		return nil, fmt.Errorf("DNS query returned error code: %d", response.Rcode)
	}

	var ips []net.IP
	for _, answer := range response.Answer {
		switch rr := answer.(type) {
		case *dns.A:
			ips = append(ips, rr.A)
		case *dns.AAAA:
			ips = append(ips, rr.AAAA)
		}
	}
	return ips, nil
}

// getCachedIPs retrieves cached IP addresses if they haven't expired.
// Expired entries are deleted under a write lock.
func (r *ExternalDNSResolver) getCachedIPs(domain string) []net.IP {
	r.cacheMutex.RLock()
	record, exists := r.cache[domain]
	if !exists {
		r.cacheMutex.RUnlock()
		return nil
	}
	if time.Now().After(record.ExpiresAt) {
		r.cacheMutex.RUnlock()
		r.cacheMutex.Lock()
		if rec, ok := r.cache[domain]; ok && time.Now().After(rec.ExpiresAt) {
			delete(r.cache, domain)
		}
		r.cacheMutex.Unlock()
		return nil
	}
	ips := record.IPs
	r.cacheMutex.RUnlock()
	return ips
}

// cacheResult stores a DNS resolution result in the cache
func (r *ExternalDNSResolver) cacheResult(domain string, ips []net.IP) {
	r.cacheMutex.Lock()
	defer r.cacheMutex.Unlock()

	// Check cache size limit and clean up if necessary
	if len(r.cache) >= r.maxCacheEntries {
		r.cleanupExpiredEntries()

		// If still over limit, remove oldest entries
		if len(r.cache) >= r.maxCacheEntries {
			r.removeOldestEntries(r.maxCacheEntries / 4) // Remove 25% of entries
		}
	}

	// Store new cache entry
	r.cache[domain] = &CachedRecord{
		IPs:       ips,
		ExpiresAt: time.Now().Add(r.cacheTTL),
		Domain:    domain,
		CreatedAt: time.Now(),
	}
}

// cleanupExpiredEntries removes expired entries from the cache
func (r *ExternalDNSResolver) cleanupExpiredEntries() {
	now := time.Now()
	for domain, record := range r.cache {
		if now.After(record.ExpiresAt) {
			delete(r.cache, domain)
		}
	}
}

// removeOldestEntries removes the oldest cache entries to manage memory usage
func (r *ExternalDNSResolver) removeOldestEntries(count int) {
	if count <= 0 {
		return
	}

	// Simple implementation: remove first N entries
	// In production, might want to sort by creation time
	removed := 0
	for domain := range r.cache {
		if removed >= count {
			break
		}
		delete(r.cache, domain)
		removed++
	}
}

// GetMetrics returns current DNS resolver metrics for monitoring
func (r *ExternalDNSResolver) GetMetrics() DNSMetrics {
	r.cacheMutex.RLock()
	cacheSize := len(r.cache)
	r.cacheMutex.RUnlock()

	var avg time.Duration
	if n := r.latencyCount.Load(); n > 0 {
		avg = time.Duration(r.latencySumNs.Load() / n)
	}
	var last time.Time
	if ns := r.lastQueryUnix.Load(); ns > 0 {
		last = time.Unix(0, ns)
	}

	return DNSMetrics{
		TotalQueries:   r.totalQueries.Load(),
		CacheHits:      r.cacheHits.Load(),
		CacheMisses:    r.cacheMisses.Load(),
		Failures:       r.failures.Load(),
		AverageLatency: avg,
		LastQueryTime:  last,
		CacheSize:      cacheSize,
	}
}

// FlushCache clears all cached DNS entries
func (r *ExternalDNSResolver) FlushCache() {
	r.cacheMutex.Lock()
	defer r.cacheMutex.Unlock()

	r.cache = make(map[string]*CachedRecord)
}

// SetTimeout configures the DNS query timeout
func (r *ExternalDNSResolver) SetTimeout(timeout time.Duration) {
	r.timeout = timeout
	r.client.Timeout = timeout
}

// ValidateDomain performs basic domain name validation
func ValidateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}

	if len(domain) > 253 {
		return fmt.Errorf("domain name too long: %d characters", len(domain))
	}

	// Basic format validation
	if domain[0] == '.' || domain[len(domain)-1] == '.' {
		return fmt.Errorf("domain cannot start or end with dot")
	}

	return nil
}
