package usecase

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"auth-hub/internal/domain"

	"github.com/stretchr/testify/assert"
)

// mockCSRFGenerator implements domain.CSRFTokenGenerator for testing.
type mockCSRFGenerator struct {
	token string
	err   error
}

func (m *mockCSRFGenerator) Generate(_ string) (string, error) {
	return m.token, m.err
}

func TestGenerateCSRF_Success(t *testing.T) {
	validator := &mockValidator{
		identity: &domain.Identity{UserID: "user-123"},
	}
	csrf := &mockCSRFGenerator{token: "csrf-token-abc"}
	logger := slog.Default()

	uc := NewGenerateCSRF(validator, csrf, logger)
	token, err := uc.Execute(context.Background(), "ory_kratos_session=sess-123", "sess-123")

	assert.NoError(t, err)
	assert.Equal(t, "csrf-token-abc", token)
	assert.True(t, validator.called)
}

func TestGenerateCSRF_EmptyCookie(t *testing.T) {
	validator := &mockValidator{}
	csrf := &mockCSRFGenerator{}
	logger := slog.Default()

	uc := NewGenerateCSRF(validator, csrf, logger)
	token, err := uc.Execute(context.Background(), "", "")

	assert.Empty(t, token)
	assert.True(t, errors.Is(err, domain.ErrSessionNotFound))
}

func TestGenerateCSRF_InvalidSession(t *testing.T) {
	validator := &mockValidator{err: domain.ErrAuthFailed}
	csrf := &mockCSRFGenerator{}
	logger := slog.Default()

	uc := NewGenerateCSRF(validator, csrf, logger)
	token, err := uc.Execute(context.Background(), "ory_kratos_session=invalid", "invalid")

	assert.Empty(t, token)
	assert.True(t, errors.Is(err, domain.ErrAuthFailed))
}

func TestGenerateCSRF_EmptySessionID(t *testing.T) {
	validator := &mockValidator{
		identity: &domain.Identity{UserID: "user-123"},
	}
	csrf := &mockCSRFGenerator{}
	logger := slog.Default()

	uc := NewGenerateCSRF(validator, csrf, logger)
	token, err := uc.Execute(context.Background(), "other=value", "")

	assert.Empty(t, token)
	assert.True(t, errors.Is(err, domain.ErrSessionNotFound))
}

func TestGenerateCSRF_GeneratorError(t *testing.T) {
	validator := &mockValidator{
		identity: &domain.Identity{UserID: "user-123"},
	}
	csrf := &mockCSRFGenerator{err: errors.New("hmac error")}
	logger := slog.Default()

	uc := NewGenerateCSRF(validator, csrf, logger)
	token, err := uc.Execute(context.Background(), "ory_kratos_session=sess-123", "sess-123")

	assert.Empty(t, token)
	assert.True(t, errors.Is(err, domain.ErrCSRFSecretMissing))
}
