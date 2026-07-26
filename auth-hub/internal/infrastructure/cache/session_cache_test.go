package cache

import (
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
	c.entries["sess-exp"].expiresAt = time.Now().Add(-time.Second)
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
	c.entries["sess-not-yet"].expiresAt = time.Now().Add(time.Hour)
	c.mu.Unlock()

	got, found := c.Get("sess-not-yet")
	assert.True(t, found, "entry with a future expiresAt must still be valid")
	assert.NotNil(t, got)
}
