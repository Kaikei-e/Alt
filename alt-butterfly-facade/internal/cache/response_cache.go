// Package cache provides response caching functionality for the BFF service.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// CacheEntry represents a cached response.
type CacheEntry struct {
	Response   []byte
	StatusCode int
	Headers    http.Header
	CachedAt   time.Time
	TTL        time.Duration
}

// IsExpired checks if the cache entry has expired.
func (e *CacheEntry) IsExpired() bool {
	return time.Now().After(e.CachedAt.Add(e.TTL))
}

// CacheStats holds cache statistics.
type CacheStats struct {
	Hits   int64
	Misses int64
	Size   int
}

// ResponseCache is a thread-safe in-memory cache for HTTP responses.
type ResponseCache struct {
	mu       sync.RWMutex
	entries  map[string]*CacheEntry
	order    []string // For LRU eviction
	maxSize  int
	hits     int64
	misses   int64
}

// NewResponseCache creates a new response cache with the given maximum size.
func NewResponseCache(maxSize int) *ResponseCache {
	return &ResponseCache{
		entries: make(map[string]*CacheEntry),
		order:   make([]string, 0),
		maxSize: maxSize,
	}
}

// Get retrieves a cache entry by key.
// Returns the entry and true if found and not expired, nil and false otherwise.
func (c *ResponseCache) Get(key string) (*CacheEntry, bool) {
	c.mu.RLock()
	entry, found := c.entries[key]
	c.mu.RUnlock()

	if !found {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	if entry.IsExpired() {
		c.Delete(key)
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	atomic.AddInt64(&c.hits, 1)
	return entry, true
}

// Set stores a cache entry.
// If the cache is at capacity, it evicts the oldest entry.
func (c *ResponseCache) Set(key string, entry *CacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if key already exists
	if _, exists := c.entries[key]; exists {
		c.entries[key] = entry
		return
	}

	// Evict if at capacity
	for len(c.entries) >= c.maxSize && len(c.order) > 0 {
		oldestKey := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldestKey)
	}

	c.entries[key] = entry
	c.order = append(c.order, key)
}

// Delete removes a cache entry by key.
func (c *ResponseCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, key)

	// Remove from order slice
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
}

// Clear removes all entries from the cache.
func (c *ResponseCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*CacheEntry)
	c.order = make([]string, 0)
}

// Size returns the current number of entries in the cache.
func (c *ResponseCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Stats returns cache statistics.
func (c *ResponseCache) Stats() CacheStats {
	return CacheStats{
		Hits:   atomic.LoadInt64(&c.hits),
		Misses: atomic.LoadInt64(&c.misses),
		Size:   c.Size(),
	}
}

// BuildCacheKey creates a cache key from user ID, endpoint, and request body.
func BuildCacheKey(userID, endpoint string, body []byte) string {
	hash := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(hash[:8]) // First 8 bytes for brevity
	return userID + ":" + endpoint + ":" + bodyHash
}

// CacheConfig holds configuration for which endpoints are cacheable and their TTLs.
type CacheConfig struct {
	mu       sync.RWMutex
	ttls     map[string]time.Duration
	disabled bool
}

// Default TTLs for cacheable endpoints
var defaultTTLs = map[string]time.Duration{
	"/alt.feeds.v2.FeedService/GetDetailedFeedStats": 30 * time.Second,
	"/alt.feeds.v2.FeedService/GetUnreadCount":       15 * time.Second,
	"/alt.feeds.v2.FeedService/GetFeedStats":         30 * time.Second,
}

// Streaming endpoints that should never be cached
var streamingEndpoints = map[string]bool{
	"/alt.feeds.v2.FeedService/StreamFeedStats":              true,
	"/alt.feeds.v2.FeedService/StreamSummarize":              true,
	"/alt.augur.v2.AugurService/StreamChat":                  true,
	"/alt.morning_letter.v2.MorningLetterService/StreamChat": true,
}

// Mutation endpoints that should never be cached
var mutationEndpoints = map[string]bool{
	"/alt.feeds.v2.FeedService/CreateFeed":   true,
	"/alt.feeds.v2.FeedService/UpdateFeed":   true,
	"/alt.feeds.v2.FeedService/DeleteFeed":   true,
	"/alt.feeds.v2.FeedService/MarkAsRead":   true,
	"/alt.feeds.v2.FeedService/MarkAsUnread": true,
}

// NewCacheConfig creates a new cache configuration with default TTLs.
func NewCacheConfig() *CacheConfig {
	config := &CacheConfig{
		ttls: make(map[string]time.Duration),
	}

	// Copy default TTLs
	for endpoint, ttl := range defaultTTLs {
		config.ttls[endpoint] = ttl
	}

	return config
}

// GetTTL returns the TTL for an endpoint.
// Returns 0 if the endpoint is not cacheable.
func (c *CacheConfig) GetTTL(endpoint string) time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if ttl, ok := c.ttls[endpoint]; ok {
		return ttl
	}
	return 0
}

// SetTTL sets a custom TTL for an endpoint.
func (c *CacheConfig) SetTTL(endpoint string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ttls[endpoint] = ttl
}

// IsCacheable returns true if the endpoint can be cached.
func (c *CacheConfig) IsCacheable(endpoint string) bool {
	// Never cache streaming endpoints
	if streamingEndpoints[endpoint] {
		return false
	}

	// Never cache mutations
	if isMutation(endpoint) {
		return false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check if disabled globally
	if c.disabled {
		return false
	}

	// Check if we have a TTL configured
	_, ok := c.ttls[endpoint]
	return ok
}

// SetDisabled enables or disables caching globally.
func (c *CacheConfig) SetDisabled(disabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.disabled = disabled
}

// isMutation checks if an endpoint is a mutation operation.
func isMutation(endpoint string) bool {
	if mutationEndpoints[endpoint] {
		return true
	}

	// Heuristic: endpoints with Create, Update, Delete, Mark are mutations
	lowerEndpoint := strings.ToLower(endpoint)
	mutationPrefixes := []string{"/create", "/update", "/delete", "/mark", "/set", "/add", "/remove"}
	for _, prefix := range mutationPrefixes {
		if strings.Contains(lowerEndpoint, prefix) {
			return true
		}
	}

	return false
}
