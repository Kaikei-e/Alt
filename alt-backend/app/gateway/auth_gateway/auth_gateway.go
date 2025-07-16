package auth_gateway

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"alt/domain"
	"alt/port/auth_port"
)

// AuthGateway implements the auth port interface using the auth driver
type AuthGateway struct {
	authClient auth_port.AuthClient
	logger     *slog.Logger
}

// NewAuthGateway creates a new auth gateway
func NewAuthGateway(authClient auth_port.AuthClient, logger *slog.Logger) *AuthGateway {
	return &AuthGateway{
		authClient: authClient,
		logger:     logger,
	}
}

// ValidateSession validates a session token and returns user context
func (g *AuthGateway) ValidateSession(ctx context.Context, sessionToken string, tenantID string) (*auth_port.SessionValidationResponse, error) {
	if sessionToken == "" {
		return nil, fmt.Errorf("session token is required")
	}

	g.logger.Debug("validating session",
		"session_token_prefix", sessionToken[:min(len(sessionToken), 8)],
		"tenant_id", tenantID)

	response, err := g.authClient.ValidateSession(ctx, sessionToken, tenantID)
	if err != nil {
		g.logger.Error("session validation failed",
			"error", err,
			"tenant_id", tenantID)
		return nil, fmt.Errorf("failed to validate session: %w", err)
	}

	// Convert auth client response to port response
	result := &auth_port.SessionValidationResponse{
		Valid:  response.Valid,
		UserID: response.UserID,
		Email:  response.Email,
		Role:   response.Role,
	}

	// If session is valid, enrich with domain context
	if response.Valid && response.Context != nil {
		result.Context = response.Context
	} else if response.Valid {
		// Create a basic user context if auth service didn't provide one
		userID, err := uuid.Parse(response.UserID)
		if err != nil {
			g.logger.Warn("invalid user ID format", "user_id", response.UserID)
			userID = uuid.New()
		}

		var tenantUUID uuid.UUID
		if tenantID != "" {
			if parsedTenant, err := uuid.Parse(tenantID); err == nil {
				tenantUUID = parsedTenant
			}
		}

		result.Context = &domain.UserContext{
			UserID:      userID,
			Email:       response.Email,
			Role:        domain.UserRole(response.Role),
			TenantID:    tenantUUID,
			SessionID:   sessionToken,
			LoginAt:     time.Now(),
			ExpiresAt:   time.Now().Add(24 * time.Hour), // Default 24h expiry
			Permissions: []string{},
		}
	}

	g.logger.Debug("session validation successful",
		"valid", result.Valid,
		"user_id", result.UserID)

	return result, nil
}

// GenerateCSRFToken generates a CSRF token for the given session
func (g *AuthGateway) GenerateCSRFToken(ctx context.Context, sessionToken string) (*auth_port.CSRFTokenResponse, error) {
	if sessionToken == "" {
		return nil, fmt.Errorf("session token is required")
	}

	g.logger.Debug("generating CSRF token",
		"session_token_prefix", sessionToken[:min(len(sessionToken), 8)])

	response, err := g.authClient.GenerateCSRFToken(ctx, sessionToken)
	if err != nil {
		g.logger.Error("CSRF token generation failed", "error", err)
		return nil, fmt.Errorf("failed to generate CSRF token: %w", err)
	}

	result := &auth_port.CSRFTokenResponse{
		Token:     response.Token,
		ExpiresAt: response.ExpiresAt,
	}

	g.logger.Debug("CSRF token generated successfully",
		"token_prefix", result.Token[:min(len(result.Token), 8)])

	return result, nil
}

// ValidateCSRFToken validates a CSRF token with the given session
func (g *AuthGateway) ValidateCSRFToken(ctx context.Context, token, sessionToken string) (*auth_port.CSRFValidationResponse, error) {
	if token == "" {
		return nil, fmt.Errorf("CSRF token is required")
	}
	if sessionToken == "" {
		return nil, fmt.Errorf("session token is required")
	}

	g.logger.Debug("validating CSRF token",
		"csrf_token_prefix", token[:min(len(token), 8)],
		"session_token_prefix", sessionToken[:min(len(sessionToken), 8)])

	response, err := g.authClient.ValidateCSRFToken(ctx, token, sessionToken)
	if err != nil {
		g.logger.Error("CSRF token validation failed", "error", err)
		return nil, fmt.Errorf("failed to validate CSRF token: %w", err)
	}

	result := &auth_port.CSRFValidationResponse{
		Valid: response.Valid,
	}

	g.logger.Debug("CSRF token validation completed",
		"valid", result.Valid)

	return result, nil
}

// HealthCheck checks if the Auth Service is healthy
func (g *AuthGateway) HealthCheck(ctx context.Context) error {
	g.logger.Debug("checking auth service health")

	err := g.authClient.HealthCheck(ctx)
	if err != nil {
		g.logger.Error("auth service health check failed", "error", err)
		return fmt.Errorf("auth service health check failed: %w", err)
	}

	g.logger.Debug("auth service health check successful")
	return nil
}

// min returns the minimum of two integers (helper function for Go versions < 1.21)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
