package cache

import (
	"sync"
	"time"

	"auth-hub/internal/domain"
)

// cacheEntry represents a cached session with user identity information.
type cacheEntry struct {
	session   domain.CachedSession
	expiresAt time.Time
}

// SessionCache provides thread-safe in-memory session caching with TTL.
// Implements domain.SessionCache.
type SessionCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	ttl     time.Duration
}

// NewSessionCache creates a new session cache with the specified TTL.
func NewSessionCache(ttl time.Duration) *SessionCache {
	c := &SessionCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}
	go c.cleanupLoop()
	return c
}

// Get retrieves a cached session by session ID.
func (c *SessionCache) Get(sessionID string) (*domain.CachedSession, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, found := c.entries[sessionID]
	if !found || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return &entry.session, true
}

// Set stores session data in the cache.
func (c *SessionCache) Set(sessionID string, session domain.CachedSession) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[sessionID] = &cacheEntry{
		session:   session,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// cleanup removes expired entries.
func (c *SessionCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for id, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, id)
		}
	}
}

// cleanupLoop runs periodic cleanup of expired entries.
func (c *SessionCache) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}
