package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"auth-hub/internal/domain"
)

// GenerateCSRF orchestrates CSRF token generation for an authenticated session.
type GenerateCSRF struct {
	validator domain.SessionValidator
	csrf      domain.CSRFTokenGenerator
	logger    *slog.Logger
}

// NewGenerateCSRF creates a new GenerateCSRF usecase.
func NewGenerateCSRF(v domain.SessionValidator, csrf domain.CSRFTokenGenerator, l *slog.Logger) *GenerateCSRF {
	return &GenerateCSRF{validator: v, csrf: csrf, logger: l}
}

// Execute validates the session cookie and generates a CSRF token.
func (uc *GenerateCSRF) Execute(ctx context.Context, rawCookie string, sessionID string) (string, error) {
	if rawCookie == "" {
		return "", domain.ErrSessionNotFound
	}

	// Validate session with Kratos
	_, err := uc.validator.ValidateSession(ctx, rawCookie)
	if err != nil {
		return "", fmt.Errorf("%w: %w", domain.ErrAuthFailed, err)
	}

	if sessionID == "" {
		return "", domain.ErrSessionNotFound
	}

	// Generate CSRF token
	token, err := uc.csrf.Generate(sessionID)
	if err != nil {
		uc.logger.ErrorContext(ctx, "failed to generate CSRF token", "error", err)
		return "", fmt.Errorf("%w: %w", domain.ErrCSRFSecretMissing, err)
	}

	return token, nil
}
