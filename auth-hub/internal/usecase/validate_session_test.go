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
	cache.Set("cookie-abc", domain.CachedSession{
		UserID:    "user-123",
		TenantID:  "user-123",
		Email:     "test@example.com",
		SessionID: "session-abc",
	})
	validator := &mockValidator{}
	logger := slog.Default()

	uc := NewValidateSession(validator, cache, logger)
	identity, err := uc.Execute(context.Background(), "cookie-abc")

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
			UserID:    "user-456",
			Email:     "new@example.com",
			SessionID: "session-xyz",
		},
	}
	logger := slog.Default()

	uc := NewValidateSession(validator, cache, logger)
	identity, err := uc.Execute(context.Background(), "cookie-xyz")

	assert.NoError(t, err)
	assert.Equal(t, "user-456", identity.UserID)
	assert.Equal(t, "new@example.com", identity.Email)
	assert.Equal(t, "session-xyz", identity.SessionID)
	assert.True(t, validator.called)
	assert.Equal(t, "ory_kratos_session=cookie-xyz", validator.cookie)

	// Verify cache was populated
	cached, found := cache.Get("cookie-xyz")
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

// TestValidateSession_CacheMiss_TenantIDFallback locks in the single-tenant
// fallback (TenantID == UserID) documented on Execute's doc comment, which the
// original cache-miss test never actually asserted.
func TestValidateSession_CacheMiss_TenantIDFallback(t *testing.T) {
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
	assert.Equal(t, "user-456", identity.TenantID, "single-tenant fallback: TenantID must equal UserID")
}

// TestValidateSession_AdminRole_CacheMiss verifies the Role granted by Kratos
// on a fresh validation survives into the returned Identity. This is the
// nginx /validate path (unlike GetSession) that issues the backend JWT, so a
// dropped Role here is a silent privilege downgrade for every downstream call.
func TestValidateSession_AdminRole_CacheMiss(t *testing.T) {
	cache := newMockCache()
	validator := &mockValidator{
		identity: &domain.Identity{
			UserID: "admin-001",
			Email:  "admin@example.com",
			Role:   "admin",
		},
	}
	logger := slog.Default()

	uc := NewValidateSession(validator, cache, logger)
	identity, err := uc.Execute(context.Background(), "admin-session")

	assert.NoError(t, err)
	assert.Equal(t, "admin", identity.Role, "Role from Kratos must be preserved on cache miss")

	// Verify the cache entry itself carries Role, otherwise the very next
	// request for this session (cache hit) silently downgrades the admin.
	cached, found := cache.Get("admin-session")
	assert.True(t, found)
	assert.Equal(t, "admin", cached.Role, "cached session must retain Role so cache hits don't downgrade privilege")
}

// TestValidateSession_CacheMiss_SessionIDIsKratosSessionID pins that the
// SessionID travelling into the sid claim of the 30-minute backend JWT is the
// stable Kratos session id, never the raw ory_kratos_session cookie. JWT
// payloads are base64url, not encrypted, so echoing the cookie there turns any
// captured token into the long-lived bearer credential itself.
func TestValidateSession_CacheMiss_SessionIDIsKratosSessionID(t *testing.T) {
	const rawCookie = "MTc0N2E5ZTQtcmF3LWNvb2tpZS1zZWNyZXQ"

	cache := newMockCache()
	validator := &mockValidator{
		identity: &domain.Identity{
			UserID:    "user-456",
			Email:     "new@example.com",
			SessionID: "9f3c1b2a-0000-4d5e-8f1a-2b3c4d5e6f70",
		},
	}
	logger := slog.Default()

	uc := NewValidateSession(validator, cache, logger)
	identity, err := uc.Execute(context.Background(), rawCookie)

	assert.NoError(t, err)
	assert.Equal(t, "9f3c1b2a-0000-4d5e-8f1a-2b3c4d5e6f70", identity.SessionID)
	assert.NotEqual(t, rawCookie, identity.SessionID, "the raw session cookie must never leave as SessionID")

	cached, found := cache.Get(rawCookie)
	assert.True(t, found)
	assert.Equal(t, "9f3c1b2a-0000-4d5e-8f1a-2b3c4d5e6f70", cached.SessionID,
		"cache must carry the Kratos session id so cache hits do not fall back to the cookie")
}

// TestValidateSession_CacheHit_SessionIDIsKratosSessionID is the cache-hit half
// of the same invariant: a warm cache must not reintroduce the cookie value.
func TestValidateSession_CacheHit_SessionIDIsKratosSessionID(t *testing.T) {
	const rawCookie = "MTc0N2E5ZTQtcmF3LWNvb2tpZS1zZWNyZXQ"

	cache := newMockCache()
	cache.Set(rawCookie, domain.CachedSession{
		UserID:    "user-123",
		TenantID:  "user-123",
		Email:     "test@example.com",
		SessionID: "9f3c1b2a-0000-4d5e-8f1a-2b3c4d5e6f70",
	})
	validator := &mockValidator{}
	logger := slog.Default()

	uc := NewValidateSession(validator, cache, logger)
	identity, err := uc.Execute(context.Background(), rawCookie)

	assert.NoError(t, err)
	assert.Equal(t, "9f3c1b2a-0000-4d5e-8f1a-2b3c4d5e6f70", identity.SessionID)
	assert.NotEqual(t, rawCookie, identity.SessionID, "the raw session cookie must never leave as SessionID")
	assert.False(t, validator.called, "should not call Kratos on cache hit")
}

// TestValidateSession_AdminRole_CacheHit is the regression test for the bug
// exposed by TestValidateSession_AdminRole_CacheMiss: once a session is
// cached, subsequent /validate calls within the cache TTL must still report
// the admin's Role, not silently fall back to an empty/default role.
func TestValidateSession_AdminRole_CacheHit(t *testing.T) {
	cache := newMockCache()
	cache.Set("admin-session", domain.CachedSession{
		UserID:   "admin-001",
		TenantID: "admin-001",
		Email:    "admin@example.com",
		Role:     "admin",
	})
	validator := &mockValidator{}
	logger := slog.Default()

	uc := NewValidateSession(validator, cache, logger)
	identity, err := uc.Execute(context.Background(), "admin-session")

	assert.NoError(t, err)
	assert.Equal(t, "admin", identity.Role, "cache hit must not downgrade a cached admin's Role")
	assert.False(t, validator.called, "should not call Kratos on cache hit")
}
