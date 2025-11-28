package middleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"alt/config"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

const (
	backendTokenHeader = "X-Alt-Backend-Token"
	userContextKey     = "altUser"
)

var (
	errMissingToken    = errors.New("missing backend token")
	errInvalidToken    = errors.New("invalid backend token")
	errInvalidClaims   = errors.New("invalid claims")
	errInvalidIssuer   = errors.New("invalid issuer")
	errInvalidAudience = errors.New("invalid audience")
)

// BackendClaims represents the JWT claims for backend authentication
type BackendClaims struct {
	Email string `json:"email"`
	Role  string `json:"role"`
	Sid   string `json:"sid"`
	jwt.RegisteredClaims
}

// UserContext holds user information extracted from JWT
type UserContext struct {
	ID    string
	Email string
	Role  string
	Sid   string
}

// JWTAuthMiddleware validates JWT tokens for backend authentication
type JWTAuthMiddleware struct {
	logger   *slog.Logger
	config   *config.Config
	secret   []byte
	issuer   string
	audience string
}

// NewJWTAuthMiddleware creates a new JWT authentication middleware
func NewJWTAuthMiddleware(logger *slog.Logger, cfg *config.Config) *JWTAuthMiddleware {
	secret := []byte(cfg.Auth.BackendTokenSecret)
	if len(secret) == 0 {
		if logger != nil {
			logger.Warn("BACKEND_TOKEN_SECRET not set, JWT auth will deny all requests")
		}
	}

	return &JWTAuthMiddleware{
		logger:   logger,
		config:   cfg,
		secret:   secret,
		issuer:   cfg.Auth.BackendTokenIssuer,
		audience: cfg.Auth.BackendTokenAudience,
	}
}

// RequireJWT ensures that a valid JWT token is present before allowing the request to proceed
func (m *JWTAuthMiddleware) RequireJWT() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userCtx, err := m.validateJWT(c)
			if err != nil {
				switch {
				case errors.Is(err, errMissingToken):
					return echo.NewHTTPError(http.StatusUnauthorized, "missing backend token")
				case errors.Is(err, errInvalidToken), errors.Is(err, errInvalidClaims):
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid backend token")
				case errors.Is(err, errInvalidIssuer), errors.Is(err, errInvalidAudience):
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid token issuer or audience")
				default:
					if m.logger != nil {
						m.logger.Error("JWT validation error", "error", err)
					}
					return echo.NewHTTPError(http.StatusUnauthorized, "authentication failed")
				}
			}

			// Attach user context to request
			ctx := context.WithValue(c.Request().Context(), userContextKey, userCtx)
			c.SetRequest(c.Request().WithContext(ctx))

			return next(c)
		}
	}
}

// validateJWT validates the JWT token and returns user context
func (m *JWTAuthMiddleware) validateJWT(c echo.Context) (*UserContext, error) {
	tokenStr := c.Request().Header.Get(backendTokenHeader)
	if tokenStr == "" {
		return nil, errMissingToken
	}

	if len(m.secret) == 0 {
		return nil, fmt.Errorf("JWT secret not configured")
	}

	// Parse and validate token
	parsed, err := jwt.ParseWithClaims(tokenStr, &BackendClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.secret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("%w: %v", errInvalidToken, err)
	}

	if !parsed.Valid {
		return nil, errInvalidToken
	}

	claims, ok := parsed.Claims.(*BackendClaims)
	if !ok {
		return nil, errInvalidClaims
	}

	// Verify issuer
	if claims.Issuer != m.issuer {
		return nil, errInvalidIssuer
	}

	// Verify audience
	audienceMatch := false
	for _, aud := range claims.Audience {
		if aud == m.audience {
			audienceMatch = true
			break
		}
	}
	if !audienceMatch {
		return nil, errInvalidAudience
	}

	// Additional validation: check that header values match JWT claims if present
	headerUserID := c.Request().Header.Get(userIDHeader)
	headerSessionID := c.Request().Header.Get(sessionIDHeader)

	if headerUserID != "" && headerUserID != claims.Subject {
		return nil, fmt.Errorf("user id mismatch: header=%s, token=%s", headerUserID, claims.Subject)
	}

	if headerSessionID != "" && headerSessionID != claims.Sid {
		return nil, fmt.Errorf("session id mismatch: header=%s, token=%s", headerSessionID, claims.Sid)
	}

	return &UserContext{
		ID:    claims.Subject,
		Email: claims.Email,
		Role:  claims.Role,
		Sid:   claims.Sid,
	}, nil
}

// GetUserContext extracts user context from request context
func GetUserContext(c echo.Context) (*UserContext, bool) {
	ctx := c.Request().Context()
	userCtx, ok := ctx.Value(userContextKey).(*UserContext)
	return userCtx, ok
}
