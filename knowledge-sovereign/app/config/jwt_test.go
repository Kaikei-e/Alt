package config

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadLocalUserJWTVerifier(t *testing.T) {
	t.Run("returns nil when no secret configured", func(t *testing.T) {
		t.Setenv("USER_JWT_SECRET_FILE", "")
		t.Setenv("BACKEND_TOKEN_SECRET_FILE", "")
		t.Setenv("USER_JWT_SECRET", "")
		t.Setenv("BACKEND_TOKEN_SECRET", "")

		v, err := LoadLocalUserJWTVerifier()
		require.NoError(t, err)
		assert.Nil(t, v)
	})

	t.Run("loads from USER_JWT_SECRET with default issuer and audience", func(t *testing.T) {
		t.Setenv("USER_JWT_SECRET_FILE", "")
		t.Setenv("BACKEND_TOKEN_SECRET_FILE", "")
		t.Setenv("USER_JWT_SECRET", "super-secret-jwt-key-at-least-32-bytes")
		t.Setenv("USER_JWT_ISSUER", "")
		t.Setenv("BACKEND_TOKEN_ISSUER", "")
		t.Setenv("USER_JWT_AUDIENCE", "")
		t.Setenv("BACKEND_TOKEN_AUDIENCE", "")

		v, err := LoadLocalUserJWTVerifier()
		require.NoError(t, err)
		require.NotNil(t, v)
		assert.Equal(t, "super-secret-jwt-key-at-least-32-bytes", v.Secret)
		assert.Equal(t, "auth-hub", v.Issuer)
		assert.Equal(t, "alt-backend", v.Audience)
	})

	t.Run("loads from USER_JWT_SECRET_FILE and custom issuer/audience", func(t *testing.T) {
		dir := t.TempDir()
		secretFile := filepath.Join(dir, "jwt_secret")
		require.NoError(t, os.WriteFile(secretFile, []byte("  file-secret-key-at-least-32-bytes\n"), 0o600))

		t.Setenv("USER_JWT_SECRET_FILE", secretFile)
		t.Setenv("BACKEND_TOKEN_ISSUER", "custom-issuer")
		t.Setenv("BACKEND_TOKEN_AUDIENCE", "custom-aud")

		v, err := LoadLocalUserJWTVerifier()
		require.NoError(t, err)
		require.NotNil(t, v)
		assert.Equal(t, "file-secret-key-at-least-32-bytes", v.Secret)
		assert.Equal(t, "custom-issuer", v.Issuer)
		assert.Equal(t, "custom-aud", v.Audience)
	})

	t.Run("returns error when secret file missing", func(t *testing.T) {
		t.Setenv("USER_JWT_SECRET_FILE", "/nonexistent/path/secret")
		_, err := LoadLocalUserJWTVerifier()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read USER_JWT_SECRET_FILE")
	})
}

func TestLocalUserJWTVerifier_ValidateToken(t *testing.T) {
	ctx := context.Background()
	secret := "test-jwt-secret-key-32-bytes-long!!"
	userID := uuid.New().String()
	tenantID := uuid.New().String()

	verifier := &LocalUserJWTVerifier{
		Secret:   secret,
		Issuer:   "auth-hub",
		Audience: "alt-backend",
	}

	mintHS256 := func(claims localJWTClaims, key string) string {
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		s, err := tok.SignedString([]byte(key))
		require.NoError(t, err)
		return s
	}

	validClaims := func() localJWTClaims {
		return localJWTClaims{
			Email:    "user@example.com",
			Role:     "member",
			Sid:      "session-1",
			TenantID: tenantID,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   userID,
				Issuer:    "auth-hub",
				Audience:  jwt.ClaimStrings{"alt-backend"},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
	}

	t.Run("validates valid HS256 token", func(t *testing.T) {
		tokenStr := mintHS256(validClaims(), secret)
		claims, err := verifier.ValidateToken(ctx, tokenStr)
		require.NoError(t, err)
		require.NotNil(t, claims)
		assert.Equal(t, userID, claims.Subject)
		assert.Equal(t, tenantID, claims.TenantID)
		assert.Equal(t, "user@example.com", claims.Email)
		assert.Equal(t, "member", claims.Role)
		assert.Equal(t, "session-1", claims.Sid)
	})

	t.Run("rejects empty token or unconfigured verifier", func(t *testing.T) {
		_, err := verifier.ValidateToken(ctx, "")
		require.Error(t, err)

		var nilVerifier *LocalUserJWTVerifier
		_, err = nilVerifier.ValidateToken(ctx, "tok")
		require.Error(t, err)
	})

	t.Run("rejects expired token", func(t *testing.T) {
		c := validClaims()
		c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-1 * time.Minute))
		_, err := verifier.ValidateToken(ctx, mintHS256(c, secret))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token")
	})

	t.Run("rejects non-HS256 signing method (RS256 and none)", func(t *testing.T) {
		privKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		rsTok := jwt.NewWithClaims(jwt.SigningMethodRS256, validClaims())
		rsStr, err := rsTok.SignedString(privKey)
		require.NoError(t, err)

		_, err = verifier.ValidateToken(ctx, rsStr)
		require.Error(t, err)

		noneTok := jwt.NewWithClaims(jwt.SigningMethodNone, validClaims())
		noneStr, err := noneTok.SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)

		_, err = verifier.ValidateToken(ctx, noneStr)
		require.Error(t, err)
	})

	t.Run("rejects wrong issuer and wrong audience", func(t *testing.T) {
		c := validClaims()
		c.Issuer = "wrong-issuer"
		_, err := verifier.ValidateToken(ctx, mintHS256(c, secret))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "issuer")

		c2 := validClaims()
		c2.Audience = jwt.ClaimStrings{"wrong-aud"}
		_, err = verifier.ValidateToken(ctx, mintHS256(c2, secret))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "audience")
	})

	t.Run("rejects empty or non-UUID subject and tenant_id", func(t *testing.T) {
		c := validClaims()
		c.Subject = "not-a-uuid"
		_, err := verifier.ValidateToken(ctx, mintHS256(c, secret))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token subject")

		c2 := validClaims()
		c2.TenantID = "not-a-uuid"
		_, err = verifier.ValidateToken(ctx, mintHS256(c2, secret))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token tenant_id")
	})
}
