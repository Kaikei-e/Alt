package cache

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

type CacheState int

const (
	CacheStateMissing CacheState = iota
	CacheStateFresh
	CacheStateStale
	CacheStateExpired

	maxExpiredSweep = 1024
)

// SharedCache is a generic in-memory cache with stale-while-revalidate.
type SharedCache[K comparable, V any] struct {
	mu       sync.RWMutex
	items    map[K]*cacheEntry[V]
	ttl      time.Duration
	staleTTL time.Duration
	sf       singleflight.Group
	loader   func(ctx context.Context, key K) (V, error)
	now      func() time.Time
}

type cacheEntry[V any] struct {
	value      V
	storedAt   time.Time
	version    int64
	refreshing int32
}

func NewSharedCache[K comparable, V any](
	ttl time.Duration,
	staleTTL time.Duration,
	loader func(ctx context.Context, key K) (V, error),
) *SharedCache[K, V] {
	return &SharedCache[K, V]{
		items:    make(map[K]*cacheEntry[V]),
		ttl:      ttl,
		staleTTL: staleTTL,
		loader:   loader,
		now:      time.Now,
	}
}

func (c *SharedCache[K, V]) Get(ctx context.Context, key K) (V, error) {
	value, state := c.Peek(key)
	switch state {
	case CacheStateFresh:
		return value, nil
	case CacheStateStale:
		c.RefreshInBackground(ctx, key)
		return value, nil
	default:
		return c.loadAndStore(ctx, key)
	}
}

func (c *SharedCache[K, V]) Refresh(ctx context.Context, key K) (V, error) {
	return c.loadAndStore(ctx, key)
}

func (c *SharedCache[K, V]) Peek(key K) (V, CacheState) {
	var zero V

	now := c.now()
	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return zero, CacheStateMissing
	}

	age := now.Sub(entry.storedAt)
	if age <= c.ttl {
		return entry.value, CacheStateFresh
	}
	if age <= c.ttl+c.staleTTL {
		return entry.value, CacheStateStale
	}

	c.mu.Lock()
	if current, exists := c.items[key]; exists && now.Sub(current.storedAt) > c.ttl+c.staleTTL && atomic.LoadInt32(&current.refreshing) == 0 {
		delete(c.items, key)
	}
	c.evictExpiredLocked(now)
	c.mu.Unlock()
	return zero, CacheStateExpired
}

func (c *SharedCache[K, V]) Set(key K, value V) {
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()

	c.evictExpiredLocked(now)
	c.items[key] = &cacheEntry[V]{
		value:    value,
		storedAt: now,
	}
}

func (c *SharedCache[K, V]) evictExpiredLocked(now time.Time) {
	swept := 0
	maxAge := c.ttl + c.staleTTL
	for k, entry := range c.items {
		if swept >= maxExpiredSweep {
			break
		}
		swept++
		if now.Sub(entry.storedAt) > maxAge && atomic.LoadInt32(&entry.refreshing) == 0 {
			delete(c.items, k)
		}
	}
}

func (c *SharedCache[K, V]) Invalidate(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

func (c *SharedCache[K, V]) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[K]*cacheEntry[V])
}

func (c *SharedCache[K, V]) RefreshInBackground(ctx context.Context, key K) {
	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok || c.now().Sub(entry.storedAt) <= c.ttl {
		return
	}
	if !atomic.CompareAndSwapInt32(&entry.refreshing, 0, 1) {
		return
	}

	go func() {
		defer atomic.StoreInt32(&entry.refreshing, 0)
		refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if _, err := c.loadAndStore(refreshCtx, key); err != nil {
			slog.Default().Error("shared cache background refresh failed",
				"key", fmt.Sprintf("%v", key),
				"error", err,
			)
		}
	}()
}

func (c *SharedCache[K, V]) loadAndStore(ctx context.Context, key K) (V, error) {
	var zero V
	if c.loader == nil {
		return zero, nil
	}

	cacheKey := singleflightKey(key)
	result, err, _ := c.sf.Do(cacheKey, func() (interface{}, error) {
		value, loadErr := c.loader(ctx, key)
		if loadErr != nil {
			return nil, loadErr
		}
		c.Set(key, value)
		return value, nil
	})
	if err != nil {
		return zero, err
	}

	return result.(V), nil
}

func singleflightKey[K comparable](key K) string {
	return fmt.Sprintf("%v", key)
}
