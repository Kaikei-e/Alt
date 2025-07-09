package csrf_token_gateway

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CSRFTokenDriver defines the interface for CSRF token driver operations
type CSRFTokenDriver interface {
	StoreToken(ctx context.Context, token string, expiration time.Time) error
	GetToken(ctx context.Context, token string) (time.Time, error)
	DeleteToken(ctx context.Context, token string) error
	GenerateRandomToken() (string, error)
}

// CSRFTokenGateway handles CSRF token operations at the gateway layer
type CSRFTokenGateway struct {
	driver CSRFTokenDriver
}

// NewCSRFTokenGateway creates a new CSRF token gateway
func NewCSRFTokenGateway(driver CSRFTokenDriver) *CSRFTokenGateway {
	return &CSRFTokenGateway{
		driver: driver,
	}
}

// GenerateToken generates a new CSRF token and stores it
func (g *CSRFTokenGateway) GenerateToken(ctx context.Context) (string, error) {
	// Generate random token
	token, err := g.driver.GenerateRandomToken()
	if err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	// Set expiration to 1 hour from now
	expiration := time.Now().Add(1 * time.Hour)

	// Store token with expiration
	err = g.driver.StoreToken(ctx, token, expiration)
	if err != nil {
		return "", fmt.Errorf("failed to store CSRF token: %w", err)
	}

	return token, nil
}

// ValidateToken validates a CSRF token
func (g *CSRFTokenGateway) ValidateToken(ctx context.Context, token string) (bool, error) {
	// Reject empty tokens
	if strings.TrimSpace(token) == "" {
		return false, nil
	}

	// Get token expiration from storage
	expiration, err := g.driver.GetToken(ctx, token)
	if err != nil {
		// Token not found or driver error - consider invalid
		return false, nil
	}

	// Check if token is expired
	if time.Now().After(expiration) {
		// Token is expired, delete it
		_ = g.driver.DeleteToken(ctx, token) // Ignore deletion errors
		return false, nil
	}

	return true, nil
}

// InvalidateToken invalidates a CSRF token
func (g *CSRFTokenGateway) InvalidateToken(ctx context.Context, token string) error {
	// Ignore empty tokens
	if strings.TrimSpace(token) == "" {
		return nil
	}

	err := g.driver.DeleteToken(ctx, token)
	if err != nil {
		return fmt.Errorf("failed to delete CSRF token: %w", err)
	}

	return nil
}