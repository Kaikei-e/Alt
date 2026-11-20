package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"auth-hub/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCSRFGenerator implements domain.CSRFTokenGenerator for testing.
type mockCSRFGenerator struct {
	issued string
	err    error
}

func (m *mockCSRFGenerator) Generate(_ string) (string, error) {
	return m.issued, m.err
}

func TestGenerateCSRF_Success(t *testing.T) {
	validator := &mockValidator{
		identity: &domain.Identity{UserID: "user-123"},
	}
	csrf := &mockCSRFGenerator{issued: "csrf-token-abc"}
	logger := slog.Default()

	uc := NewGenerateCSRF(validator, csrf, logger)
	token, err := uc.Execute(context.Background(), "ory_kratos_session=sess-123", "sess-123")

	require.NoError(t, err)
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
	assert.ErrorIs(t, err, domain.ErrSessionNotFound)
}

func TestGenerateCSRF_InvalidSession(t *testing.T) {
	validator := &mockValidator{err: domain.ErrAuthFailed}
	csrf := &mockCSRFGenerator{}
	logger := slog.Default()

	uc := NewGenerateCSRF(validator, csrf, logger)
	token, err := uc.Execute(context.Background(), "ory_kratos_session=invalid", "invalid")

	assert.Empty(t, token)
	assert.ErrorIs(t, err, domain.ErrAuthFailed)
}

func TestGenerateCSRF_PropagatesUpstreamErrors(t *testing.T) {
	tests := []struct {
		name         string
		validatorErr error
		wantTarget   error
	}{
		{
			name:         "kratos unavailable is not masked as ErrAuthFailed",
			validatorErr: fmt.Errorf("%w: kratos returned status 503", domain.ErrKratosUnavailable),
			wantTarget:   domain.ErrKratosUnavailable,
		},
		{
			name:         "rate limited is not masked as ErrAuthFailed",
			validatorErr: fmt.Errorf("%w: kratos returned status 429", domain.ErrRateLimited),
			wantTarget:   domain.ErrRateLimited,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &mockValidator{err: tt.validatorErr}
			csrf := &mockCSRFGenerator{issued: "unused"}
			uc := NewGenerateCSRF(validator, csrf, slog.Default())

			token, err := uc.Execute(context.Background(), "ory_kratos_session=sess-123", "sess-123")

			require.Error(t, err)
			assert.Empty(t, token)
			require.ErrorIs(t, err, tt.wantTarget)
			assert.NotErrorIs(t, err, domain.ErrAuthFailed)
		})
	}
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
	assert.ErrorIs(t, err, domain.ErrSessionNotFound)
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
	assert.ErrorIs(t, err, domain.ErrCSRFSecretMissing)
}
