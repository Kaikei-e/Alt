// ABOUTME: RemoteTokenService implements TokenProvider by fetching tokens from centralized auth-token-manager
// ABOUTME: Replaces the complex SimpleTokenService for sidecar operation

package service

import (
	"context"
	"fmt"
	"log/slog"
	"pre-processor-sidecar/models"
	"pre-processor-sidecar/repository"
)

// RemoteTokenService implements TokenProvider utilizing RemoteTokenRepository
type RemoteTokenService struct {
	repo   *repository.RemoteTokenRepository
	logger *slog.Logger
}

// NewRemoteTokenService creates a new remote token service
func NewRemoteTokenService(repo *repository.RemoteTokenRepository, logger *slog.Logger) *RemoteTokenService {
	return &RemoteTokenService{
		repo:   repo,
		logger: logger,
	}
}

// GetValidToken retrieves a valid token from the remote service
// The remote service (auth-token-manager) is responsible for ensuring validity and refreshing
func (s *RemoteTokenService) GetValidToken(ctx context.Context) (*models.OAuth2Token, error) {
	token, err := s.repo.GetCurrentToken(ctx)
	if err != nil {
		s.logger.Error("Failed to fetch token from remote service", "error", err)
		return nil, fmt.Errorf("remote token fetch failed: %w", err)
	}
	return token, nil
}

// EnsureValidToken alias for GetValidToken as remote service handles validation
func (s *RemoteTokenService) EnsureValidToken(ctx context.Context) (*models.OAuth2Token, error) {
	return s.GetValidToken(ctx)
}
