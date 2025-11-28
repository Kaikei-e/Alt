package token

import (
	"time"

	"auth-hub/client"
	"auth-hub/config"

	"github.com/golang-jwt/jwt/v5"
)

// BackendClaims represents the JWT claims for backend authentication
type BackendClaims struct {
	Email string `json:"email"`
	Role  string `json:"role"`
	Sid   string `json:"sid"`
	jwt.RegisteredClaims
}

// IssueBackendToken generates a JWT token for backend authentication
func IssueBackendToken(cfg *config.Config, identity *client.Identity, sessionID string) (string, error) {
	now := time.Now()
	claims := BackendClaims{
		Email: identity.Email,
		Role:  "user", // Default role, can be extended if identity has role field
		Sid:   sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    cfg.BackendTokenIssuer,
			Audience:  jwt.ClaimStrings{cfg.BackendTokenAudience},
			Subject:   identity.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(cfg.BackendTokenTTL)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.BackendTokenSecret))
}
