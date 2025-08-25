package auth_port

import (
	"context"

	"alt/domain"
)

//go:generate mockgen -source=auth_port.go -destination=../../mocks/mock_auth_port_interfaces.go -package=mocks

// AuthPort defines the interface for authentication operations
type AuthPort interface {
	ValidateSession(ctx context.Context, sessionToken string) (*domain.UserContext, error)
	ValidateSessionWithCookie(ctx context.Context, cookieHeader string) (*domain.UserContext, error)
	RefreshSession(ctx context.Context, sessionToken string) (*domain.UserContext, error)
	GetUserByID(ctx context.Context, userID string) (*domain.UserContext, error)
}

// AuthClientPort defines the port interface for authentication client operations
// This is separate from the driver implementation to maintain clean architecture
type AuthClientPort interface {
	// ValidateSession validates a session token and returns user context
	ValidateSession(ctx context.Context, sessionToken string, tenantID string) (*SessionValidationResponse, error)

	// ValidateSessionWithCookie validates a session using cookie header
	ValidateSessionWithCookie(ctx context.Context, cookieHeader string) (*SessionValidationResponse, error)

	// GenerateCSRFToken generates a CSRF token for the given session
	GenerateCSRFToken(ctx context.Context, sessionToken string) (*CSRFTokenResponse, error)

	// ValidateCSRFToken validates a CSRF token with the given session
	ValidateCSRFToken(ctx context.Context, token, sessionToken string) (*CSRFValidationResponse, error)

	// HealthCheck checks if the Auth Service is healthy
	HealthCheck(ctx context.Context) error
}

// SessionValidationResponse represents the response from session validation
type SessionValidationResponse struct {
	Valid   bool                `json:"valid"`
	UserID  string              `json:"user_id,omitempty"`
	Email   string              `json:"email,omitempty"`
	Role    string              `json:"role,omitempty"`
	Context *domain.UserContext `json:"context,omitempty"`
}

// CSRFTokenResponse represents the response containing a CSRF token
type CSRFTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

// CSRFValidationResponse represents the response from CSRF validation
type CSRFValidationResponse struct {
	Valid bool `json:"valid"`
}

// LegacyCSRFService defines the interface for existing CSRF implementation
type LegacyCSRFService interface {
	// Middleware returns the legacy CSRF middleware
	Middleware() func(next func(c interface{}) error) func(c interface{}) error

	// GenerateToken generates a legacy CSRF token
	GenerateToken(ctx context.Context) (string, error)

	// ValidateToken validates a legacy CSRF token
	ValidateToken(ctx context.Context, token string) (bool, error)
}
