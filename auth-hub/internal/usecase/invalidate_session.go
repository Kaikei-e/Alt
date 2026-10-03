package usecase

import (
	"context"
	"log/slog"

	"auth-hub/internal/domain"
)

// InvalidateSession purges a session from the in-memory cache upon logout.
type InvalidateSession struct {
	cache  domain.SessionCache
	logger *slog.Logger
}

// NewInvalidateSession creates a new InvalidateSession usecase.
func NewInvalidateSession(cache domain.SessionCache, logger *slog.Logger) *InvalidateSession {
	return &InvalidateSession{cache: cache, logger: logger}
}

// Execute removes the cached session matching cookieValue.
func (uc *InvalidateSession) Execute(ctx context.Context, cookieValue string) error {
	if cookieValue == "" {
		return domain.ErrSessionNotFound
	}
	uc.cache.Delete(cookieValue)
	uc.logger.InfoContext(ctx, "session cache invalidated")
	return nil
}
