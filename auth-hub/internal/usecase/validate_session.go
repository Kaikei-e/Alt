package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"auth-hub/internal/domain"
)

// ValidateSession orchestrates session validation with cache-through strategy.
type ValidateSession struct {
	validator domain.SessionValidator
	cache     domain.SessionCache
	logger    *slog.Logger
}

// NewValidateSession creates a new ValidateSession usecase.
func NewValidateSession(v domain.SessionValidator, c domain.SessionCache, l *slog.Logger) *ValidateSession {
	return &ValidateSession{validator: v, cache: c, logger: l}
}

// Execute validates the session identified by cookieValue.
// Returns the identity with TenantID set (single-tenant: TenantID == UserID).
func (uc *ValidateSession) Execute(ctx context.Context, cookieValue string) (*domain.Identity, error) {
	// Check cache first
	if cached, found := uc.cache.Get(cookieValue); found {
		return &domain.Identity{
			UserID:    cached.UserID,
			Email:     cached.Email,
			SessionID: cookieValue,
		}, nil
	}

	// Cache miss – validate with Kratos
	fullCookie := fmt.Sprintf("ory_kratos_session=%s", cookieValue)
	identity, err := uc.validator.ValidateSession(ctx, fullCookie)
	if err != nil {
		return nil, err
	}

	// Store in cache (single-tenant: TenantID == UserID)
	uc.cache.Set(cookieValue, domain.CachedSession{
		UserID:   identity.UserID,
		TenantID: identity.UserID,
		Email:    identity.Email,
	})

	identity.SessionID = cookieValue
	return identity, nil
}
