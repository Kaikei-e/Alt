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
