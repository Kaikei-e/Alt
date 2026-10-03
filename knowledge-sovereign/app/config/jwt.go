package config

import (
	"context"
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
	Secret   string
	Issuer   string
	Audience string
}

// LoadUserJWTVerifier loads JWT verifier credentials from environment variables.
func LoadLocalUserJWTVerifier() (*LocalUserJWTVerifier, error) {
	secret := ""
	if path := os.Getenv("USER_JWT_SECRET_FILE"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read USER_JWT_SECRET_FILE: %w", err)
		}
		secret = strings.TrimSpace(string(data))
	} else if path := os.Getenv("BACKEND_TOKEN_SECRET_FILE"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read BACKEND_TOKEN_SECRET_FILE: %w", err)
		}
		secret = strings.TrimSpace(string(data))
	} else if envSecret := os.Getenv("USER_JWT_SECRET"); envSecret != "" {
		secret = strings.TrimSpace(envSecret)
	} else if envSecret := os.Getenv("BACKEND_TOKEN_SECRET"); envSecret != "" {
		secret = strings.TrimSpace(envSecret)
	}

	if secret == "" {
		return nil, nil
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
		Secret:   secret,
		Issuer:   issuer,
		Audience: audience,
	}, nil
}

// ValidateToken validates the JWT signature, algorithm, issuer, audience, and standard claims.
func (v *LocalUserJWTVerifier) ValidateToken(ctx context.Context, tokenStr string) (*BackendClaims, error) {
	if tokenStr == "" {
		return nil, errors.New("missing user delegation token")
	}
	if v == nil || v.Secret == "" {
		return nil, errors.New("user JWT verifier not configured: missing secret")
	}

	token, err := jwt.ParseWithClaims(tokenStr, &localJWTClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(v.Secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
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
