package token

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"auth-hub/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateTestKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return pub, priv
}

func TestNewJWTIssuer_PanicsOnInvalidPrivateKey(t *testing.T) {
	t.Run("panics on short key", func(t *testing.T) {
		assert.Panics(t, func() {
			NewJWTIssuer(JWTConfig{
				PrivateKey: ed25519.PrivateKey("short-key"),
				Issuer:     "auth-hub",
				Audience:   "alt-backend",
				TTL:        5 * time.Minute,
			})
		})
	})

	t.Run("panics on nil key", func(t *testing.T) {
		assert.Panics(t, func() {
			NewJWTIssuer(JWTConfig{
				Issuer:   "auth-hub",
				Audience: "alt-backend",
				TTL:      5 * time.Minute,
			})
		})
	})
}

func TestJWTIssuer_IssueBackendToken(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	issuer := NewJWTIssuer(JWTConfig{
		PrivateKey: priv,
		Issuer:     "auth-hub",
		Audience:   "alt-backend",
		TTL:        5 * time.Minute,
	})

	identity := &domain.Identity{
		UserID: "user-123",
		Email:  "test@example.com",
	}

	tokenStr, err := issuer.IssueBackendToken(identity, "session-abc")
	require.NoError(t, err)
	assert.NotEmpty(t, tokenStr)

	parsed, err := jwt.ParseWithClaims(tokenStr, &backendClaims{}, func(token *jwt.Token) (any, error) {
		return pub, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}))
	require.NoError(t, err)
	assert.True(t, parsed.Valid)
	assert.Equal(t, "EdDSA", parsed.Header["alg"])

	claims := parsed.Claims.(*backendClaims)
	assert.Equal(t, "user-123", claims.Subject)
	assert.Equal(t, "test@example.com", claims.Email)
	assert.Equal(t, "user", claims.Role)
	assert.Equal(t, "session-abc", claims.Sid)
	assert.Equal(t, "auth-hub", claims.Issuer)
	assert.Contains(t, claims.Audience, "alt-backend")
}

func TestJWTIssuer_AdminRole(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	issuer := NewJWTIssuer(JWTConfig{
		PrivateKey: priv,
		Issuer:     "auth-hub",
		Audience:   "alt-backend",
		TTL:        5 * time.Minute,
	})

	identity := &domain.Identity{
		UserID: "admin-001",
		Email:  "admin@example.com",
		Role:   "admin",
	}

	tokenStr, err := issuer.IssueBackendToken(identity, "session-admin")
	require.NoError(t, err)

	parsed, err := jwt.ParseWithClaims(tokenStr, &backendClaims{}, func(token *jwt.Token) (any, error) {
		return pub, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}))
	require.NoError(t, err)

	claims := parsed.Claims.(*backendClaims)
	assert.Equal(t, "admin", claims.Role)
}

func TestJWTIssuer_EmptyRole_DefaultsToUser(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	issuer := NewJWTIssuer(JWTConfig{
		PrivateKey: priv,
		Issuer:     "auth-hub",
		Audience:   "alt-backend",
		TTL:        5 * time.Minute,
	})

	identity := &domain.Identity{
		UserID: "user-123",
		Email:  "test@example.com",
		Role:   "",
	}

	tokenStr, err := issuer.IssueBackendToken(identity, "session-abc")
	require.NoError(t, err)

	parsed, err := jwt.ParseWithClaims(tokenStr, &backendClaims{}, func(token *jwt.Token) (any, error) {
		return pub, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}))
	require.NoError(t, err)

	claims := parsed.Claims.(*backendClaims)
	assert.Equal(t, "user", claims.Role)
}

func TestJWTIssuer_ExpiredToken(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	issuer := NewJWTIssuer(JWTConfig{
		PrivateKey: priv,
		Issuer:     "auth-hub",
		Audience:   "alt-backend",
		TTL:        -1 * time.Minute,
	})

	identity := &domain.Identity{
		UserID: "user-123",
		Email:  "test@example.com",
	}

	tokenStr, err := issuer.IssueBackendToken(identity, "session-abc")
	require.NoError(t, err)

	_, err = jwt.ParseWithClaims(tokenStr, &backendClaims{}, func(token *jwt.Token) (any, error) {
		return pub, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}))
	assert.Error(t, err)
}

func TestJWTIssuer_IncludesTenantIDClaim(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	issuer := NewJWTIssuer(JWTConfig{
		PrivateKey: priv,
		Issuer:     "auth-hub",
		Audience:   "alt-backend",
		TTL:        5 * time.Minute,
	})

	identity := &domain.Identity{
		UserID:   "user-123",
		TenantID: "tenant-999",
		Email:    "test@example.com",
	}

	tokenStr, err := issuer.IssueBackendToken(identity, "session-abc")
	require.NoError(t, err)

	parsed, err := jwt.ParseWithClaims(tokenStr, &backendClaims{}, func(token *jwt.Token) (any, error) {
		return pub, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}))
	require.NoError(t, err)

	claims := parsed.Claims.(*backendClaims)
	assert.Equal(t, "tenant-999", claims.TenantID,
		"JWT must carry the tenant_id claim taken from identity.TenantID")
}

func TestJWTIssuer_TenantIDDefaultsToUserID(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	issuer := NewJWTIssuer(JWTConfig{
		PrivateKey: priv,
		Issuer:     "auth-hub",
		Audience:   "alt-backend",
		TTL:        5 * time.Minute,
	})

	identity := &domain.Identity{
		UserID: "user-123",
		Email:  "test@example.com",
	}

	tokenStr, err := issuer.IssueBackendToken(identity, "session-abc")
	require.NoError(t, err)

	parsed, err := jwt.ParseWithClaims(tokenStr, &backendClaims{}, func(token *jwt.Token) (any, error) {
		return pub, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}))
	require.NoError(t, err)

	claims := parsed.Claims.(*backendClaims)
	assert.Equal(t, "user-123", claims.TenantID,
		"empty Identity.TenantID must default to UserID (single-tenant fallback)")
}

func TestJWTIssuer_InvalidSignature(t *testing.T) {
	_, priv := generateTestKeyPair(t)
	wrongPub, _ := generateTestKeyPair(t)
	issuer := NewJWTIssuer(JWTConfig{
		PrivateKey: priv,
		Issuer:     "auth-hub",
		Audience:   "alt-backend",
		TTL:        5 * time.Minute,
	})

	identity := &domain.Identity{
		UserID: "user-123",
		Email:  "test@example.com",
	}

	tokenStr, err := issuer.IssueBackendToken(identity, "session-abc")
	require.NoError(t, err)

	_, err = jwt.ParseWithClaims(tokenStr, &backendClaims{}, func(token *jwt.Token) (any, error) {
		return wrongPub, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}))
	assert.Error(t, err)
}

func TestJWTIssuer_IntrospectBackendToken(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	issuer := NewJWTIssuer(JWTConfig{
		PrivateKey: priv,
		Issuer:     "auth-hub",
		Audience:   "alt-backend",
		TTL:        5 * time.Minute,
	})

	subUUID := uuid.NewString()
	tenantUUID := uuid.NewString()

	validClaims := func() backendClaims {
		now := time.Now()
		return backendClaims{
			Email:    "user@example.com",
			Role:     "user",
			Sid:      "sess-1",
			TenantID: tenantUUID,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    "auth-hub",
				Audience:  jwt.ClaimStrings{"alt-backend"},
				Subject:   subUUID,
				IssuedAt:  jwt.NewNumericDate(now),
				ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
			},
		}
	}

	t.Run("valid EdDSA token accepted", func(t *testing.T) {
		tok, err := issuer.IssueBackendToken(&domain.Identity{
			UserID:   subUUID,
			TenantID: tenantUUID,
			Email:    "user@example.com",
			Role:     "user",
		}, "sess-1")
		require.NoError(t, err)

		res, err := issuer.IntrospectBackendToken(tok)
		require.NoError(t, err)
		assert.True(t, res.Active)
		assert.Equal(t, subUUID, res.Sub)
		assert.Equal(t, tenantUUID, res.TenantID)
		assert.Greater(t, res.Exp, time.Now().Unix())
	})

	t.Run("HS256 token signed with public key bytes as HMAC secret rejected", func(t *testing.T) {
		hsTok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims()).SignedString([]byte(pub))
		require.NoError(t, err)

		res, err := issuer.IntrospectBackendToken(hsTok)
		require.Error(t, err)
		assert.False(t, res.Active)

		spkiDER, err := x509.MarshalPKIXPublicKey(pub)
		require.NoError(t, err)
		spkiPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: spkiDER})
		hsPEMTok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims()).SignedString(spkiPEM)
		require.NoError(t, err)

		resPEM, err := issuer.IntrospectBackendToken(hsPEMTok)
		require.Error(t, err)
		assert.False(t, resPEM.Active)
	})

	t.Run("alg none rejected", func(t *testing.T) {
		noneTok, err := jwt.NewWithClaims(jwt.SigningMethodNone, validClaims()).SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)

		res, err := issuer.IntrospectBackendToken(noneTok)
		require.Error(t, err)
		assert.False(t, res.Active)
	})

	t.Run("token signed by a different Ed25519 key rejected", func(t *testing.T) {
		_, otherPriv := generateTestKeyPair(t)
		otherTok, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, validClaims()).SignedString(otherPriv)
		require.NoError(t, err)

		res, err := issuer.IntrospectBackendToken(otherTok)
		require.Error(t, err)
		assert.False(t, res.Active)
	})

	t.Run("wrong issuer rejected", func(t *testing.T) {
		c := validClaims()
		c.Issuer = "other-issuer"
		tok, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, c).SignedString(priv)
		require.NoError(t, err)

		res, err := issuer.IntrospectBackendToken(tok)
		require.Error(t, err)
		assert.False(t, res.Active)
	})

	t.Run("wrong audience rejected", func(t *testing.T) {
		c := validClaims()
		c.Audience = jwt.ClaimStrings{"other-audience"}
		tok, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, c).SignedString(priv)
		require.NoError(t, err)

		res, err := issuer.IntrospectBackendToken(tok)
		require.Error(t, err)
		assert.False(t, res.Active)
	})

	t.Run("expired token rejected", func(t *testing.T) {
		c := validClaims()
		c.IssuedAt = jwt.NewNumericDate(time.Now().Add(-10 * time.Minute))
		c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-5 * time.Minute))
		tok, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, c).SignedString(priv)
		require.NoError(t, err)

		res, err := issuer.IntrospectBackendToken(tok)
		require.Error(t, err)
		assert.False(t, res.Active)
	})
}
