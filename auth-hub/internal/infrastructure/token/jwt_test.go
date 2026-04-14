package token

import (
	"testing"
	"time"

	"auth-hub/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func TestJWTIssuer_IssueBackendToken(t *testing.T) {
	issuer := NewJWTIssuer(JWTConfig{
		Secret:   "this-is-a-valid-backend-token-secret-32-chars-long",
		Issuer:   "auth-hub",
		Audience: "alt-backend",
		TTL:      5 * time.Minute,
	})

	identity := &domain.Identity{
		UserID: "user-123",
		Email:  "test@example.com",
	}

	tokenStr, err := issuer.IssueBackendToken(identity, "session-abc")
	assert.NoError(t, err)
	assert.NotEmpty(t, tokenStr)

	// Parse and validate
	parsed, err := jwt.ParseWithClaims(tokenStr, &backendClaims{}, func(token *jwt.Token) (any, error) {
		return []byte("this-is-a-valid-backend-token-secret-32-chars-long"), nil
	})
	assert.NoError(t, err)
	assert.True(t, parsed.Valid)

	claims := parsed.Claims.(*backendClaims)
	assert.Equal(t, "user-123", claims.Subject)
	assert.Equal(t, "test@example.com", claims.Email)
	assert.Equal(t, "user", claims.Role)
	assert.Equal(t, "session-abc", claims.Sid)
	assert.Equal(t, "auth-hub", claims.Issuer)
	assert.Contains(t, claims.Audience, "alt-backend")
}

func TestJWTIssuer_AdminRole(t *testing.T) {
	issuer := NewJWTIssuer(JWTConfig{
		Secret:   "this-is-a-valid-backend-token-secret-32-chars-long",
		Issuer:   "auth-hub",
		Audience: "alt-backend",
		TTL:      5 * time.Minute,
	})

	identity := &domain.Identity{
		UserID: "admin-001",
		Email:  "admin@example.com",
		Role:   "admin",
	}

	tokenStr, err := issuer.IssueBackendToken(identity, "session-admin")
	assert.NoError(t, err)

	parsed, err := jwt.ParseWithClaims(tokenStr, &backendClaims{}, func(token *jwt.Token) (any, error) {
		return []byte("this-is-a-valid-backend-token-secret-32-chars-long"), nil
	})
	assert.NoError(t, err)

	claims := parsed.Claims.(*backendClaims)
	assert.Equal(t, "admin", claims.Role)
}

func TestJWTIssuer_EmptyRole_DefaultsToUser(t *testing.T) {
	issuer := NewJWTIssuer(JWTConfig{
		Secret:   "this-is-a-valid-backend-token-secret-32-chars-long",
		Issuer:   "auth-hub",
		Audience: "alt-backend",
		TTL:      5 * time.Minute,
	})

	identity := &domain.Identity{
		UserID: "user-123",
		Email:  "test@example.com",
		Role:   "",
	}

	tokenStr, err := issuer.IssueBackendToken(identity, "session-abc")
	assert.NoError(t, err)

	parsed, err := jwt.ParseWithClaims(tokenStr, &backendClaims{}, func(token *jwt.Token) (any, error) {
		return []byte("this-is-a-valid-backend-token-secret-32-chars-long"), nil
	})
	assert.NoError(t, err)

	claims := parsed.Claims.(*backendClaims)
	assert.Equal(t, "user", claims.Role)
}

func TestJWTIssuer_ExpiredToken(t *testing.T) {
	issuer := NewJWTIssuer(JWTConfig{
		Secret:   "this-is-a-valid-backend-token-secret-32-chars-long",
		Issuer:   "auth-hub",
		Audience: "alt-backend",
		TTL:      -1 * time.Minute, // Already expired
	})

	identity := &domain.Identity{
		UserID: "user-123",
		Email:  "test@example.com",
	}

	tokenStr, err := issuer.IssueBackendToken(identity, "session-abc")
	assert.NoError(t, err) // Generation succeeds

	// Parsing should fail due to expiration
	_, err = jwt.ParseWithClaims(tokenStr, &backendClaims{}, func(token *jwt.Token) (any, error) {
		return []byte("this-is-a-valid-backend-token-secret-32-chars-long"), nil
	})
	assert.Error(t, err)
}

// H-003a: tenant_id claim must be present in the issued JWT so that
// alt-backend can use it instead of falling back to Subject.
func TestJWTIssuer_IncludesTenantIDClaim(t *testing.T) {
	issuer := NewJWTIssuer(JWTConfig{
		Secret:   "this-is-a-valid-backend-token-secret-32-chars-long",
		Issuer:   "auth-hub",
		Audience: "alt-backend",
		TTL:      5 * time.Minute,
	})

	identity := &domain.Identity{
		UserID:   "user-123",
		TenantID: "tenant-999",
		Email:    "test@example.com",
	}

	tokenStr, err := issuer.IssueBackendToken(identity, "session-abc")
	assert.NoError(t, err)

	parsed, err := jwt.ParseWithClaims(tokenStr, &backendClaims{}, func(token *jwt.Token) (any, error) {
		return []byte("this-is-a-valid-backend-token-secret-32-chars-long"), nil
	})
	assert.NoError(t, err)

	claims := parsed.Claims.(*backendClaims)
	assert.Equal(t, "tenant-999", claims.TenantID,
		"JWT must carry the tenant_id claim taken from identity.TenantID")
}

// H-003a: when the caller omits TenantID, the issuer must default to UserID
// to preserve the existing single-tenant semantics (UserID == TenantID).
func TestJWTIssuer_TenantIDDefaultsToUserID(t *testing.T) {
	issuer := NewJWTIssuer(JWTConfig{
		Secret:   "this-is-a-valid-backend-token-secret-32-chars-long",
		Issuer:   "auth-hub",
		Audience: "alt-backend",
		TTL:      5 * time.Minute,
	})

	identity := &domain.Identity{
		UserID: "user-123",
		Email:  "test@example.com",
		// TenantID intentionally left empty
	}

	tokenStr, err := issuer.IssueBackendToken(identity, "session-abc")
	assert.NoError(t, err)

	parsed, err := jwt.ParseWithClaims(tokenStr, &backendClaims{}, func(token *jwt.Token) (any, error) {
		return []byte("this-is-a-valid-backend-token-secret-32-chars-long"), nil
	})
	assert.NoError(t, err)

	claims := parsed.Claims.(*backendClaims)
	assert.Equal(t, "user-123", claims.TenantID,
		"empty Identity.TenantID must default to UserID (single-tenant fallback)")
}

func TestJWTIssuer_InvalidSignature(t *testing.T) {
	issuer := NewJWTIssuer(JWTConfig{
		Secret:   "this-is-a-valid-backend-token-secret-32-chars-long",
		Issuer:   "auth-hub",
		Audience: "alt-backend",
		TTL:      5 * time.Minute,
	})

	identity := &domain.Identity{
		UserID: "user-123",
		Email:  "test@example.com",
	}

	tokenStr, err := issuer.IssueBackendToken(identity, "session-abc")
	assert.NoError(t, err)

	// Parse with wrong secret
	_, err = jwt.ParseWithClaims(tokenStr, &backendClaims{}, func(token *jwt.Token) (any, error) {
		return []byte("wrong-secret-that-should-fail-validation"), nil
	})
	assert.Error(t, err)
}
