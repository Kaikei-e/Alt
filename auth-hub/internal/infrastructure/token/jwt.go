package token

import (
	"time"

	"auth-hub/internal/domain"

	"github.com/golang-jwt/jwt/v5"
)

// JWTConfig holds JWT generation configuration.
type JWTConfig struct {
	Secret   string
	Issuer   string
	Audience string
	TTL      time.Duration
}

// backendClaims represents the JWT claims for backend authentication.
type backendClaims struct {
	Email string `json:"email"`
	Role  string `json:"role"`
	Sid   string `json:"sid"`
	jwt.RegisteredClaims
}

// JWTIssuer generates JWT tokens for backend authentication.
// Implements domain.TokenIssuer.
type JWTIssuer struct {
	cfg JWTConfig
}

// NewJWTIssuer creates a new JWT issuer.
func NewJWTIssuer(cfg JWTConfig) *JWTIssuer {
	return &JWTIssuer{cfg: cfg}
}

// IssueBackendToken generates a signed JWT token.
func (j *JWTIssuer) IssueBackendToken(identity *domain.Identity, sessionID string) (string, error) {
	now := time.Now()
	claims := backendClaims{
		Email: identity.Email,
		Role:  "user",
		Sid:   sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    j.cfg.Issuer,
			Audience:  jwt.ClaimStrings{j.cfg.Audience},
			Subject:   identity.UserID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.cfg.TTL)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(j.cfg.Secret))
}
