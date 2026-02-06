package usecase

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"auth-hub/internal/domain"

	"github.com/stretchr/testify/assert"
)

// mockValidator implements domain.SessionValidator for testing.
type mockValidator struct {
	identity *domain.Identity
	err      error
	called   bool
	cookie   string
}

func (m *mockValidator) ValidateSession(_ context.Context, cookie string) (*domain.Identity, error) {
	m.called = true
	m.cookie = cookie
	return m.identity, m.err
}

// mockCache implements domain.SessionCache for testing.
type mockCache struct {
	entries map[string]domain.CachedSession
}

func newMockCache() *mockCache {
	return &mockCache{entries: make(map[string]domain.CachedSession)}
}

func (m *mockCache) Get(sessionID string) (*domain.CachedSession, bool) {
	entry, found := m.entries[sessionID]
	if !found {
		return nil, false
	}
	return &entry, true
}

func (m *mockCache) Set(sessionID string, session domain.CachedSession) {
	m.entries[sessionID] = session
}

func TestValidateSession_CacheHit(t *testing.T) {
	cache := newMockCache()
	cache.Set("session-abc", domain.CachedSession{
		UserID:   "user-123",
		TenantID: "user-123",
		Email:    "test@example.com",
	})
	validator := &mockValidator{}
	logger := slog.Default()

	uc := NewValidateSession(validator, cache, logger)
	identity, err := uc.Execute(context.Background(), "session-abc")

	assert.NoError(t, err)
	assert.Equal(t, "user-123", identity.UserID)
	assert.Equal(t, "test@example.com", identity.Email)
	assert.Equal(t, "session-abc", identity.SessionID)
	assert.False(t, validator.called, "should not call Kratos on cache hit")
}

func TestValidateSession_CacheMiss(t *testing.T) {
	cache := newMockCache()
	validator := &mockValidator{
		identity: &domain.Identity{
			UserID: "user-456",
			Email:  "new@example.com",
		},
	}
	logger := slog.Default()

	uc := NewValidateSession(validator, cache, logger)
	identity, err := uc.Execute(context.Background(), "session-xyz")

	assert.NoError(t, err)
	assert.Equal(t, "user-456", identity.UserID)
	assert.Equal(t, "new@example.com", identity.Email)
	assert.Equal(t, "session-xyz", identity.SessionID)
	assert.True(t, validator.called)
	assert.Equal(t, "ory_kratos_session=session-xyz", validator.cookie)

	// Verify cache was populated
	cached, found := cache.Get("session-xyz")
	assert.True(t, found)
	assert.Equal(t, "user-456", cached.UserID)
}

func TestValidateSession_KratosError(t *testing.T) {
	cache := newMockCache()
	validator := &mockValidator{
		err: domain.ErrAuthFailed,
	}
	logger := slog.Default()

	uc := NewValidateSession(validator, cache, logger)
	identity, err := uc.Execute(context.Background(), "bad-session")

	assert.Nil(t, identity)
	assert.True(t, errors.Is(err, domain.ErrAuthFailed))
}
