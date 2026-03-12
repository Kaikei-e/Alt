package cache

import (
	"context"
	"fmt"
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
		c.refreshInBackground(key)
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

	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return zero, CacheStateMissing
	}

	age := c.now().Sub(entry.storedAt)
	if age <= c.ttl {
		return entry.value, CacheStateFresh
	}
	if age <= c.ttl+c.staleTTL {
		return entry.value, CacheStateStale
	}
	return zero, CacheStateExpired
}

func (c *SharedCache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = &cacheEntry[V]{
		value:    value,
		storedAt: c.now(),
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

func (c *SharedCache[K, V]) refreshInBackground(key K) {
	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return
	}
	if !atomic.CompareAndSwapInt32(&entry.refreshing, 0, 1) {
		return
	}

	go func() {
		defer atomic.StoreInt32(&entry.refreshing, 0)
		_, _ = c.loadAndStore(context.Background(), key)
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
