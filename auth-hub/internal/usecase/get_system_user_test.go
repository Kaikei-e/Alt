package usecase

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockIdentityProvider implements domain.IdentityProvider for testing.
type mockIdentityProvider struct {
	userID string
	err    error
}

func (m *mockIdentityProvider) GetFirstIdentityID(_ context.Context) (string, error) {
	return m.userID, m.err
}

func TestGetSystemUser_Success(t *testing.T) {
	provider := &mockIdentityProvider{userID: "system-user-001"}
	logger := slog.Default()

	uc := NewGetSystemUser(provider, logger)
	userID, err := uc.Execute(context.Background())

	assert.NoError(t, err)
	assert.Equal(t, "system-user-001", userID)
}

func TestGetSystemUser_ProviderError(t *testing.T) {
	provider := &mockIdentityProvider{err: errors.New("admin API not configured")}
	logger := slog.Default()

	uc := NewGetSystemUser(provider, logger)
	userID, err := uc.Execute(context.Background())

	assert.Error(t, err)
	assert.Empty(t, userID)
	assert.Contains(t, err.Error(), "admin API not configured")
}
