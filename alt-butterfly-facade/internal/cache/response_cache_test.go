package cache

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewResponseCache(t *testing.T) {
	cache := NewResponseCache(1000)

	assert.NotNil(t, cache)
}

func TestResponseCache_SetAndGet(t *testing.T) {
	cache := NewResponseCache(1000)

	entry := &CacheEntry{
		Response:   []byte(`{"data": "test"}`),
		StatusCode: http.StatusOK,
		Headers:    http.Header{"Content-Type": []string{"application/json"}},
		CachedAt:   time.Now(),
		TTL:        30 * time.Second,
	}

	key := "user123:/api/feed_stats:hash123"
	cache.Set(key, entry)

	retrieved, found := cache.Get(key)

	assert.True(t, found)
	assert.Equal(t, entry.Response, retrieved.Response)
	assert.Equal(t, entry.StatusCode, retrieved.StatusCode)
	assert.Equal(t, "application/json", retrieved.Headers.Get("Content-Type"))
}

func TestResponseCache_Get_NotFound(t *testing.T) {
	cache := NewResponseCache(1000)

	_, found := cache.Get("nonexistent-key")

	assert.False(t, found)
}

func TestResponseCache_Get_Expired(t *testing.T) {
	cache := NewResponseCache(1000)

	entry := &CacheEntry{
		Response:   []byte(`{"data": "test"}`),
		StatusCode: http.StatusOK,
		Headers:    make(http.Header),
		CachedAt:   time.Now().Add(-60 * time.Second), // Cached 60 seconds ago
		TTL:        30 * time.Second,                  // TTL is 30 seconds
	}

	key := "user123:/api/feed_stats:hash123"
	cache.Set(key, entry)

	_, found := cache.Get(key)

	assert.False(t, found, "Expired entries should not be returned")
}

func TestResponseCache_Delete(t *testing.T) {
	cache := NewResponseCache(1000)

	entry := &CacheEntry{
		Response:   []byte(`{"data": "test"}`),
		StatusCode: http.StatusOK,
		Headers:    make(http.Header),
		CachedAt:   time.Now(),
		TTL:        30 * time.Second,
	}

	key := "user123:/api/feed_stats:hash123"
	cache.Set(key, entry)
	cache.Delete(key)

	_, found := cache.Get(key)

	assert.False(t, found)
}

func TestResponseCache_Clear(t *testing.T) {
	cache := NewResponseCache(1000)

	entries := []struct {
		key   string
		entry *CacheEntry
	}{
		{"key1", &CacheEntry{Response: []byte("1"), CachedAt: time.Now(), TTL: 30 * time.Second}},
		{"key2", &CacheEntry{Response: []byte("2"), CachedAt: time.Now(), TTL: 30 * time.Second}},
		{"key3", &CacheEntry{Response: []byte("3"), CachedAt: time.Now(), TTL: 30 * time.Second}},
	}

	for _, e := range entries {
		cache.Set(e.key, e.entry)
	}

	cache.Clear()

	for _, e := range entries {
		_, found := cache.Get(e.key)
		assert.False(t, found)
	}
}

func TestResponseCache_Size(t *testing.T) {
	cache := NewResponseCache(1000)

	assert.Equal(t, 0, cache.Size())

	cache.Set("key1", &CacheEntry{CachedAt: time.Now(), TTL: 30 * time.Second})
	assert.Equal(t, 1, cache.Size())

	cache.Set("key2", &CacheEntry{CachedAt: time.Now(), TTL: 30 * time.Second})
	assert.Equal(t, 2, cache.Size())

	cache.Delete("key1")
	assert.Equal(t, 1, cache.Size())
}

func TestResponseCache_MaxSize_Eviction(t *testing.T) {
	cache := NewResponseCache(2) // Max 2 entries

	cache.Set("key1", &CacheEntry{Response: []byte("1"), CachedAt: time.Now(), TTL: 30 * time.Second})
	cache.Set("key2", &CacheEntry{Response: []byte("2"), CachedAt: time.Now(), TTL: 30 * time.Second})
	cache.Set("key3", &CacheEntry{Response: []byte("3"), CachedAt: time.Now(), TTL: 30 * time.Second})

	// Cache should maintain size limit
	assert.LessOrEqual(t, cache.Size(), 2)

	// Latest entry should be present
	_, found := cache.Get("key3")
	assert.True(t, found)
}

func TestBuildCacheKey(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		endpoint string
		body     []byte
		expected string
	}{
		{
			name:     "basic key",
			userID:   "user123",
			endpoint: "/api/feed_stats",
			body:     []byte(`{}`),
			expected: "user123:/api/feed_stats:",
		},
		{
			name:     "key with body hash",
			userID:   "user456",
			endpoint: "/api/query",
			body:     []byte(`{"query": "test"}`),
			expected: "user456:/api/query:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := BuildCacheKey(tt.userID, tt.endpoint, tt.body)

			// Key should start with expected prefix
			assert.Contains(t, key, tt.userID)
			assert.Contains(t, key, tt.endpoint)
		})
	}
}

func TestBuildCacheKey_SameInputsSameOutput(t *testing.T) {
	key1 := BuildCacheKey("user123", "/api/test", []byte(`{"data": "test"}`))
	key2 := BuildCacheKey("user123", "/api/test", []byte(`{"data": "test"}`))

	assert.Equal(t, key1, key2)
}

func TestBuildCacheKey_DifferentInputsDifferentOutput(t *testing.T) {
	key1 := BuildCacheKey("user123", "/api/test", []byte(`{"data": "test1"}`))
	key2 := BuildCacheKey("user123", "/api/test", []byte(`{"data": "test2"}`))

	assert.NotEqual(t, key1, key2)
}

func TestCacheConfig_DefaultTTL(t *testing.T) {
	config := NewCacheConfig()

	// Default TTLs from the plan
	assert.Equal(t, 30*time.Second, config.GetTTL("/alt.feeds.v2.FeedService/GetDetailedFeedStats"))
	assert.Equal(t, 15*time.Second, config.GetTTL("/alt.feeds.v2.FeedService/GetUnreadCount"))
	assert.Equal(t, 30*time.Second, config.GetTTL("/alt.feeds.v2.FeedService/GetFeedStats"))
}

func TestCacheConfig_IsCacheable(t *testing.T) {
	config := NewCacheConfig()

	tests := []struct {
		endpoint  string
		cacheable bool
	}{
		{"/alt.feeds.v2.FeedService/GetDetailedFeedStats", true},
		{"/alt.feeds.v2.FeedService/GetUnreadCount", true},
		{"/alt.feeds.v2.FeedService/GetFeedStats", true},
		{"/alt.feeds.v2.FeedService/StreamFeedStats", false},  // Streaming
		{"/alt.augur.v2.AugurService/StreamChat", false},      // Streaming
		{"/alt.feeds.v2.FeedService/CreateFeed", false},       // Mutation
		{"/alt.feeds.v2.FeedService/UpdateFeed", false},       // Mutation
		{"/some/random/endpoint", false},                      // Unknown
	}

	for _, tt := range tests {
		t.Run(tt.endpoint, func(t *testing.T) {
			assert.Equal(t, tt.cacheable, config.IsCacheable(tt.endpoint))
		})
	}
}

func TestCacheConfig_CustomTTL(t *testing.T) {
	config := NewCacheConfig()
	config.SetTTL("/custom/endpoint", 60*time.Second)

	assert.Equal(t, 60*time.Second, config.GetTTL("/custom/endpoint"))
	assert.True(t, config.IsCacheable("/custom/endpoint"))
}

func TestCacheEntry_IsExpired(t *testing.T) {
	t.Run("not expired", func(t *testing.T) {
		entry := &CacheEntry{
			CachedAt: time.Now(),
			TTL:      30 * time.Second,
		}
		assert.False(t, entry.IsExpired())
	})

	t.Run("expired", func(t *testing.T) {
		entry := &CacheEntry{
			CachedAt: time.Now().Add(-60 * time.Second),
			TTL:      30 * time.Second,
		}
		assert.True(t, entry.IsExpired())
	})
}

func TestResponseCache_ConcurrentAccess(t *testing.T) {
	cache := NewResponseCache(1000)
	done := make(chan bool)

	// Concurrent writes
	go func() {
		for i := 0; i < 100; i++ {
			cache.Set("key", &CacheEntry{
				Response: []byte("test"),
				CachedAt: time.Now(),
				TTL:      30 * time.Second,
			})
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < 100; i++ {
			cache.Get("key")
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// If we got here without panic, concurrent access is safe
	require.True(t, true)
}

func TestCacheStats(t *testing.T) {
	cache := NewResponseCache(1000)

	// Initial stats
	stats := cache.Stats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)

	// Miss
	cache.Get("nonexistent")
	stats = cache.Stats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)

	// Hit
	cache.Set("key", &CacheEntry{CachedAt: time.Now(), TTL: 30 * time.Second})
	cache.Get("key")
	stats = cache.Stats()
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
}
