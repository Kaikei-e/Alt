package cache

import (
	"sync"
	"time"
)

// cacheEntry represents a cached session with user identity information
type cacheEntry struct {
	UserID    string
	TenantID  string
	Email     string
	ExpiresAt time.Time
}

// SessionCache provides thread-safe in-memory session caching with TTL
type SessionCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	ttl     time.Duration
}

// NewSessionCache creates a new session cache with the specified TTL
func NewSessionCache(ttl time.Duration) *SessionCache {
	cache := &SessionCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}

	// Start background cleanup goroutine
	go cache.cleanupLoop()

	return cache
}

// Set stores session identity information in the cache
func (c *SessionCache) Set(sessionID, userID, tenantID, email string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[sessionID] = &cacheEntry{
		UserID:    userID,
		TenantID:  tenantID,
		Email:     email,
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

// Get retrieves session identity information from the cache
// Returns nil and false if the session is not found or has expired
func (c *SessionCache) Get(sessionID string) (*cacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, found := c.entries[sessionID]
	if !found {
		return nil, false
	}

	// Check if entry has expired
	if time.Now().After(entry.ExpiresAt) {
		return nil, false
	}

	return entry, true
}

// cleanup removes expired entries from the cache
// This method is called by cleanupLoop and can be called manually in tests
func (c *SessionCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for sessionID, entry := range c.entries {
		if now.After(entry.ExpiresAt) {
			delete(c.entries, sessionID)
		}
	}
}

// cleanupLoop runs periodic cleanup of expired entries
func (c *SessionCache) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}
