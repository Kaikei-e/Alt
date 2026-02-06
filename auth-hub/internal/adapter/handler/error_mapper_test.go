package handler

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"auth-hub/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestMapDomainError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"session not found", domain.ErrSessionNotFound, http.StatusUnauthorized},
		{"auth failed", domain.ErrAuthFailed, http.StatusUnauthorized},
		{"session expired", domain.ErrSessionExpired, http.StatusUnauthorized},
		{"session inactive", domain.ErrSessionInactive, http.StatusUnauthorized},
		{"missing identity", domain.ErrMissingIdentity, http.StatusUnauthorized},
		{"kratos unavailable", domain.ErrKratosUnavailable, http.StatusBadGateway},
		{"admin not configured", domain.ErrAdminNotConfigured, http.StatusInternalServerError},
		{"no identities found", domain.ErrNoIdentitiesFound, http.StatusInternalServerError},
		{"token generation", domain.ErrTokenGeneration, http.StatusInternalServerError},
		{"csrf secret missing", domain.ErrCSRFSecretMissing, http.StatusInternalServerError},
		{"backend secret weak", domain.ErrBackendSecretWeak, http.StatusInternalServerError},
		{"rate limited", domain.ErrRateLimited, http.StatusTooManyRequests},
		{"unknown error", errors.New("something unexpected"), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpErr := mapDomainError(tt.err)
			assert.Equal(t, tt.wantCode, httpErr.Code)
		})
	}
}

func TestMapDomainError_WrappedErrors(t *testing.T) {
	// Wrapped domain errors should still be detected
	wrapped := fmt.Errorf("context: %w", domain.ErrAuthFailed)
	httpErr := mapDomainError(wrapped)
	assert.Equal(t, http.StatusUnauthorized, httpErr.Code)

	// Double-wrapped
	doubleWrapped := fmt.Errorf("outer: %w", wrapped)
	httpErr2 := mapDomainError(doubleWrapped)
	assert.Equal(t, http.StatusUnauthorized, httpErr2.Code)
}

func TestMapDomainError_ReturnsEchoHTTPError(t *testing.T) {
	httpErr := mapDomainError(domain.ErrRateLimited)
	assert.NotNil(t, httpErr)
	assert.Equal(t, http.StatusTooManyRequests, httpErr.Code)
}
