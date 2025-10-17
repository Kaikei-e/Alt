package csrf_token_gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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

// GenerateHMACToken generates a CSRF token from session ID using HMAC-SHA256
// This is a public method to allow testing and reuse across the application
func (g *CSRFTokenGateway) GenerateHMACToken(sessionID string, secret string) string {
	if sessionID == "" || secret == "" {
		return ""
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(sessionID))
	hash := mac.Sum(nil)

	// Return base64 URL-encoded HMAC
	return base64.URLEncoding.EncodeToString(hash)
}

// ValidateHMACToken validates a CSRF token using HMAC-SHA256 with session ID
// Uses constant-time comparison to prevent timing attacks
func (g *CSRFTokenGateway) ValidateHMACToken(ctx context.Context, token string, sessionID string, secret string) (bool, error) {
	// Reject empty inputs
	if sessionID == "" || token == "" {
		return false, nil
	}

	// Generate expected token from session ID
	expectedToken := g.GenerateHMACToken(sessionID, secret)
	if expectedToken == "" {
		return false, nil
	}

	// Use constant-time comparison to prevent timing attacks
	// hmac.Equal uses crypto/subtle.ConstantTimeCompare internally
	return hmac.Equal([]byte(token), []byte(expectedToken)), nil
}
