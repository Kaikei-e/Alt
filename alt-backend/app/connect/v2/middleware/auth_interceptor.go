// Package middleware provides Connect-RPC interceptors for authentication and other cross-cutting concerns.
package middleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"alt/config"
	"alt/domain"
)

const (
	backendTokenHeader = "X-Alt-Backend-Token"
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

// AuthInterceptor provides JWT authentication for Connect-RPC handlers
type AuthInterceptor struct {
	logger   *slog.Logger
	secret   []byte
	issuer   string
	audience string
}

// NewAuthInterceptor creates a new authentication interceptor
func NewAuthInterceptor(logger *slog.Logger, cfg *config.Config) *AuthInterceptor {
	secret := []byte(cfg.Auth.BackendTokenSecret)
	if len(secret) == 0 {
		if logger != nil {
			logger.Warn("BACKEND_TOKEN_SECRET not set, JWT auth will deny all requests")
		}
	}

	return &AuthInterceptor{
		logger:   logger,
		secret:   secret,
		issuer:   cfg.Auth.BackendTokenIssuer,
		audience: cfg.Auth.BackendTokenAudience,
	}
}

// Interceptor returns a connect.Interceptor that can be used with Connect handlers
// It supports both unary and streaming RPCs
func (a *AuthInterceptor) Interceptor() connect.Interceptor {
	return &authInterceptor{auth: a}
}

// authInterceptor implements both unary and streaming interceptors
type authInterceptor struct {
	auth *AuthInterceptor
}

// WrapUnary implements connect.Interceptor for unary RPCs
func (i *authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		userCtx, err := i.auth.validateToken(req.Header().Get(backendTokenHeader))
		if err != nil {
			i.auth.logError("unary auth failed", err)
			return nil, i.auth.toConnectError(err)
		}

		// Use domain's SetUserContext to attach user to context
		ctx = domain.SetUserContext(ctx, userCtx)
		return next(ctx, req)
	}
}

// WrapStreamingClient implements connect.Interceptor for client streaming RPCs
func (i *authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		return next(ctx, spec)
	}
}

// WrapStreamingHandler implements connect.Interceptor for server streaming RPCs
func (i *authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		userCtx, err := i.auth.validateToken(conn.RequestHeader().Get(backendTokenHeader))
		if err != nil {
			i.auth.logError("streaming auth failed", err)
			return i.auth.toConnectError(err)
		}

		// Use domain's SetUserContext to attach user to context
		ctx = domain.SetUserContext(ctx, userCtx)
		return next(ctx, conn)
	}
}

// validateToken validates the JWT token and returns user context
func (a *AuthInterceptor) validateToken(tokenStr string) (*domain.UserContext, error) {
	if tokenStr == "" {
		return nil, errMissingToken
	}

	if len(a.secret) == 0 {
		return nil, fmt.Errorf("JWT secret not configured")
	}

	// Parse and validate token
	parsed, err := jwt.ParseWithClaims(tokenStr, &BackendClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return a.secret, nil
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
	if claims.Issuer != a.issuer {
		return nil, errInvalidIssuer
	}

	// Verify audience
	audienceMatch := false
	for _, aud := range claims.Audience {
		if aud == a.audience {
			audienceMatch = true
			break
		}
	}
	if !audienceMatch {
		return nil, errInvalidAudience
	}

	// Parse user ID
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID in token: %w", err)
	}

	// Get expiration and issued-at times from standard JWT claims
	var expiresAt, loginAt time.Time
	if claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.Time
	}
	if claims.IssuedAt != nil {
		loginAt = claims.IssuedAt.Time
	}

	// Note: Using UserID as TenantID (single-tenant architecture per auth-hub)
	return &domain.UserContext{
		UserID:    userID,
		Email:     claims.Email,
		Role:      domain.UserRole(claims.Role),
		TenantID:  userID, // Use UserID as TenantID (single-tenant model)
		SessionID: claims.Sid,
		LoginAt:   loginAt,
		ExpiresAt: expiresAt,
	}, nil
}

// toConnectError converts authentication errors to Connect errors
func (a *AuthInterceptor) toConnectError(err error) *connect.Error {
	switch {
	case errors.Is(err, errMissingToken):
		return connect.NewError(connect.CodeUnauthenticated, errors.New("missing backend token"))
	case errors.Is(err, errInvalidToken), errors.Is(err, errInvalidClaims):
		return connect.NewError(connect.CodeUnauthenticated, errors.New("invalid backend token"))
	case errors.Is(err, errInvalidIssuer), errors.Is(err, errInvalidAudience):
		return connect.NewError(connect.CodeUnauthenticated, errors.New("invalid token issuer or audience"))
	default:
		return connect.NewError(connect.CodeUnauthenticated, errors.New("authentication failed"))
	}
}

// logError logs authentication errors
func (a *AuthInterceptor) logError(msg string, err error) {
	if a.logger != nil {
		a.logger.Error(msg, "error", err)
	}
}

// GetUserContext extracts user context from the request context.
// This is a convenience function that wraps domain.GetUserFromContext.
func GetUserContext(ctx context.Context) (*domain.UserContext, error) {
	return domain.GetUserFromContext(ctx)
}
