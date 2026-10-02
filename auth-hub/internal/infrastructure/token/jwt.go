package token

import (
	"time"

	"auth-hub/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// JWTConfig holds JWT generation configuration.
type JWTConfig struct {
	Secret   string
	Issuer   string
	Audience string
	TTL      time.Duration
}

// backendClaims represents the JWT claims for backend authentication.
// TenantID carries the tenant_id claim consumed by alt-backend; in single-tenant
// deployments it equals Subject (UserID), but keeping it as a dedicated claim
// decouples tenant from user identity and prepares for multi-tenant upgrades.
type backendClaims struct {
	Email    string `json:"email"`
	Role     string `json:"role"`
	Sid      string `json:"sid"`
	TenantID string `json:"tenant_id"`
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
	role := identity.Role
	if role == "" {
		role = "user"
	}

	// Single-tenant fallback: if the identity does not carry an explicit
	// tenant, derive it from UserID so downstream always sees a non-empty
	// tenant_id claim. Once multi-tenant support is wired upstream, callers
	// set Identity.TenantID explicitly and this branch becomes a no-op.
	tenantID := identity.TenantID
	if tenantID == "" {
		tenantID = identity.UserID
	}

	now := time.Now()
	claims := backendClaims{
		Email:    identity.Email,
		Role:     role,
		Sid:      sessionID,
		TenantID: tenantID,
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

// IntrospectBackendToken validates a signed JWT token and returns its key claims.
func (j *JWTIssuer) IntrospectBackendToken(tokenStr string) (*domain.IntrospectedToken, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &backendClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Strict HS256 enforcement
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		if token.Header["alg"] != "HS256" {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(j.cfg.Secret), nil
	}, jwt.WithIssuer(j.cfg.Issuer), jwt.WithAudience(j.cfg.Audience), jwt.WithExpirationRequired())

	if err != nil {
		return &domain.IntrospectedToken{Active: false}, err
	}

	if claims, ok := token.Claims.(*backendClaims); ok && token.Valid {
		if claims.Subject == "" || claims.TenantID == "" {
			return &domain.IntrospectedToken{Active: false}, jwt.ErrInvalidKey
		}

		if err := validateUUID(claims.Subject); err != nil {
			return &domain.IntrospectedToken{Active: false}, jwt.ErrInvalidKey
		}
		if err := validateUUID(claims.TenantID); err != nil {
			return &domain.IntrospectedToken{Active: false}, jwt.ErrInvalidKey
		}

		var exp int64
		if claims.ExpiresAt != nil {
			exp = claims.ExpiresAt.Unix()
		}

		return &domain.IntrospectedToken{
			Active:   true,
			Sub:      claims.Subject,
			TenantID: claims.TenantID,
			Exp:      exp,
		}, nil
	}

	return &domain.IntrospectedToken{Active: false}, jwt.ErrSignatureInvalid
}

func validateUUID(u string) error {
	fromExt, err := uuid.Parse(u)
	if err != nil {
		return err
	}
	_ = fromExt
	return nil
}
