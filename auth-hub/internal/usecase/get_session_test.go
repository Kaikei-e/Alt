package usecase

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"auth-hub/internal/domain"

	"github.com/stretchr/testify/assert"
)

// mockTokenIssuer implements domain.TokenIssuer for testing.
type mockTokenIssuer struct {
	token           string
	err             error
	issuedSessionID string
}

func (m *mockTokenIssuer) IssueBackendToken(_ *domain.Identity, sessionID string) (string, error) {
	m.issuedSessionID = sessionID
	return m.token, m.err
}

func TestGetSession_CacheHit(t *testing.T) {
	createdAt := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	cache := newMockCache()
	cache.Set("cookie-abc", domain.CachedSession{
		UserID:    "user-123",
		TenantID:  "tenant-123",
		Email:     "test@example.com",
		SessionID: "session-abc",
		CreatedAt: createdAt,
	})
	validator := &mockValidator{}
	tokenIssuer := &mockTokenIssuer{token: "jwt-token-123"}
	logger := slog.Default()

	uc := NewGetSession(validator, cache, tokenIssuer, logger)
	result, err := uc.Execute(context.Background(), "cookie-abc")

	assert.NoError(t, err)
	assert.Equal(t, "user-123", result.UserID)
	assert.Equal(t, "tenant-123", result.TenantID)
	assert.Equal(t, "test@example.com", result.Email)
	assert.Equal(t, "user", result.Role)
	assert.Equal(t, "session-abc", result.SessionID)
	assert.Equal(t, "jwt-token-123", result.BackendToken)
	assert.Equal(t, createdAt, result.CreatedAt)
	assert.False(t, validator.called)
}

func TestGetSession_CacheMiss(t *testing.T) {
	createdAt := time.Date(2025, 1, 10, 8, 30, 0, 0, time.UTC)
	cache := newMockCache()
	validator := &mockValidator{
		identity: &domain.Identity{
			UserID:    "user-456",
			Email:     "new@example.com",
			CreatedAt: createdAt,
		},
	}
	tokenIssuer := &mockTokenIssuer{token: "jwt-new-token"}
	logger := slog.Default()

	uc := NewGetSession(validator, cache, tokenIssuer, logger)
	result, err := uc.Execute(context.Background(), "session-xyz")

	assert.NoError(t, err)
	assert.Equal(t, "user-456", result.UserID)
	assert.Equal(t, "user-456", result.TenantID) // Single-tenant
	assert.Equal(t, "jwt-new-token", result.BackendToken)
	assert.Equal(t, createdAt, result.CreatedAt)
	assert.True(t, validator.called)

	// Verify cache was populated with CreatedAt for accurate cache-hit returns
	cached, found := cache.Get("session-xyz")
	assert.True(t, found)
	assert.Equal(t, "user-456", cached.UserID)
	assert.Equal(t, createdAt, cached.CreatedAt)
}

func TestGetSession_KratosError(t *testing.T) {
	cache := newMockCache()
	validator := &mockValidator{err: domain.ErrAuthFailed}
	tokenIssuer := &mockTokenIssuer{token: "unused"}
	logger := slog.Default()

	uc := NewGetSession(validator, cache, tokenIssuer, logger)
	result, err := uc.Execute(context.Background(), "bad-session")

	assert.Nil(t, result)
	assert.True(t, errors.Is(err, domain.ErrAuthFailed))
}

func TestGetSession_AdminRole_CacheMiss(t *testing.T) {
	cache := newMockCache()
	validator := &mockValidator{
		identity: &domain.Identity{
			UserID: "admin-001",
			Email:  "admin@example.com",
			Role:   "admin",
		},
	}
	tokenIssuer := &mockTokenIssuer{token: "jwt-admin-token"}
	logger := slog.Default()

	uc := NewGetSession(validator, cache, tokenIssuer, logger)
	result, err := uc.Execute(context.Background(), "admin-session")

	assert.NoError(t, err)
	assert.Equal(t, "admin", result.Role)
	assert.Equal(t, "jwt-admin-token", result.BackendToken)

	// Verify cache was populated with role
	cached, found := cache.Get("admin-session")
	assert.True(t, found)
	assert.Equal(t, "admin", cached.Role)
}

func TestGetSession_AdminRole_CacheHit(t *testing.T) {
	cache := newMockCache()
	cache.Set("admin-session", domain.CachedSession{
		UserID:   "admin-001",
		TenantID: "admin-001",
		Email:    "admin@example.com",
		Role:     "admin",
	})
	validator := &mockValidator{}
	tokenIssuer := &mockTokenIssuer{token: "jwt-admin-cached"}
	logger := slog.Default()

	uc := NewGetSession(validator, cache, tokenIssuer, logger)
	result, err := uc.Execute(context.Background(), "admin-session")

	assert.NoError(t, err)
	assert.Equal(t, "admin", result.Role)
	assert.False(t, validator.called)
}

func TestGetSession_EmptyRole_DefaultsToUser(t *testing.T) {
	cache := newMockCache()
	validator := &mockValidator{
		identity: &domain.Identity{
			UserID: "user-789",
			Email:  "norol@example.com",
			Role:   "",
		},
	}
	tokenIssuer := &mockTokenIssuer{token: "jwt-default-role"}
	logger := slog.Default()

	uc := NewGetSession(validator, cache, tokenIssuer, logger)
	result, err := uc.Execute(context.Background(), "session-norole")

	assert.NoError(t, err)
	assert.Equal(t, "user", result.Role)
}

// TestGetSession_CacheMiss_SessionIDIsKratosSessionID pins the same invariant
// as the /validate path for /session: the value that becomes the JWT sid claim
// and the session.id field of the JSON body is the stable Kratos session id,
// never the raw ory_kratos_session cookie.
func TestGetSession_CacheMiss_SessionIDIsKratosSessionID(t *testing.T) {
	const rawCookie = "MTc0N2E5ZTQtcmF3LWNvb2tpZS1zZWNyZXQ"

	cache := newMockCache()
	validator := &mockValidator{
		identity: &domain.Identity{
			UserID:    "user-456",
			Email:     "new@example.com",
			SessionID: "9f3c1b2a-0000-4d5e-8f1a-2b3c4d5e6f70",
		},
	}
	tokenIssuer := &mockTokenIssuer{token: "jwt-new-token"}
	logger := slog.Default()

	uc := NewGetSession(validator, cache, tokenIssuer, logger)
	result, err := uc.Execute(context.Background(), rawCookie)

	assert.NoError(t, err)
	assert.Equal(t, "9f3c1b2a-0000-4d5e-8f1a-2b3c4d5e6f70", result.SessionID)
	assert.NotEqual(t, rawCookie, result.SessionID, "the raw session cookie must never leave in the response body")
	assert.Equal(t, "9f3c1b2a-0000-4d5e-8f1a-2b3c4d5e6f70", tokenIssuer.issuedSessionID,
		"the sid claim must carry the Kratos session id, not the cookie")

	cached, found := cache.Get(rawCookie)
	assert.True(t, found)
	assert.Equal(t, "9f3c1b2a-0000-4d5e-8f1a-2b3c4d5e6f70", cached.SessionID)
}

// TestGetSession_CacheHit_SessionIDIsKratosSessionID is the cache-hit half of
// the same invariant.
func TestGetSession_CacheHit_SessionIDIsKratosSessionID(t *testing.T) {
	const rawCookie = "MTc0N2E5ZTQtcmF3LWNvb2tpZS1zZWNyZXQ"

	cache := newMockCache()
	cache.Set(rawCookie, domain.CachedSession{
		UserID:    "user-123",
		TenantID:  "tenant-123",
		Email:     "test@example.com",
		SessionID: "9f3c1b2a-0000-4d5e-8f1a-2b3c4d5e6f70",
	})
	validator := &mockValidator{}
	tokenIssuer := &mockTokenIssuer{token: "jwt-token-123"}
	logger := slog.Default()

	uc := NewGetSession(validator, cache, tokenIssuer, logger)
	result, err := uc.Execute(context.Background(), rawCookie)

	assert.NoError(t, err)
	assert.Equal(t, "9f3c1b2a-0000-4d5e-8f1a-2b3c4d5e6f70", result.SessionID)
	assert.NotEqual(t, rawCookie, result.SessionID, "the raw session cookie must never leave in the response body")
	assert.Equal(t, "9f3c1b2a-0000-4d5e-8f1a-2b3c4d5e6f70", tokenIssuer.issuedSessionID,
		"the sid claim must carry the Kratos session id, not the cookie")
	assert.False(t, validator.called)
}

func TestGetSession_TokenGenerationError(t *testing.T) {
	cache := newMockCache()
	cache.Set("session-abc", domain.CachedSession{
		UserID:   "user-123",
		TenantID: "tenant-123",
		Email:    "test@example.com",
	})
	validator := &mockValidator{}
	tokenIssuer := &mockTokenIssuer{err: errors.New("signing error")}
	logger := slog.Default()

	uc := NewGetSession(validator, cache, tokenIssuer, logger)
	result, err := uc.Execute(context.Background(), "session-abc")

	assert.Nil(t, result)
	assert.True(t, errors.Is(err, domain.ErrTokenGeneration))
}
