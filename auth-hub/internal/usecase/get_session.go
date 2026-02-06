package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"auth-hub/internal/domain"
)

// SessionResult holds the data returned by GetSession.
type SessionResult struct {
	UserID       string
	TenantID     string
	Email        string
	Role         string
	SessionID    string
	CreatedAt    time.Time
	BackendToken string
}

// GetSession orchestrates session retrieval with JWT generation for frontend consumption.
type GetSession struct {
	validator domain.SessionValidator
	cache     domain.SessionCache
	token     domain.TokenIssuer
	logger    *slog.Logger
}

// NewGetSession creates a new GetSession usecase.
func NewGetSession(v domain.SessionValidator, c domain.SessionCache, t domain.TokenIssuer, l *slog.Logger) *GetSession {
	return &GetSession{validator: v, cache: c, token: t, logger: l}
}

// Execute validates the session and generates a backend JWT token.
func (uc *GetSession) Execute(ctx context.Context, cookieValue string) (*SessionResult, error) {
	var identity *domain.Identity
	var tenantID string
	var createdAt time.Time

	// Check cache first
	if cached, found := uc.cache.Get(cookieValue); found {
		identity = &domain.Identity{
			UserID:    cached.UserID,
			Email:     cached.Email,
			SessionID: cookieValue,
		}
		tenantID = cached.TenantID
		createdAt = time.Now().Add(-24 * time.Hour) // Approximate from cache
	} else {
		// Cache miss – validate with Kratos
		fullCookie := fmt.Sprintf("ory_kratos_session=%s", cookieValue)
		kratosIdentity, err := uc.validator.ValidateSession(ctx, fullCookie)
		if err != nil {
			return nil, err
		}

		identity = kratosIdentity
		identity.SessionID = cookieValue
		tenantID = identity.UserID // Single-tenant
		createdAt = identity.CreatedAt

		// Populate cache
		uc.cache.Set(cookieValue, domain.CachedSession{
			UserID:   identity.UserID,
			TenantID: tenantID,
			Email:    identity.Email,
		})
	}

	// Generate backend JWT
	backendToken, err := uc.token.IssueBackendToken(identity, cookieValue)
	if err != nil {
		uc.logger.ErrorContext(ctx, "failed to issue backend token", "error", err)
		return nil, fmt.Errorf("%w: %w", domain.ErrTokenGeneration, err)
	}

	return &SessionResult{
		UserID:       identity.UserID,
		TenantID:     tenantID,
		Email:        identity.Email,
		Role:         "user", // Default role
		SessionID:    cookieValue,
		CreatedAt:    createdAt,
		BackendToken: backendToken,
	}, nil
}
