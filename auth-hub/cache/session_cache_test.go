package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewSessionCache(t *testing.T) {
	t.Run("creates cache with specified TTL", func(t *testing.T) {
		ttl := 5 * time.Minute
		cache := NewSessionCache(ttl)

		assert.NotNil(t, cache)
		assert.Equal(t, ttl, cache.ttl)
	})
}

func TestSessionCache_SetAndGet(t *testing.T) {
	t.Run("set and get session successfully", func(t *testing.T) {
		cache := NewSessionCache(5 * time.Minute)

		sessionID := "session-123"
		userID := "user-456"
		tenantID := "tenant-789"
		email := "user@example.com"

		cache.Set(sessionID, userID, tenantID, email)

		entry, found := cache.Get(sessionID)
		assert.True(t, found)
		assert.NotNil(t, entry)
		assert.Equal(t, userID, entry.UserID)
		assert.Equal(t, tenantID, entry.TenantID)
		assert.Equal(t, email, entry.Email)
	})

	t.Run("get non-existent session returns not found", func(t *testing.T) {
		cache := NewSessionCache(5 * time.Minute)

		entry, found := cache.Get("non-existent")
		assert.False(t, found)
		assert.Nil(t, entry)
	})

	t.Run("overwrite existing session", func(t *testing.T) {
		cache := NewSessionCache(5 * time.Minute)

		sessionID := "session-123"
		cache.Set(sessionID, "user-1", "tenant-1", "email1@example.com")
		cache.Set(sessionID, "user-2", "tenant-2", "email2@example.com")

		entry, found := cache.Get(sessionID)
		assert.True(t, found)
		assert.Equal(t, "user-2", entry.UserID)
		assert.Equal(t, "tenant-2", entry.TenantID)
		assert.Equal(t, "email2@example.com", entry.Email)
	})
}

func TestSessionCache_TTLExpiration(t *testing.T) {
	t.Run("expired session not found", func(t *testing.T) {
		cache := NewSessionCache(100 * time.Millisecond)

		sessionID := "session-123"
		cache.Set(sessionID, "user-456", "tenant-789", "user@example.com")

		// Verify it exists initially
		entry, found := cache.Get(sessionID)
		assert.True(t, found)
		assert.NotNil(t, entry)

		// Wait for expiration
		time.Sleep(150 * time.Millisecond)

		// Should not be found after expiration
		entry, found = cache.Get(sessionID)
		assert.False(t, found)
		assert.Nil(t, entry)
	})

	t.Run("session accessed before expiration remains valid", func(t *testing.T) {
		cache := NewSessionCache(200 * time.Millisecond)

		sessionID := "session-123"
		cache.Set(sessionID, "user-456", "tenant-789", "user@example.com")

		// Access before expiration
		time.Sleep(100 * time.Millisecond)
		entry, found := cache.Get(sessionID)
		assert.True(t, found)
		assert.NotNil(t, entry)

		// Still accessible just before expiration
		time.Sleep(50 * time.Millisecond)
		entry, found = cache.Get(sessionID)
		assert.True(t, found)
		assert.NotNil(t, entry)

		// Expired after TTL
		time.Sleep(100 * time.Millisecond)
		entry, found = cache.Get(sessionID)
		assert.False(t, found)
		assert.Nil(t, entry)
	})
}

func TestSessionCache_MultipleEntries(t *testing.T) {
	t.Run("store and retrieve multiple sessions", func(t *testing.T) {
		cache := NewSessionCache(5 * time.Minute)

		sessions := []struct {
			sessionID string
			userID    string
			tenantID  string
			email     string
		}{
			{"session-1", "user-1", "tenant-1", "user1@example.com"},
			{"session-2", "user-2", "tenant-2", "user2@example.com"},
			{"session-3", "user-3", "tenant-3", "user3@example.com"},
		}

		// Set all sessions
		for _, s := range sessions {
			cache.Set(s.sessionID, s.userID, s.tenantID, s.email)
		}

		// Verify all sessions exist
		for _, s := range sessions {
			entry, found := cache.Get(s.sessionID)
			assert.True(t, found, "session %s should be found", s.sessionID)
			assert.Equal(t, s.userID, entry.UserID)
			assert.Equal(t, s.tenantID, entry.TenantID)
			assert.Equal(t, s.email, entry.Email)
		}
	})
}

func TestSessionCache_Cleanup(t *testing.T) {
	t.Run("cleanup removes expired entries", func(t *testing.T) {
		cache := NewSessionCache(100 * time.Millisecond)

		// Add multiple sessions
		cache.Set("session-1", "user-1", "tenant-1", "email1@example.com")
		cache.Set("session-2", "user-2", "tenant-2", "email2@example.com")

		// Wait for expiration
		time.Sleep(150 * time.Millisecond)

		// Manually trigger cleanup
		cache.cleanup()

		// Verify entries are removed from internal map
		cache.mu.RLock()
		entryCount := len(cache.entries)
		cache.mu.RUnlock()

		assert.Equal(t, 0, entryCount, "all expired entries should be removed")
	})

	t.Run("cleanup preserves non-expired entries", func(t *testing.T) {
		cache := NewSessionCache(1 * time.Second)

		// Add first session
		cache.Set("session-old", "user-old", "tenant-old", "old@example.com")

		// Wait a bit
		time.Sleep(200 * time.Millisecond)

		// Add new session
		cache.Set("session-new", "user-new", "tenant-new", "new@example.com")

		// Wait for old session to expire
		time.Sleep(900 * time.Millisecond)

		// Cleanup
		cache.cleanup()

		// Old session should be gone
		_, found := cache.Get("session-old")
		assert.False(t, found)

		// New session should still exist
		entry, found := cache.Get("session-new")
		assert.True(t, found)
		assert.Equal(t, "user-new", entry.UserID)
	})
}

func TestSessionCache_ConcurrentAccess(t *testing.T) {
	t.Run("concurrent reads and writes are safe", func(t *testing.T) {
		cache := NewSessionCache(5 * time.Minute)

		done := make(chan bool)
		sessions := 100

		// Concurrent writes
		go func() {
			for i := 0; i < sessions; i++ {
				sessionID := "session-" + string(rune(i))
				userID := "user-" + string(rune(i))
				cache.Set(sessionID, userID, userID, userID+"@example.com")
			}
			done <- true
		}()

		// Concurrent reads
		go func() {
			for i := 0; i < sessions; i++ {
				sessionID := "session-" + string(rune(i))
				cache.Get(sessionID)
			}
			done <- true
		}()

		// Wait for both goroutines
		<-done
		<-done

		// Should not panic (test passes if we reach here)
	})
}

func TestSessionCache_EmptyValues(t *testing.T) {
	t.Run("handles empty email", func(t *testing.T) {
		cache := NewSessionCache(5 * time.Minute)

		cache.Set("session-123", "user-456", "tenant-789", "")

		entry, found := cache.Get("session-123")
		assert.True(t, found)
		assert.Equal(t, "user-456", entry.UserID)
		assert.Equal(t, "", entry.Email)
	})
}

func BenchmarkSessionCache_Set(b *testing.B) {
	cache := NewSessionCache(5 * time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sessionID := "session-bench"
		cache.Set(sessionID, "user-123", "tenant-456", "bench@example.com")
	}
}

func BenchmarkSessionCache_Get(b *testing.B) {
	cache := NewSessionCache(5 * time.Minute)
	cache.Set("session-bench", "user-123", "tenant-456", "bench@example.com")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get("session-bench")
	}
}

func BenchmarkSessionCache_ConcurrentAccess(b *testing.B) {
	cache := NewSessionCache(5 * time.Minute)

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				cache.Set("session-bench", "user-123", "tenant-456", "bench@example.com")
			} else {
				cache.Get("session-bench")
			}
			i++
		}
	})
}
