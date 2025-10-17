package csrf_token_usecase

import (
	"context"
	"fmt"
	"strings"
)

// CSRFTokenGateway defines the interface for CSRF token gateway operations
type CSRFTokenGateway interface {
	GenerateToken(ctx context.Context) (string, error)
	ValidateToken(ctx context.Context, token string) (bool, error)
	ValidateHMACToken(ctx context.Context, token string, sessionID string, secret string) (bool, error)
	InvalidateToken(ctx context.Context, token string) error
}

// CSRFTokenUsecase handles CSRF token operations
type CSRFTokenUsecase struct {
	gateway    CSRFTokenGateway
	csrfSecret string
}

// NewCSRFTokenUsecase creates a new CSRF token usecase
func NewCSRFTokenUsecase(gateway CSRFTokenGateway) *CSRFTokenUsecase {
	return &CSRFTokenUsecase{
		gateway:    gateway,
		csrfSecret: getCSRFSecret(),
	}
}

// NewCSRFTokenUsecaseWithSecret creates a new CSRF token usecase with explicit secret
func NewCSRFTokenUsecaseWithSecret(gateway CSRFTokenGateway, secret string) *CSRFTokenUsecase {
	return &CSRFTokenUsecase{
		gateway:    gateway,
		csrfSecret: secret,
	}
}

// getCSRFSecret retrieves CSRF secret from environment or uses default for development
func getCSRFSecret() string {
	// This will be loaded from environment variable in production
	// For now, return a development default
	return "development-csrf-secret-change-in-production"
}

// GenerateToken generates a new CSRF token
func (u *CSRFTokenUsecase) GenerateToken(ctx context.Context) (string, error) {
	token, err := u.gateway.GenerateToken(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to generate CSRF token: %w", err)
	}

	return token, nil
}

// ValidateToken validates a CSRF token
func (u *CSRFTokenUsecase) ValidateToken(ctx context.Context, token string) (bool, error) {
	// Reject empty tokens
	if strings.TrimSpace(token) == "" {
		return false, nil
	}

	valid, err := u.gateway.ValidateToken(ctx, token)
	if err != nil {
		return false, fmt.Errorf("failed to validate CSRF token: %w", err)
	}

	return valid, nil
}

// InvalidateToken invalidates a CSRF token
func (u *CSRFTokenUsecase) InvalidateToken(ctx context.Context, token string) error {
	// Ignore empty tokens
	if strings.TrimSpace(token) == "" {
		return nil
	}

	err := u.gateway.InvalidateToken(ctx, token)
	if err != nil {
		return fmt.Errorf("failed to invalidate CSRF token: %w", err)
	}

	return nil
}

// ValidateTokenWithSession validates a CSRF token using session-based HMAC validation
// Falls back to random token validation if session ID is not provided
func (u *CSRFTokenUsecase) ValidateTokenWithSession(ctx context.Context, token string, sessionID string) (bool, error) {
	// Reject empty tokens
	if strings.TrimSpace(token) == "" {
		return false, nil
	}

	// If session ID is provided, try HMAC validation first
	if strings.TrimSpace(sessionID) != "" {
		valid, err := u.gateway.ValidateHMACToken(ctx, token, sessionID, u.csrfSecret)
		if err != nil {
			return false, fmt.Errorf("failed to validate HMAC CSRF token: %w", err)
		}
		return valid, nil
	}

	// Fallback to random token validation for backward compatibility
	valid, err := u.gateway.ValidateToken(ctx, token)
	if err != nil {
		return false, fmt.Errorf("failed to validate CSRF token: %w", err)
	}

	return valid, nil
}
