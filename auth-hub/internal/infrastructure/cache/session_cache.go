package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"auth-hub/internal/domain"
)

const DefaultMaxEntries = 10000

// hashKey hashes the raw session cookie using SHA-256 so raw Kratos session
// tokens are never stored in memory map keys (AUTH-04).
func hashKey(cookie string) string {
	sum := sha256.Sum256([]byte(cookie))
	return hex.EncodeToString(sum[:])
}

// cacheEntry represents a cached session with user identity information.
type cacheEntry struct {
	session   domain.CachedSession
	expiresAt time.Time
}

// SessionCache provides thread-safe in-memory session caching with TTL.
// Implements domain.SessionCache.
type SessionCache struct {
	mu         sync.RWMutex
	entries    map[string]*cacheEntry
	ttl        time.Duration
	maxEntries int
}

// NewSessionCache creates a new session cache with the specified TTL.
func NewSessionCache(ttl time.Duration) *SessionCache {
	c := &SessionCache{
		entries:    make(map[string]*cacheEntry),
		ttl:        ttl,
		maxEntries: DefaultMaxEntries,
	}
	go c.cleanupLoop()
	return c
}

// Get retrieves a cached session by session ID.
func (c *SessionCache) Get(sessionID string) (*domain.CachedSession, bool) {
	key := hashKey(sessionID)

	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, found := c.entries[key]
	if !found || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return &entry.session, true
}

// Set stores session data in the cache, enforcing a maxEntries cap.
func (c *SessionCache) Set(sessionID string, session domain.CachedSession) {
	key := hashKey(sessionID)
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	maxEntries := c.maxEntries
	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}

	if _, exists := c.entries[key]; !exists && len(c.entries) >= maxEntries {
		for k, entry := range c.entries {
			if now.After(entry.expiresAt) {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= maxEntries {
			for k := range c.entries {
				delete(c.entries, k)
				break
			}
		}
	}

	c.entries[key] = &cacheEntry{
		session:   session,
		expiresAt: now.Add(c.ttl),
	}
}

// Delete removes session data from the cache.
func (c *SessionCache) Delete(sessionID string) {
	key := hashKey(sessionID)

	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
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
