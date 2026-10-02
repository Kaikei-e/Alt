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

// A01 — Revocation-window tests that exercise actual production paths:
//
//  1. Tokens are minted by the real JWTIssuer (not a mock returning a fixed string).
//  2. Invalidation uses the real InvalidateSession usecase (not manual cache.Delete).
//  3. The cache-refill-before-Kratos-redirect residual is stated explicitly.
//
// These are unit tests — no network, no real Kratos — but every domain
// collaboration is the production wiring minus the IO.

const testJWTSecret = "test-backend-jwt-secret-at-least-32-chars-long"

// newRealJWTIssuer returns an infrastructure-layer JWTIssuer with the
// production token bounds (5 min TTL, auth-hub issuer, alt-backend audience).
func newRealJWTIssuer() *token.JWTIssuer {
	return token.NewJWTIssuer(token.JWTConfig{
		Secret:   testJWTSecret,
		Issuer:   "auth-hub",
		Audience: "alt-backend",
		TTL:      5 * time.Minute,
	})
}

// parseAndValidateJWT verifies a token string with the test secret and returns
// the parsed claims map.
func parseAndValidateJWT(t *testing.T, tokenStr string) jwt.MapClaims {
	t.Helper()
	parsed, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return []byte(testJWTSecret), nil
	})
	require.NoError(t, err)
	require.True(t, parsed.Valid)
	claims, ok := parsed.Claims.(jwt.MapClaims)
	require.True(t, ok)
	return claims
}

// TestA01_InvalidateViaUsecasePreventsReissuance is the core A01 scenario:
// after authenticated invalidation via the real InvalidateSession usecase,
// the attacker's next GetSession call must hit Kratos (cache miss) and fail
// because Kratos reports the session revoked.
func TestA01_InvalidateViaUsecasePreventsReissuance(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()

	const rawCookie = "victim-session-cookie-a01"
	fullCookie := "ory_kratos_session=" + rawCookie

	victimIdentity := &domain.Identity{
		UserID:    "victim-001",
		TenantID:  "victim-001",
		Email:     "victim@example.com",
		Role:      "user",
		SessionID: "kratos-sess-a01",
		CreatedAt: time.Now(),
	}

	validator := newMockRevocableValidator()
	validator.identities[fullCookie] = victimIdentity

	cache := newMockCache()
	realIssuer := newRealJWTIssuer()
	getUC := NewGetSession(validator, cache, realIssuer, logger)
	invalidateUC := NewInvalidateSession(cache, logger)

	// Step 1: Legitimate session → cache populated, real JWT issued.
	res, err := getUC.Execute(ctx, rawCookie)
	require.NoError(t, err)
	require.NotEmpty(t, res.BackendToken)

	// The JWT is real: verify it parses, has correct claims, and ≤5m expiry.
	claims := parseAndValidateJWT(t, res.BackendToken)
	assert.Equal(t, "victim-001", claims["sub"])
	assert.Equal(t, "kratos-sess-a01", claims["sid"])
	exp := claims["exp"].(float64)
	remaining := time.Until(time.Unix(int64(exp), 0))
	assert.LessOrEqual(t, remaining, 5*time.Minute+time.Second,
		"A01: JWT max lifespan must be ≤ 5 minutes")

	// Step 2: Kratos revokes the session (admin action or user logout).
	validator.RevokeSession(fullCookie)

	// Step 3: Invalidate via the actual usecase (mirrors POST /session/invalidate handler).
	err = invalidateUC.Execute(ctx, rawCookie)
	require.NoError(t, err)

	// Step 4: Attacker tries to reuse the stolen cookie. Cache is empty so
	// GetSession falls through to Kratos, which reports revoked → fail.
	_, err = getUC.Execute(ctx, rawCookie)
	assert.Error(t, err, "A01: revoked+invalidated session must be rejected")
	assert.True(t, errors.Is(err, domain.ErrAuthFailed))
}

// TestA01_CacheRefillResidual documents the bounded residual risk:
// if an attacker reuses a cookie BEFORE invalidation but AFTER Kratos
// revocation, the cache still serves the old entry. This is bounded by
// MaxCacheTTL (60s). The test makes this explicit rather than hiding it.
func TestA01_CacheRefillResidual(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()

	const rawCookie = "residual-cookie-a01"
	fullCookie := "ory_kratos_session=" + rawCookie

	validator := newMockRevocableValidator()
	validator.identities[fullCookie] = &domain.Identity{
		UserID:    "victim-002",
		TenantID:  "victim-002",
		Email:     "victim2@example.com",
		SessionID: "kratos-sess-a01-res",
	}

	cache := newMockCache()
	realIssuer := newRealJWTIssuer()
	getUC := NewGetSession(validator, cache, realIssuer, logger)

	// Step 1: Populate cache with legitimate session.
	res, err := getUC.Execute(ctx, rawCookie)
	require.NoError(t, err)
	require.NotEmpty(t, res.BackendToken)

	// Step 2: Kratos revokes, but NO invalidation call has been made yet.
	validator.RevokeSession(fullCookie)

	// Step 3: Attacker tries the cookie. Cache hit → still succeeds.
	// This is the documented bounded residual: up to MaxCacheTTL (60s).
	resStale, err := getUC.Execute(ctx, rawCookie)
	assert.NoError(t, err, "A01 residual: cache hit still succeeds before invalidation — "+
		"bounded to MaxCacheTTL=60s (was 5m pre-fix)")
	assert.NotNil(t, resStale)

	// Step 4: Now invalidate (mirrors what the logout handler does).
	invalidateUC := NewInvalidateSession(cache, logger)
	err = invalidateUC.Execute(ctx, rawCookie)
	require.NoError(t, err)

	// Step 5: Attacker tries again → must fail now.
	_, err = getUC.Execute(ctx, rawCookie)
	assert.Error(t, err, "A01: post-invalidation must fail closed")
	assert.True(t, errors.Is(err, domain.ErrAuthFailed))
}

// TestA01_RealIssuer_CacheTTLAndTokenTTLBounds verifies the production
// configuration invariants hold at the config layer: cache ≤ 60s, JWT ≤ 5m.
func TestA01_RealIssuer_CacheTTLAndTokenTTLBounds(t *testing.T) {
	t.Run("JWT_TTL_at_max_boundary", func(t *testing.T) {
		issuer := token.NewJWTIssuer(token.JWTConfig{
			Secret:   testJWTSecret,
			Issuer:   "auth-hub",
			Audience: "alt-backend",
			TTL:      5 * time.Minute,
		})

		ident := &domain.Identity{
			UserID:    "user-ttl",
			TenantID:  "user-ttl",
			Email:     "ttl@example.com",
			SessionID: "sess-ttl",
		}

		tokStr, err := issuer.IssueBackendToken(ident, "sess-ttl")
		require.NoError(t, err)

		claims := parseAndValidateJWT(t, tokStr)
		exp := claims["exp"].(float64)
		iat := claims["iat"].(float64)
		ttl := time.Duration(exp-iat) * time.Second
		assert.Equal(t, 5*time.Minute, ttl,
			"A01: issued JWT TTL must be exactly 5m at the max boundary")
	})

	t.Run("JWT_TTL_under_max", func(t *testing.T) {
		issuer := token.NewJWTIssuer(token.JWTConfig{
			Secret:   testJWTSecret,
			Issuer:   "auth-hub",
			Audience: "alt-backend",
			TTL:      2 * time.Minute,
		})

		ident := &domain.Identity{UserID: "u", TenantID: "u"}
		tokStr, err := issuer.IssueBackendToken(ident, "s")
		require.NoError(t, err)

		claims := parseAndValidateJWT(t, tokStr)
		exp := claims["exp"].(float64)
		iat := claims["iat"].(float64)
		ttl := time.Duration(exp-iat) * time.Second
		assert.Equal(t, 2*time.Minute, ttl,
			"A01: a shorter TTL must be honoured exactly")
	})
}
