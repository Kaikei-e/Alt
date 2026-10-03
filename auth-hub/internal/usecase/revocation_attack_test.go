package usecase

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"auth-hub/internal/domain"
	"auth-hub/internal/infrastructure/token"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRevocableValidator allows simulating session revocation mid-test.
type mockRevocableValidator struct {
	revokedSessions map[string]bool
	identities      map[string]*domain.Identity
}

func newMockRevocableValidator() *mockRevocableValidator {
	return &mockRevocableValidator{
		revokedSessions: make(map[string]bool),
		identities:      make(map[string]*domain.Identity),
	}
}

func (m *mockRevocableValidator) ValidateSession(_ context.Context, cookie string) (*domain.Identity, error) {
	if m.revokedSessions[cookie] {
		return nil, domain.ErrAuthFailed
	}
	identity, ok := m.identities[cookie]
	if !ok {
		return nil, domain.ErrAuthFailed
	}
	return identity, nil
}

func (m *mockRevocableValidator) RevokeSession(cookie string) {
	m.revokedSessions[cookie] = true
}

func TestRevocationAttack_RevokedSessionRejectedWithinCacheTTL(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()

	rawCookie := "stolen-session-cookie-xyz"
	fullCookie := "ory_kratos_session=" + rawCookie
	userIdentity := &domain.Identity{
		UserID:    "victim-user-123",
		TenantID:  "victim-user-123",
		Email:     "victim@example.com",
		Role:      "user",
		SessionID: "kratos-sess-001",
		CreatedAt: time.Now(),
	}

	validator := newMockRevocableValidator()
	validator.identities[fullCookie] = userIdentity

	cache := newMockCache()
	tokenIssuer := &mockTokenIssuer{token: "jwt-token-active"}

	getUC := NewGetSession(validator, cache, tokenIssuer, logger)
	validateUC := NewValidateSession(validator, cache, logger)

	// Step 1: Initial successful validation (caches the session)
	res, err := getUC.Execute(ctx, rawCookie)
	require.NoError(t, err)
	assert.Equal(t, "victim-user-123", res.UserID)

	// Verify session is now cached
	cached, found := cache.Get(rawCookie)
	require.True(t, found)
	assert.Equal(t, "victim-user-123", cached.UserID)

	// Step 2: Legitimate user logs out / admin revokes session in Kratos
	validator.RevokeSession(fullCookie)

	// VULNERABILITY DEMONSTRATION (without invalidation):
	// An attacker would still get a valid response because of the cache:
	resStolen, err := getUC.Execute(ctx, rawCookie)
	assert.NoError(t, err, "Without cache invalidation, stolen cookie still succeeds from cache")
	assert.NotNil(t, resStolen)

	// Step 3: Authenticated logout invalidation is invoked on auth-hub
	// Using the real InvalidateSession usecase
	invalidateUC := NewInvalidateSession(cache, logger)
	err = invalidateUC.Execute(ctx, rawCookie)
	require.NoError(t, err)

	// Step 4: Attacker tries to use the revoked session within what would have been the cache TTL
	// Must fail-closed because cache was purged and Kratos reports session revoked!
	_, errAfterInvalidation := getUC.Execute(ctx, rawCookie)
	assert.Error(t, errAfterInvalidation, "Revoked session must be rejected after logout cache invalidation")
	assert.True(t, errors.Is(errAfterInvalidation, domain.ErrAuthFailed))

	// Also verify ValidateSession rejects the revoked cookie
	_, valErr := validateUC.Execute(ctx, rawCookie)
	assert.Error(t, valErr, "ValidateSession must reject revoked session after invalidation")
	assert.True(t, errors.Is(valErr, domain.ErrAuthFailed))
}

// TestBackendToken_MaxLifespanFiveMinutes verifies that issued backend JWTs
// have an expiry bounded to <= 5 minutes using the actual JWTIssuer.
func TestBackendToken_MaxLifespanFiveMinutes(t *testing.T) {
	signingSecret := "test-backend-jwt-secret-at-least-32-chars-long"
	issuer := token.NewJWTIssuer(token.JWTConfig{
		Secret:   signingSecret,
		Issuer:   "auth-hub",
		Audience: "alt-backend",
		TTL:      5 * time.Minute,
	})

	ident := &domain.Identity{
		UserID:   "user-123",
		TenantID: "user-123",
		Email:    "user@example.com",
		Role:     "user",
	}

	tokString, err := issuer.IssueBackendToken(ident, "kratos-sess-001")
	require.NoError(t, err)

	// Parse and validate the actual signed token
	parsedToken, err := jwt.Parse(tokString, func(t *jwt.Token) (interface{}, error) {
		return []byte(signingSecret), nil
	})
	require.NoError(t, err)
	require.True(t, parsedToken.Valid)

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	require.True(t, ok)

	exp, ok := claims["exp"].(float64)
	require.True(t, ok)

	remaining := time.Until(time.Unix(int64(exp), 0))
	assert.LessOrEqual(t, remaining, 5*time.Minute+time.Second, "JWT expiration must not exceed 5 minutes")
}
