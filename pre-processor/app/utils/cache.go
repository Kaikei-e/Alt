package utils

import (
	"container/list"
	"context"
	"sync"
	"time"

	logger "pre-processor/utils/logger"
)

// LRUCache implements a thread-safe Least Recently Used cache
type LRUCache struct {
	capacity int
	items    map[string]*list.Element
	order    *list.List
	mutex    sync.RWMutex
}

// LRUItem represents an item in the LRU cache
type LRUItem struct {
	key   string
	value interface{}
}

// NewLRUCache creates a new LRU cache with the specified capacity
func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

// Get retrieves a value from the cache and marks it as recently used
func (c *LRUCache) Get(key string) (interface{}, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if element, found := c.items[key]; found {
		// Move to front (most recently used)
		c.order.MoveToFront(element)
		return element.Value.(*LRUItem).value, true
	}

	return nil, false
}

// Set adds or updates a value in the cache
func (c *LRUCache) Set(key string, value interface{}) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if element, found := c.items[key]; found {
		// Update existing item
		element.Value.(*LRUItem).value = value
		c.order.MoveToFront(element)
		return
	}

	// Add new item
	item := &LRUItem{key: key, value: value}
	element := c.order.PushFront(item)
	c.items[key] = element

	// Check capacity and evict if necessary
	if c.order.Len() > c.capacity {
		c.evictLRU()
	}
}

// Clear removes all items from the cache
func (c *LRUCache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.items = make(map[string]*list.Element)
	c.order.Init()
}

// Size returns the current number of items in the cache
func (c *LRUCache) Size() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return len(c.items)
}

// Capacity returns the cache capacity
func (c *LRUCache) Capacity() int {
	return c.capacity
}

// evictLRU removes the least recently used item
func (c *LRUCache) evictLRU() {
	if c.order.Len() == 0 {
		return
	}

	// Remove from back (least recently used)
	element := c.order.Back()
	if element != nil {
		c.order.Remove(element)
		item := element.Value.(*LRUItem)
		delete(c.items, item.key)
	}
}

// URLDeduplicationCache handles URL deduplication with TTL
type URLDeduplicationCache struct {
	urls   map[string]time.Time
	ttl    time.Duration
	mutex  sync.RWMutex
	lastCleanup time.Time
}

// NewURLDeduplicationCache creates a new URL deduplication cache
func NewURLDeduplicationCache(maxSize int, ttl time.Duration) *URLDeduplicationCache {
	return &URLDeduplicationCache{
		urls:        make(map[string]time.Time),
		ttl:         ttl,
		lastCleanup: time.Now(),
	}
}

// IsDuplicate checks if a URL is a duplicate and adds it if not
func (c *URLDeduplicationCache) IsDuplicate(url string) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now()

	// Clean up expired entries periodically
	if now.Sub(c.lastCleanup) > c.ttl/2 {
		c.cleanupExpired(now)
		c.lastCleanup = now
	}

	// Check if URL exists and is not expired
	if timestamp, exists := c.urls[url]; exists {
		if now.Sub(timestamp) <= c.ttl {
			return true // Duplicate
		}
		// Expired, remove it
		delete(c.urls, url)
	}

	// Add URL with current timestamp
	c.urls[url] = now
	return false // Not a duplicate
}

// Size returns the current number of URLs in the cache
func (c *URLDeduplicationCache) Size() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return len(c.urls)
}

// cleanupExpired removes expired URLs from the cache
func (c *URLDeduplicationCache) cleanupExpired(now time.Time) {
	for url, timestamp := range c.urls {
		if now.Sub(timestamp) > c.ttl {
			delete(c.urls, url)
		}
	}
}

// CachedFeed represents a cached RSS feed
type CachedFeed struct {
	URL       string    `json:"url"`
	Content   string    `json:"content"`
	ETag      string    `json:"etag"`
	LastMod   string    `json:"last_modified"`
	CachedAt  time.Time `json:"cached_at"`
}

// FeedCache handles caching of RSS feed content
type FeedCache struct {
	feeds map[string]*CachedFeed
	ttl   time.Duration
	mutex sync.RWMutex
}

// NewFeedCache creates a new feed cache
func NewFeedCache(maxSize int, ttl time.Duration) *FeedCache {
	return &FeedCache{
		feeds: make(map[string]*CachedFeed),
		ttl:   ttl,
	}
}

// GetFeed retrieves a cached feed if it exists and is not expired
func (c *FeedCache) GetFeed(url string) *CachedFeed {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if feed, exists := c.feeds[url]; exists {
		if time.Since(feed.CachedAt) <= c.ttl {
			return feed
		}
		// Remove expired feed
		delete(c.feeds, url)
	}

	return nil
}

// SetFeed caches a feed
func (c *FeedCache) SetFeed(url string, feed *CachedFeed) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	feed.CachedAt = time.Now()
	c.feeds[url] = feed
}

// IsFeedFresh checks if a feed is fresh based on ETag
func (c *FeedCache) IsFeedFresh(url string, etag string) bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if feed, exists := c.feeds[url]; exists {
		if time.Since(feed.CachedAt) <= c.ttl {
			return feed.ETag == etag
		}
	}

	return false
}

// Size returns the current number of feeds in the cache
func (c *FeedCache) Size() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return len(c.feeds)
}

// CacheStats represents cache statistics
type CacheStats struct {
	LRUSize       int `json:"lru_size"`
	LRUCapacity   int `json:"lru_capacity"`
	URLDedupSize  int `json:"url_dedup_size"`
	FeedCacheSize int `json:"feed_cache_size"`
}

// CacheManager manages all cache instances
type CacheManager struct {
	LRU            *LRUCache
	URLDedup       *URLDeduplicationCache
	FeedCache      *FeedCache
	metricsEnabled bool
	metricsStop    chan struct{}
	metricsMutex   sync.RWMutex
}

// NewCacheManager creates a new cache manager with default settings
func NewCacheManager() *CacheManager {
	return &CacheManager{
		LRU:         NewLRUCache(1000),                                    // 1000 items
		URLDedup:    NewURLDeduplicationCache(10000, 24*time.Hour),        // 24 hour TTL
		FeedCache:   NewFeedCache(500, 30*time.Minute),                    // 30 minute TTL
		metricsStop: make(chan struct{}),
	}
}

// GetCacheStats returns statistics for all caches
func (m *CacheManager) GetCacheStats() *CacheStats {
	return &CacheStats{
		LRUSize:       m.LRU.Size(),
		LRUCapacity:   m.LRU.Capacity(),
		URLDedupSize:  m.URLDedup.Size(),
		FeedCacheSize: m.FeedCache.Size(),
	}
}

// StartMetricsCollection starts collecting cache metrics
func (m *CacheManager) StartMetricsCollection(ctx context.Context) error {
	m.metricsMutex.Lock()
	defer m.metricsMutex.Unlock()

	if m.metricsEnabled {
		return nil // Already started
	}

	m.metricsEnabled = true

	go func() {
		ticker := time.NewTicker(60 * time.Second) // Every minute
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-m.metricsStop:
				return
			case <-ticker.C:
				stats := m.GetCacheStats()
				logger.Logger.Info("Cache metrics",
					"lru_size", stats.LRUSize,
					"lru_capacity", stats.LRUCapacity,
					"url_dedup_size", stats.URLDedupSize,
					"feed_cache_size", stats.FeedCacheSize)
			}
		}
	}()

	logger.Logger.Info("Started cache metrics collection")
	return nil
}

// StopMetricsCollection stops collecting cache metrics
func (m *CacheManager) StopMetricsCollection() {
	m.metricsMutex.Lock()
	defer m.metricsMutex.Unlock()

	if !m.metricsEnabled {
		return
	}

	m.metricsEnabled = false
	close(m.metricsStop)
	m.metricsStop = make(chan struct{})

	logger.Logger.Info("Stopped cache metrics collection")
}

// ClearAll clears all caches
func (m *CacheManager) ClearAll() {
	m.LRU.Clear()
	
	// Clear URL deduplication cache
	m.URLDedup.mutex.Lock()
	m.URLDedup.urls = make(map[string]time.Time)
	m.URLDedup.mutex.Unlock()
	
	// Clear feed cache
	m.FeedCache.mutex.Lock()
	m.FeedCache.feeds = make(map[string]*CachedFeed)
	m.FeedCache.mutex.Unlock()

	logger.Logger.Info("Cleared all caches")
}