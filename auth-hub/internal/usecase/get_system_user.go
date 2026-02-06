package usecase

import (
	"context"
	"log/slog"

	"auth-hub/internal/domain"
)

// GetSystemUser retrieves the system user ID from the identity provider.
type GetSystemUser struct {
	provider domain.IdentityProvider
	logger   *slog.Logger
}

// NewGetSystemUser creates a new GetSystemUser usecase.
func NewGetSystemUser(p domain.IdentityProvider, l *slog.Logger) *GetSystemUser {
	return &GetSystemUser{provider: p, logger: l}
}

// Execute fetches the first identity ID for internal service operations.
func (uc *GetSystemUser) Execute(ctx context.Context) (string, error) {
	userID, err := uc.provider.GetFirstIdentityID(ctx)
	if err != nil {
		uc.logger.ErrorContext(ctx, "failed to fetch system user", "error", err)
		return "", err
	}
	return userID, nil
}
