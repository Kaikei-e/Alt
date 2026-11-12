package config

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateTestEd25519KeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey, []byte) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err)
	spkiPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	return pub, priv, spkiPEM
}

func writeTestVerifyKeyFile(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backend_token_verify_key.pem")
	require.NoError(t, os.WriteFile(path, content, 0o600))
	return path
}

func TestLoadLocalUserJWTVerifier(t *testing.T) {
	t.Run("fails when BACKEND_TOKEN_VERIFY_KEY_FILE is unset", func(t *testing.T) {
		t.Setenv("BACKEND_TOKEN_VERIFY_KEY_FILE", "")

		v, err := LoadLocalUserJWTVerifier()
		require.Error(t, err)
		assert.Nil(t, v)
		assert.Contains(t, err.Error(), "BACKEND_TOKEN_VERIFY_KEY_FILE")
	})

	t.Run("fails when key file does not exist", func(t *testing.T) {
		t.Setenv("BACKEND_TOKEN_VERIFY_KEY_FILE", filepath.Join(t.TempDir(), "missing.pem"))

		v, err := LoadLocalUserJWTVerifier()
		require.Error(t, err)
		assert.Nil(t, v)
		assert.Contains(t, err.Error(), "BACKEND_TOKEN_VERIFY_KEY_FILE")
	})

	t.Run("fails when key file is non-Ed25519 SPKI public key", func(t *testing.T) {
		rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		rsaDER, err := x509.MarshalPKIXPublicKey(&rsaPriv.PublicKey)
		require.NoError(t, err)
		rsaPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: rsaDER})
		t.Setenv("BACKEND_TOKEN_VERIFY_KEY_FILE", writeTestVerifyKeyFile(t, rsaPEM))

		_, err = LoadLocalUserJWTVerifier()
		require.Error(t, err)

		ecPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		ecDER, err := x509.MarshalPKIXPublicKey(&ecPriv.PublicKey)
		require.NoError(t, err)
		ecPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: ecDER})
		t.Setenv("BACKEND_TOKEN_VERIFY_KEY_FILE", writeTestVerifyKeyFile(t, ecPEM))

		_, err = LoadLocalUserJWTVerifier()
		require.Error(t, err)
	})

	t.Run("fails when key file is PKCS8 private key or malformed PEM", func(t *testing.T) {
		_, priv, _ := generateTestEd25519KeyPair(t)
		pkcs8DER, err := x509.MarshalPKCS8PrivateKey(priv)
		require.NoError(t, err)
		privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8DER})
		t.Setenv("BACKEND_TOKEN_VERIFY_KEY_FILE", writeTestVerifyKeyFile(t, privPEM))

		_, err = LoadLocalUserJWTVerifier()
		require.Error(t, err)

		t.Setenv("BACKEND_TOKEN_VERIFY_KEY_FILE", writeTestVerifyKeyFile(t, []byte("not-a-pem-file")))
		_, err = LoadLocalUserJWTVerifier()
		require.Error(t, err)
	})

	t.Run("loads SPKI Ed25519 public key with default and custom issuer/audience", func(t *testing.T) {
		pub, _, spkiPEM := generateTestEd25519KeyPair(t)
		keyPath := writeTestVerifyKeyFile(t, spkiPEM)

		t.Setenv("BACKEND_TOKEN_VERIFY_KEY_FILE", keyPath)
		t.Setenv("USER_JWT_ISSUER", "")
		t.Setenv("BACKEND_TOKEN_ISSUER", "")
		t.Setenv("USER_JWT_AUDIENCE", "")
		t.Setenv("BACKEND_TOKEN_AUDIENCE", "")

		v, err := LoadLocalUserJWTVerifier()
		require.NoError(t, err)
		require.NotNil(t, v)
		assert.Equal(t, pub, v.PublicKey)
		assert.Equal(t, "auth-hub", v.Issuer)
		assert.Equal(t, "alt-backend", v.Audience)

		t.Setenv("BACKEND_TOKEN_ISSUER", "custom-issuer")
		t.Setenv("BACKEND_TOKEN_AUDIENCE", "custom-aud")

		vCustom, err := LoadLocalUserJWTVerifier()
		require.NoError(t, err)
		require.NotNil(t, vCustom)
		assert.Equal(t, "custom-issuer", vCustom.Issuer)
		assert.Equal(t, "custom-aud", vCustom.Audience)
	})
}

func TestLoad_VerifyKeyStartupBehaviour(t *testing.T) {
	t.Run("missing verify key file fails startup when policy auth requires verifier without remote mTLS", func(t *testing.T) {
		tok := writeTempTokenFile(t, "tok", "service-token-that-is-long-enough-12345")
		policyJSON := `{
			"version": 1,
			"services": [{
				"name": "alt-backend",
				"token_file": "` + tok + `",
				"allowed_methods": ["/services.sovereign.v1.KnowledgeSovereignService/GetTrailFootprints"],
				"require_user_token": true
			}]
		}`
		policyFile := writeTempTokenFile(t, "policy.json", policyJSON)

		t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
		t.Setenv("ADMIN_AUTH", "disabled")
		t.Setenv("EVENT_AUTH", "")
		t.Setenv("EVENT_AUTH_POLICY_FILE", policyFile)
		t.Setenv("USER_JWT_INTROSPECTION_URL", "")
		t.Setenv("MTLS_CERT_FILE", "")
		t.Setenv("CERT_PATH", "")
		t.Setenv("USER_JWT_CERT_FILE", "")
		t.Setenv("BACKEND_TOKEN_VERIFY_KEY_FILE", filepath.Join(t.TempDir(), "missing.pem"))

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BACKEND_TOKEN_VERIFY_KEY_FILE")
	})

	t.Run("unset verify key file fails startup when policy auth is enabled without remote introspection", func(t *testing.T) {
		tok := writeTempTokenFile(t, "tok", "service-token-that-is-long-enough-12345")
		policyJSON := `{
			"version": 1,
			"services": [{
				"name": "alt-backend",
				"token_file": "` + tok + `",
				"allowed_methods": ["/services.sovereign.v1.KnowledgeSovereignService/GetTrailFootprints"],
				"require_user_token": true
			}]
		}`
		policyFile := writeTempTokenFile(t, "policy.json", policyJSON)

		t.Setenv("APP_ENV", "test")
		t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
		t.Setenv("ADMIN_AUTH", "disabled")
		t.Setenv("EVENT_AUTH", "")
		t.Setenv("EVENT_AUTH_POLICY_FILE", policyFile)
		t.Setenv("USER_JWT_INTROSPECTION_URL", "")
		t.Setenv("MTLS_CERT_FILE", "")
		t.Setenv("CERT_PATH", "")
		t.Setenv("USER_JWT_CERT_FILE", "")
		t.Setenv("BACKEND_TOKEN_VERIFY_KEY_FILE", "")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BACKEND_TOKEN_VERIFY_KEY_FILE")
		assert.Contains(t, err.Error(), "USER_JWT_INTROSPECTION_URL")
	})

	t.Run("loads local Ed25519 verifier when BACKEND_TOKEN_VERIFY_KEY_FILE is configured", func(t *testing.T) {
		pub, _, spkiPEM := generateTestEd25519KeyPair(t)
		keyPath := writeTestVerifyKeyFile(t, spkiPEM)
		tok := writeTempTokenFile(t, "tok", "service-token-that-is-long-enough-12345")
		policyJSON := `{
			"version": 1,
			"services": [{
				"name": "alt-backend",
				"token_file": "` + tok + `",
				"allowed_methods": ["/services.sovereign.v1.KnowledgeSovereignService/GetTrailFootprints"],
				"require_user_token": true
			}]
		}`
		policyFile := writeTempTokenFile(t, "policy.json", policyJSON)

		t.Setenv("APP_ENV", "test")
		t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
		t.Setenv("ADMIN_AUTH", "disabled")
		t.Setenv("EVENT_AUTH", "")
		t.Setenv("EVENT_AUTH_POLICY_FILE", policyFile)
		t.Setenv("USER_JWT_INTROSPECTION_URL", "")
		t.Setenv("BACKEND_TOKEN_VERIFY_KEY_FILE", keyPath)

		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, UserTokenVerifierModeLocal, cfg.UserTokenVerifierMode)
		localVerifier, ok := cfg.UserJWTVerifier.(*LocalUserJWTVerifier)
		require.True(t, ok)
		assert.Equal(t, pub, localVerifier.PublicKey)
	})
}

func TestLoad_UserTokenVerifierSelection(t *testing.T) {
	t.Run("both BACKEND_TOKEN_VERIFY_KEY_FILE and USER_JWT_INTROSPECTION_URL set returns error naming both", func(t *testing.T) {
		setupTempMTLSCerts(t)
		_, _, spkiPEM := generateTestEd25519KeyPair(t)
		keyPath := writeTestVerifyKeyFile(t, spkiPEM)

		t.Setenv("APP_ENV", "test")
		t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
		t.Setenv("ADMIN_AUTH", "disabled")
		t.Setenv("EVENT_AUTH", "disabled")
		t.Setenv("BACKEND_TOKEN_VERIFY_KEY_FILE", "  "+keyPath+"  ")
		t.Setenv("USER_JWT_INTROSPECTION_URL", "  https://auth-hub:9443/internal/token/introspect  ")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BACKEND_TOKEN_VERIFY_KEY_FILE")
		assert.Contains(t, err.Error(), "USER_JWT_INTROSPECTION_URL")
	})

	t.Run("key file only selects local mode", func(t *testing.T) {
		_, _, spkiPEM := generateTestEd25519KeyPair(t)
		keyPath := writeTestVerifyKeyFile(t, spkiPEM)

		t.Setenv("APP_ENV", "test")
		t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
		t.Setenv("ADMIN_AUTH", "disabled")
		t.Setenv("EVENT_AUTH", "disabled")
		t.Setenv("BACKEND_TOKEN_VERIFY_KEY_FILE", keyPath)
		t.Setenv("USER_JWT_INTROSPECTION_URL", "   ")

		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, "local", cfg.UserTokenVerifierMode)
		require.NotNil(t, cfg.UserJWTVerifier)
	})

	t.Run("introspection URL only selects introspection mode", func(t *testing.T) {
		setupTempMTLSCerts(t)

		t.Setenv("APP_ENV", "test")
		t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
		t.Setenv("ADMIN_AUTH", "disabled")
		t.Setenv("EVENT_AUTH", "disabled")
		t.Setenv("BACKEND_TOKEN_VERIFY_KEY_FILE", "   ")
		t.Setenv("USER_JWT_INTROSPECTION_URL", "https://auth-hub:9443/internal/token/introspect")

		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, "introspection", cfg.UserTokenVerifierMode)
		require.NotNil(t, cfg.UserJWTVerifier)
	})

	t.Run("neither set with policy requiring user tokens returns error naming both", func(t *testing.T) {
		setupTempMTLSCerts(t)
		tok := writeTempTokenFile(t, "tok", "service-token-that-is-long-enough-12345")
		policyJSON := `{
			"version": 1,
			"services": [{
				"name": "alt-backend",
				"token_file": "` + tok + `",
				"allowed_methods": ["/services.sovereign.v1.KnowledgeSovereignService/GetTrailFootprints"],
				"require_user_token": true
			}]
		}`
		policyFile := writeTempTokenFile(t, "policy.json", policyJSON)

		t.Setenv("APP_ENV", "test")
		t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
		t.Setenv("ADMIN_AUTH", "disabled")
		t.Setenv("EVENT_AUTH", "")
		t.Setenv("EVENT_AUTH_POLICY_FILE", policyFile)
		t.Setenv("BACKEND_TOKEN_VERIFY_KEY_FILE", "")
		t.Setenv("USER_JWT_INTROSPECTION_URL", "")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BACKEND_TOKEN_VERIFY_KEY_FILE")
		assert.Contains(t, err.Error(), "USER_JWT_INTROSPECTION_URL")
	})

	t.Run("neither set without user-token principal selects none even if mTLS certs exist", func(t *testing.T) {
		setupTempMTLSCerts(t)

		t.Setenv("APP_ENV", "test")
		t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
		t.Setenv("ADMIN_AUTH", "disabled")
		t.Setenv("EVENT_AUTH", "disabled")
		t.Setenv("BACKEND_TOKEN_VERIFY_KEY_FILE", "")
		t.Setenv("USER_JWT_INTROSPECTION_URL", "")

		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, "none", cfg.UserTokenVerifierMode)
		assert.Nil(t, cfg.UserJWTVerifier)
	})
}

func TestLocalUserJWTVerifier_ValidateToken(t *testing.T) {
	ctx := context.Background()
	pubKey, privKey, spkiPEM := generateTestEd25519KeyPair(t)
	userID := uuid.New().String()
	tenantID := uuid.New().String()

	verifier := &LocalUserJWTVerifier{
		PublicKey: pubKey,
		Issuer:    "auth-hub",
		Audience:  "alt-backend",
	}

	mintEdDSA := func(claims localJWTClaims, key ed25519.PrivateKey) string {
		tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
		s, err := tok.SignedString(key)
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

	t.Run("accepts valid EdDSA token", func(t *testing.T) {
		tokenStr := mintEdDSA(validClaims(), privKey)
		claims, err := verifier.ValidateToken(ctx, tokenStr)
		require.NoError(t, err)
		require.NotNil(t, claims)
		assert.Equal(t, userID, claims.Subject)
		assert.Equal(t, tenantID, claims.TenantID)
		assert.Equal(t, "user@example.com", claims.Email)
		assert.Equal(t, "member", claims.Role)
		assert.Equal(t, "session-1", claims.Sid)
	})

	t.Run("rejects HS256 token signed with public-key bytes as HMAC secret", func(t *testing.T) {
		hsRawTok := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims())
		hsRawStr, err := hsRawTok.SignedString([]byte(pubKey))
		require.NoError(t, err)

		_, err = verifier.ValidateToken(ctx, hsRawStr)
		require.Error(t, err)

		hsPEMTok := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims())
		hsPEMStr, err := hsPEMTok.SignedString(spkiPEM)
		require.NoError(t, err)

		_, err = verifier.ValidateToken(ctx, hsPEMStr)
		require.Error(t, err)
	})

	t.Run("rejects alg none and RS256", func(t *testing.T) {
		noneTok := jwt.NewWithClaims(jwt.SigningMethodNone, validClaims())
		noneStr, err := noneTok.SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)

		_, err = verifier.ValidateToken(ctx, noneStr)
		require.Error(t, err)

		rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		rsTok := jwt.NewWithClaims(jwt.SigningMethodRS256, validClaims())
		rsStr, err := rsTok.SignedString(rsaPriv)
		require.NoError(t, err)

		_, err = verifier.ValidateToken(ctx, rsStr)
		require.Error(t, err)
	})

	t.Run("rejects token signed by a different Ed25519 key", func(t *testing.T) {
		_, otherPriv, _ := generateTestEd25519KeyPair(t)
		_, err := verifier.ValidateToken(ctx, mintEdDSA(validClaims(), otherPriv))
		require.Error(t, err)
	})

	t.Run("rejects wrong issuer, wrong audience, and expired token", func(t *testing.T) {
		cIss := validClaims()
		cIss.Issuer = "wrong-issuer"
		_, err := verifier.ValidateToken(ctx, mintEdDSA(cIss, privKey))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "issuer")

		cAud := validClaims()
		cAud.Audience = jwt.ClaimStrings{"wrong-aud"}
		_, err = verifier.ValidateToken(ctx, mintEdDSA(cAud, privKey))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "audience")

		cExp := validClaims()
		cExp.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-1 * time.Minute))
		_, err = verifier.ValidateToken(ctx, mintEdDSA(cExp, privKey))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token")
	})

	t.Run("rejects empty token, unconfigured verifier, or non-UUID subject and tenant_id", func(t *testing.T) {
		_, err := verifier.ValidateToken(ctx, "")
		require.Error(t, err)

		var nilVerifier *LocalUserJWTVerifier
		_, err = nilVerifier.ValidateToken(ctx, "tok")
		require.Error(t, err)

		emptyKeyVerifier := &LocalUserJWTVerifier{Issuer: "auth-hub", Audience: "alt-backend"}
		_, err = emptyKeyVerifier.ValidateToken(ctx, mintEdDSA(validClaims(), privKey))
		require.Error(t, err)

		cSub := validClaims()
		cSub.Subject = "not-a-uuid"
		_, err = verifier.ValidateToken(ctx, mintEdDSA(cSub, privKey))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token subject")

		cTenant := validClaims()
		cTenant.TenantID = "not-a-uuid"
		_, err = verifier.ValidateToken(ctx, mintEdDSA(cTenant, privKey))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token tenant_id")
	})
}
