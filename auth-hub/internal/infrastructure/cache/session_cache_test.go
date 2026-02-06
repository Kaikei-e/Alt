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
	c := NewSessionCache(100 * time.Millisecond)

	c.Set("sess-exp", domain.CachedSession{UserID: "user-1"})

	// Before expiry
	got, found := c.Get("sess-exp")
	assert.True(t, found)
	assert.Equal(t, "user-1", got.UserID)

	// After expiry
	time.Sleep(150 * time.Millisecond)
	got, found = c.Get("sess-exp")
	assert.False(t, found)
	assert.Nil(t, got)
}
