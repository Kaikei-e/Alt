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
	InvalidateToken(ctx context.Context, token string) error
}

// CSRFTokenUsecase handles CSRF token operations
type CSRFTokenUsecase struct {
	gateway CSRFTokenGateway
}

// NewCSRFTokenUsecase creates a new CSRF token usecase
func NewCSRFTokenUsecase(gateway CSRFTokenGateway) *CSRFTokenUsecase {
	return &CSRFTokenUsecase{
		gateway: gateway,
	}
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