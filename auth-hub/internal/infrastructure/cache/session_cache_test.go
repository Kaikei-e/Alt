package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"auth-hub/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestSessionCache_SetAndGet(t *testing.T) {
	c := NewSessionCache(5 * time.Minute)

	c.Set("sess-1", domain.CachedSession{
		UserID:   "user-1",
		TenantID: "tenant-1",
		Email:    "test@example.com",
	})

	got, found := c.Get("sess-1")
	assert.True(t, found)
	assert.Equal(t, "user-1", got.UserID)
	assert.Equal(t, "tenant-1", got.TenantID)
	assert.Equal(t, "test@example.com", got.Email)
}

func TestSessionCache_NotFound(t *testing.T) {
	c := NewSessionCache(5 * time.Minute)

	got, found := c.Get("nonexistent")
	assert.False(t, found)
	assert.Nil(t, got)
}

func TestSessionCache_Expiration(t *testing.T) {
	c := NewSessionCache(5 * time.Minute)

	c.Set("sess-exp", domain.CachedSession{UserID: "user-1"})

	// Before expiry
	got, found := c.Get("sess-exp")
	assert.True(t, found)
	assert.Equal(t, "user-1", got.UserID)

	// Deterministically force expiry by rewinding the entry's expiresAt into
	// the past, rather than sleeping past a real TTL: a sleep-based version of
	// this test is flaky under load (a slow CI runner can observe the entry
	// still valid, or the whole test can take real wall-clock time for no
	// reason). Direct field access is safe here because the test lives in the
	// same package as the unexported cacheEntry type.
	c.mu.Lock()
	c.entries[hashKey("sess-exp")].expiresAt = time.Now().Add(-time.Second)
	c.mu.Unlock()

	got, found = c.Get("sess-exp")
	assert.False(t, found)
	assert.Nil(t, got)
}

// TestSessionCache_NotYetExpired_StaysValid locks in that an entry whose
// deadline is still in the future (however close) remains valid, guarding
// against an off-by-one flip of Get's "After" comparison to something that
// would expire entries early.
func TestSessionCache_NotYetExpired_StaysValid(t *testing.T) {
	c := NewSessionCache(5 * time.Minute)
	c.Set("sess-not-yet", domain.CachedSession{UserID: "user-1"})

	c.mu.Lock()
	c.entries[hashKey("sess-not-yet")].expiresAt = time.Now().Add(time.Hour)
	c.mu.Unlock()

	got, found := c.Get("sess-not-yet")
	assert.True(t, found, "entry with a future expiresAt must still be valid")
	assert.NotNil(t, got)
}

func TestSessionCache_Delete(t *testing.T) {
	c := NewSessionCache(60 * time.Second)
	c.Set("sess-to-delete", domain.CachedSession{UserID: "user-1"})

	got, found := c.Get("sess-to-delete")
	assert.True(t, found)
	assert.NotNil(t, got)

	c.Delete("sess-to-delete")

	got, found = c.Get("sess-to-delete")
	assert.False(t, found)
	assert.Nil(t, got)
}

func TestSessionCache_SHA256KeyHashing(t *testing.T) {
	c := NewSessionCache(5 * time.Minute)
	rawToken := "raw-kratos-session-cookie-secret-token"

	c.Set(rawToken, domain.CachedSession{UserID: "user-hash"})

	sum := sha256.Sum256([]byte(rawToken))
	expectedHash := hex.EncodeToString(sum[:])
	assert.Equal(t, expectedHash, hashKey(rawToken))

	c.mu.RLock()
	_, rawExists := c.entries[rawToken]
	_, hashExists := c.entries[expectedHash]
	c.mu.RUnlock()

	assert.False(t, rawExists, "raw session cookie must never be stored as a map key")
	assert.True(t, hashExists, "SHA-256 digest of session cookie must be used as map key")
}

func TestSessionCache_MaxEntriesEviction(t *testing.T) {
	c := NewSessionCache(5 * time.Minute)
	assert.Equal(t, 10000, c.maxEntries, "default maxEntries must be 10000")

	c.maxEntries = 3

	c.Set("sess-1", domain.CachedSession{UserID: "user-1"})
	c.Set("sess-2", domain.CachedSession{UserID: "user-2"})
	c.Set("sess-3", domain.CachedSession{UserID: "user-3"})

	// Expire sess-1 so inserting sess-4 sweeps expired entries first
	c.mu.Lock()
	c.entries[hashKey("sess-1")].expiresAt = time.Now().Add(-time.Second)
	c.mu.Unlock()

	c.Set("sess-4", domain.CachedSession{UserID: "user-4"})

	c.mu.RLock()
	assert.Len(t, c.entries, 3, "sweeping expired entries must keep cache within maxEntries")
	c.mu.RUnlock()
	_, found1 := c.Get("sess-1")
	assert.False(t, found1, "expired sess-1 must have been swept")
	_, found4 := c.Get("sess-4")
	assert.True(t, found4, "new sess-4 must be present")

	// Now all 3 entries are unexpired; inserting more entries must evict one entry to stay at maxEntries
	for i := 5; i <= 10; i++ {
		c.Set(fmt.Sprintf("sess-%d", i), domain.CachedSession{UserID: fmt.Sprintf("user-%d", i)})
		c.mu.RLock()
		assert.LessOrEqual(t, len(c.entries), 3, "cache size must never exceed maxEntries")
		c.mu.RUnlock()
	}

	got10, found10 := c.Get("sess-10")
	assert.True(t, found10)
	assert.Equal(t, "user-10", got10.UserID)
}
