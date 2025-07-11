package utils

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLRUCache(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create LRU cache with specified capacity",
			test: func(t *testing.T) {
				cache := NewLRUCache(10)
				assert.NotNil(t, cache)
				assert.Equal(t, 10, cache.Capacity())
				assert.Equal(t, 0, cache.Size())
			},
		},
		{
			name: "should set and get values",
			test: func(t *testing.T) {
				cache := NewLRUCache(3)

				cache.Set("key1", "value1")
				cache.Set("key2", "value2")

				value, found := cache.Get("key1")
				assert.True(t, found)
				assert.Equal(t, "value1", value)

				value, found = cache.Get("key2")
				assert.True(t, found)
				assert.Equal(t, "value2", value)

				_, found = cache.Get("nonexistent")
				assert.False(t, found)
			},
		},
		{
			name: "should evict least recently used items when capacity exceeded",
			test: func(t *testing.T) {
				cache := NewLRUCache(2) // Small capacity for testing

				cache.Set("key1", "value1")
				cache.Set("key2", "value2")
				cache.Set("key3", "value3") // Should evict key1

				_, found := cache.Get("key1")
				assert.False(t, found, "key1 should be evicted")

				_, found = cache.Get("key2")
				assert.True(t, found, "key2 should still exist")

				_, found = cache.Get("key3")
				assert.True(t, found, "key3 should exist")
			},
		},
		{
			name: "should update access order on get",
			test: func(t *testing.T) {
				cache := NewLRUCache(2)

				cache.Set("key1", "value1")
				cache.Set("key2", "value2")

				// Access key1 to make it recently used
				cache.Get("key1")

				// Add key3, should evict key2 (not key1)
				cache.Set("key3", "value3")

				_, found := cache.Get("key1")
				assert.True(t, found, "key1 should still exist after access")

				_, found = cache.Get("key2")
				assert.False(t, found, "key2 should be evicted")
			},
		},
		{
			name: "should handle cache clear",
			test: func(t *testing.T) {
				cache := NewLRUCache(5)

				cache.Set("key1", "value1")
				cache.Set("key2", "value2")
				assert.Equal(t, 2, cache.Size())

				cache.Clear()
				assert.Equal(t, 0, cache.Size())

				_, found := cache.Get("key1")
				assert.False(t, found)
			},
		},
		{
			name: "should handle concurrent access safely",
			test: func(t *testing.T) {
				cache := NewLRUCache(100)
				done := make(chan bool, 2)

				// Writer goroutine
				go func() {
					defer func() { done <- true }()
					for i := 0; i < 50; i++ {
						cache.Set(string(rune(i)), i)
					}
				}()

				// Reader goroutine
				go func() {
					defer func() { done <- true }()
					for i := 0; i < 50; i++ {
						cache.Get(string(rune(i)))
					}
				}()

				// Wait for both goroutines
				<-done
				<-done

				// Cache should still be functional
				cache.Set("test", "value")
				value, found := cache.Get("test")
				assert.True(t, found)
				assert.Equal(t, "value", value)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestURLDeduplicationCache(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create URL deduplication cache",
			test: func(t *testing.T) {
				cache := NewURLDeduplicationCache(1000, 1*time.Hour)
				assert.NotNil(t, cache)
			},
		},
		{
			name: "should detect duplicate URLs",
			test: func(t *testing.T) {
				cache := NewURLDeduplicationCache(100, 1*time.Hour)

				url := "https://example.com/article1"

				// First time should not be duplicate
				isDuplicate := cache.IsDuplicate(url)
				assert.False(t, isDuplicate)

				// Second time should be duplicate
				isDuplicate = cache.IsDuplicate(url)
				assert.True(t, isDuplicate)
			},
		},
		{
			name: "should handle URL expiration",
			test: func(t *testing.T) {
				cache := NewURLDeduplicationCache(100, 10*time.Millisecond)

				url := "https://example.com/article1"

				// Add URL
				cache.IsDuplicate(url)

				// Wait for expiration
				time.Sleep(20 * time.Millisecond)

				// Should not be duplicate after expiration
				isDuplicate := cache.IsDuplicate(url)
				assert.False(t, isDuplicate)
			},
		},
		{
			name: "should clean up expired entries",
			test: func(t *testing.T) {
				cache := NewURLDeduplicationCache(100, 10*time.Millisecond)

				// Add multiple URLs
				urls := []string{
					"https://example.com/article1",
					"https://example.com/article2",
					"https://example.com/article3",
				}

				for _, url := range urls {
					cache.IsDuplicate(url)
				}

				assert.Equal(t, 3, cache.Size())

				// Wait for expiration
				time.Sleep(20 * time.Millisecond)

				// Trigger cleanup by adding new URL
				cache.IsDuplicate("https://example.com/new")

				// Old entries should be cleaned up
				assert.Equal(t, 1, cache.Size())
			},
		},
		{
			name: "should handle concurrent URL checking",
			test: func(t *testing.T) {
				cache := NewURLDeduplicationCache(1000, 1*time.Hour)
				done := make(chan bool, 2)

				urls := make([]string, 100)
				for i := 0; i < 100; i++ {
					urls[i] = "https://example.com/article" + string(rune(i))
				}

				// First goroutine checks URLs
				go func() {
					defer func() { done <- true }()
					for _, url := range urls {
						cache.IsDuplicate(url)
					}
				}()

				// Second goroutine checks same URLs
				go func() {
					defer func() { done <- true }()
					for _, url := range urls {
						cache.IsDuplicate(url)
					}
				}()

				// Wait for both goroutines
				<-done
				<-done

				// All URLs should be cached
				assert.Equal(t, len(urls), cache.Size())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestFeedCache(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create feed cache",
			test: func(t *testing.T) {
				cache := NewFeedCache(100, 30*time.Minute)
				assert.NotNil(t, cache)
			},
		},
		{
			name: "should cache and retrieve feed data",
			test: func(t *testing.T) {
				cache := NewFeedCache(100, 30*time.Minute)

				feedURL := "https://example.com/feed.xml"
				feedData := &CachedFeed{
					URL:     feedURL,
					Content: "<rss><channel><title>Test Feed</title></channel></rss>",
					ETag:    "etag123",
				}

				cache.SetFeed(feedURL, feedData)

				retrieved := cache.GetFeed(feedURL)
				require.NotNil(t, retrieved)
				assert.Equal(t, feedData.URL, retrieved.URL)
				assert.Equal(t, feedData.Content, retrieved.Content)
				assert.Equal(t, feedData.ETag, retrieved.ETag)
			},
		},
		{
			name: "should return nil for non-existent feed",
			test: func(t *testing.T) {
				cache := NewFeedCache(100, 30*time.Minute)

				retrieved := cache.GetFeed("https://nonexistent.com/feed.xml")
				assert.Nil(t, retrieved)
			},
		},
		{
			name: "should handle feed expiration",
			test: func(t *testing.T) {
				cache := NewFeedCache(100, 10*time.Millisecond)

				feedURL := "https://example.com/feed.xml"
				feedData := &CachedFeed{
					URL:     feedURL,
					Content: "<rss><channel><title>Test Feed</title></channel></rss>",
				}

				cache.SetFeed(feedURL, feedData)

				// Should exist initially
				retrieved := cache.GetFeed(feedURL)
				assert.NotNil(t, retrieved)

				// Wait for expiration
				time.Sleep(20 * time.Millisecond)

				// Should be expired
				retrieved = cache.GetFeed(feedURL)
				assert.Nil(t, retrieved)
			},
		},
		{
			name: "should check if feed is fresh",
			test: func(t *testing.T) {
				cache := NewFeedCache(100, 30*time.Minute)

				feedURL := "https://example.com/feed.xml"
				feedData := &CachedFeed{
					URL:     feedURL,
					Content: "<rss><channel><title>Test Feed</title></channel></rss>",
					ETag:    "etag123",
				}

				cache.SetFeed(feedURL, feedData)

				// Should be fresh
				isFresh := cache.IsFeedFresh(feedURL, "etag123")
				assert.True(t, isFresh)

				// Different ETag should not be fresh
				isFresh = cache.IsFeedFresh(feedURL, "different-etag")
				assert.False(t, isFresh)

				// Non-existent feed should not be fresh
				isFresh = cache.IsFeedFresh("https://nonexistent.com/feed.xml", "etag123")
				assert.False(t, isFresh)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestCacheManager(t *testing.T) {
	t.Run("should create cache manager with all cache types", func(t *testing.T) {
		manager := NewCacheManager()
		assert.NotNil(t, manager)
		assert.NotNil(t, manager.LRU)
		assert.NotNil(t, manager.URLDedup)
		assert.NotNil(t, manager.FeedCache)
	})

	t.Run("should provide cache statistics", func(t *testing.T) {
		manager := NewCacheManager()

		stats := manager.GetCacheStats()
		assert.NotNil(t, stats)
		assert.GreaterOrEqual(t, stats.LRUSize, 0)
		assert.GreaterOrEqual(t, stats.URLDedupSize, 0)
		assert.GreaterOrEqual(t, stats.FeedCacheSize, 0)
	})

	t.Run("should start and stop metrics collection", func(t *testing.T) {
		manager := NewCacheManager()
		ctx := context.Background()

		err := manager.StartMetricsCollection(ctx)
		assert.NoError(t, err)

		manager.StopMetricsCollection()
	})

	t.Run("should clear all caches", func(t *testing.T) {
		manager := NewCacheManager()

		// Add some data to caches
		manager.LRU.Set("test", "value")
		manager.URLDedup.IsDuplicate("https://example.com/test")
		manager.FeedCache.SetFeed("https://example.com/feed.xml", &CachedFeed{
			URL:     "https://example.com/feed.xml",
			Content: "test content",
		})

		// Clear all caches
		manager.ClearAll()

		// Verify all caches are empty
		stats := manager.GetCacheStats()
		assert.Equal(t, 0, stats.LRUSize)
		assert.Equal(t, 0, stats.URLDedupSize)
		assert.Equal(t, 0, stats.FeedCacheSize)
	})
}
