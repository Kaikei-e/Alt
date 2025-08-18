package auth_gateway

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"alt/domain"
	"alt/driver/auth"
	"alt/port/auth_port"
)

// AuthGateway implements the auth port interface using the auth driver
type AuthGateway struct {
	authClient auth.AuthClient
	logger     *slog.Logger
}

// NewAuthGateway creates a new auth gateway
func NewAuthGateway(authClient auth.AuthClient, logger *slog.Logger) auth_port.AuthPort {
	return &AuthGateway{
		authClient: authClient,
		logger:     logger,
	}
}

func (g *AuthGateway) ValidateSession(ctx context.Context, sessionToken string) (*domain.UserContext, error) {
	// Safely log session token prefix
	tokenPrefix := sessionToken
	if len(sessionToken) > 10 {
		tokenPrefix = sessionToken[:10] + "..."
	}
	g.logger.Debug("validating session", "token_prefix", tokenPrefix)
	
	response, err := g.authClient.ValidateSession(ctx, sessionToken, "")
	if err != nil {
		return nil, fmt.Errorf("session validation failed: %w", err)
	}

	if !response.Valid {
		return nil, fmt.Errorf("session is invalid")
	}

	// auth-serviceのユーザー情報をdomain.UserContextに変換
	userID, err := uuid.Parse(response.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID format: %w", err)
	}

	userContext := &domain.UserContext{
		UserID:    userID,
		Email:     response.Email,
		Role:      domain.UserRole(response.Role),
		SessionID: sessionToken,
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	return userContext, nil
}

func (g *AuthGateway) ValidateSessionWithCookie(ctx context.Context, cookieHeader string) (*domain.UserContext, error) {
	g.logger.Debug("validating session with cookie", "cookie_length", len(cookieHeader))
	
	response, err := g.authClient.ValidateSessionWithCookie(ctx, cookieHeader)
	if err != nil {
		return nil, fmt.Errorf("session validation failed: %w", err)
	}

	if !response.Valid {
		return nil, fmt.Errorf("session is invalid")
	}

	// auth-serviceのユーザー情報をdomain.UserContextに変換
	userID, err := uuid.Parse(response.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID format: %w", err)
	}

	userContext := &domain.UserContext{
		UserID:    userID,
		Email:     response.Email,
		Role:      domain.UserRole(response.Role),
		SessionID: cookieHeader, // For cookie auth, store the cookie header
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	return userContext, nil
}

func (g *AuthGateway) RefreshSession(ctx context.Context, sessionToken string) (*domain.UserContext, error) {
	// セッション更新ロジック
	return g.ValidateSession(ctx, sessionToken)
}

func (g *AuthGateway) GetUserByID(ctx context.Context, userID string) (*domain.UserContext, error) {
	// TODO: auth-serviceにユーザーIDでユーザー取得APIが実装されたら実装
	return nil, fmt.Errorf("GetUserByID not implemented yet")
}
