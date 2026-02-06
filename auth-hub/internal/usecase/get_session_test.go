package usecase

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"auth-hub/internal/domain"

	"github.com/stretchr/testify/assert"
)

// mockTokenIssuer implements domain.TokenIssuer for testing.
type mockTokenIssuer struct {
	token string
	err   error
}

func (m *mockTokenIssuer) IssueBackendToken(_ *domain.Identity, _ string) (string, error) {
	return m.token, m.err
}

func TestGetSession_CacheHit(t *testing.T) {
	cache := newMockCache()
	cache.Set("session-abc", domain.CachedSession{
		UserID:   "user-123",
		TenantID: "tenant-123",
		Email:    "test@example.com",
	})
	validator := &mockValidator{}
	tokenIssuer := &mockTokenIssuer{token: "jwt-token-123"}
	logger := slog.Default()

	uc := NewGetSession(validator, cache, tokenIssuer, logger)
	result, err := uc.Execute(context.Background(), "session-abc")

	assert.NoError(t, err)
	assert.Equal(t, "user-123", result.UserID)
	assert.Equal(t, "tenant-123", result.TenantID)
	assert.Equal(t, "test@example.com", result.Email)
	assert.Equal(t, "user", result.Role)
	assert.Equal(t, "session-abc", result.SessionID)
	assert.Equal(t, "jwt-token-123", result.BackendToken)
	assert.False(t, validator.called)
}

func TestGetSession_CacheMiss(t *testing.T) {
	cache := newMockCache()
	validator := &mockValidator{
		identity: &domain.Identity{
			UserID: "user-456",
			Email:  "new@example.com",
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
	assert.True(t, validator.called)

	// Verify cache was populated
	cached, found := cache.Get("session-xyz")
	assert.True(t, found)
	assert.Equal(t, "user-456", cached.UserID)
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
