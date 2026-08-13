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
// cookieValue is the bearer credential and is used only as the in-process cache
// key; the session id that reaches the sid claim and the JSON response body is
// the identity provider's stable session id.
func (uc *GetSession) Execute(ctx context.Context, cookieValue string) (*SessionResult, error) {
	var identity *domain.Identity
	var tenantID string
	var createdAt time.Time

	var role string

	// Check cache first
	if cached, found := uc.cache.Get(cookieValue); found {
		identity = &domain.Identity{
			UserID:    cached.UserID,
			TenantID:  cached.TenantID,
			Email:     cached.Email,
			Role:      cached.Role,
			SessionID: cached.SessionID,
		}
		tenantID = cached.TenantID
		role = cached.Role
		createdAt = cached.CreatedAt
	} else {
		// Cache miss – validate with Kratos
		fullCookie := fmt.Sprintf("ory_kratos_session=%s", cookieValue)
		kratosIdentity, err := uc.validator.ValidateSession(ctx, fullCookie)
		if err != nil {
			return nil, err
		}

		identity = kratosIdentity
		tenantID = identity.UserID // Single-tenant: tenant == user
		identity.TenantID = tenantID
		role = identity.Role
		createdAt = identity.CreatedAt

		// Populate cache
		uc.cache.Set(cookieValue, domain.CachedSession{
			UserID:    identity.UserID,
			TenantID:  tenantID,
			Email:     identity.Email,
			Role:      identity.Role,
			SessionID: identity.SessionID,
			CreatedAt: identity.CreatedAt,
		})
	}

	// Generate backend JWT
	backendToken, err := uc.token.IssueBackendToken(identity, identity.SessionID)
	if err != nil {
		uc.logger.ErrorContext(ctx, "failed to issue backend token", "error", err)
		return nil, fmt.Errorf("%w: %w", domain.ErrTokenGeneration, err)
	}

	if role == "" {
		role = "user"
	}

	return &SessionResult{
		UserID:       identity.UserID,
		TenantID:     tenantID,
		Email:        identity.Email,
		Role:         role,
		SessionID:    identity.SessionID,
		CreatedAt:    createdAt,
		BackendToken: backendToken,
	}, nil
}
