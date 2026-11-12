package config

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"knowledge-sovereign/domain/authcontext"
	"knowledge-sovereign/port/authport"
)

// BackendClaims represents the JWT claims for backend user authentication.
// Subject carries the authenticated user_id; TenantID carries the tenant_id.
type BackendClaims = authcontext.VerifiedClaims

type localJWTClaims struct {
	Email    string `json:"email"`
	Role     string `json:"role"`
	Sid      string `json:"sid"`
	TenantID string `json:"tenant_id"`
	jwt.RegisteredClaims
}

// UserJWTVerifier validates user delegation tokens presented in X-Alt-Backend-Token.
// TokenVerifier validates user delegation tokens.
type TokenVerifier = authport.TokenVerifier

type LocalUserJWTVerifier struct {
	PublicKey ed25519.PublicKey
	Issuer    string
	Audience  string
}

// LoadLocalUserJWTVerifier loads an Ed25519 SPKI public key from BACKEND_TOKEN_VERIFY_KEY_FILE.
func LoadLocalUserJWTVerifier() (*LocalUserJWTVerifier, error) {
	path := strings.TrimSpace(os.Getenv("BACKEND_TOKEN_VERIFY_KEY_FILE"))
	if path == "" {
		return nil, errors.New("BACKEND_TOKEN_VERIFY_KEY_FILE is required")
	}

	data, err := readSecretFile(path)
	if err != nil {
		return nil, fmt.Errorf("read BACKEND_TOKEN_VERIFY_KEY_FILE: %w", err)
	}

	block, rest := pem.Decode(data)
	if block == nil || len(strings.TrimSpace(string(rest))) > 0 || block.Type != "PUBLIC KEY" {
		return nil, errors.New("BACKEND_TOKEN_VERIFY_KEY_FILE must contain a single SPKI PUBLIC KEY PEM block")
	}

	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse BACKEND_TOKEN_VERIFY_KEY_FILE: %w", err)
	}

	pubKey, ok := parsed.(ed25519.PublicKey)
	if !ok || len(pubKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("BACKEND_TOKEN_VERIFY_KEY_FILE must contain an Ed25519 public key, got %T", parsed)
	}

	issuer := os.Getenv("USER_JWT_ISSUER")
	if issuer == "" {
		issuer = os.Getenv("BACKEND_TOKEN_ISSUER")
	}
	if issuer == "" {
		issuer = "auth-hub"
	}

	audience := os.Getenv("USER_JWT_AUDIENCE")
	if audience == "" {
		audience = os.Getenv("BACKEND_TOKEN_AUDIENCE")
	}
	if audience == "" {
		audience = "alt-backend"
	}

	return &LocalUserJWTVerifier{
		PublicKey: pubKey,
		Issuer:    issuer,
		Audience:  audience,
	}, nil
}

// ValidateToken validates the JWT signature, algorithm, issuer, audience, and standard claims.
func (v *LocalUserJWTVerifier) ValidateToken(ctx context.Context, tokenStr string) (*BackendClaims, error) {
	if tokenStr == "" {
		return nil, errors.New("missing user delegation token")
	}
	if v == nil || len(v.PublicKey) != ed25519.PublicKeySize {
		return nil, errors.New("user JWT verifier not configured: missing Ed25519 public key")
	}

	token, err := jwt.ParseWithClaims(tokenStr, &localJWTClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok || t.Method.Alg() != jwt.SigningMethodEdDSA.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return v.PublicKey, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	localClaims, ok := token.Claims.(*localJWTClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	if v.Issuer != "" && localClaims.Issuer != v.Issuer {
		return nil, fmt.Errorf("token issuer %q does not match expected %q", localClaims.Issuer, v.Issuer)
	}

	if v.Audience != "" {
		audienceMatch := false
		for _, aud := range localClaims.Audience {
			if aud == v.Audience {
				audienceMatch = true
				break
			}
		}
		if !audienceMatch {
			return nil, fmt.Errorf("token audience does not match expected %q", v.Audience)
		}
	}

	if localClaims.Subject == "" {
		return nil, errors.New("token subject is empty")
	}
	if _, err := uuid.Parse(localClaims.Subject); err != nil {
		return nil, fmt.Errorf("invalid token subject: %w", err)
	}
	if localClaims.TenantID == "" {
		return nil, errors.New("token tenant_id is empty")
	}
	if _, err := uuid.Parse(localClaims.TenantID); err != nil {
		return nil, fmt.Errorf("invalid token tenant_id: %w", err)
	}

	claims := &BackendClaims{
		Email:    localClaims.Email,
		Role:     localClaims.Role,
		Sid:      localClaims.Sid,
		TenantID: localClaims.TenantID,
		Subject:  localClaims.Subject,
	}
	if localClaims.ExpiresAt != nil {
		claims.ExpiresAt = localClaims.ExpiresAt.Time
	}

	return claims, nil
}
