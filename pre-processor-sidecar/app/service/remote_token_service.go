// ABOUTME: RemoteTokenService implements TokenProvider by fetching tokens from centralized auth-token-manager
// ABOUTME: Replaces the complex SimpleTokenService for sidecar operation

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"pre-processor-sidecar/models"
	"pre-processor-sidecar/repository"
)

// ErrTokenUnavailable is the typed sentinel returned by RemoteTokenService.GetValidToken
// when auth-token-manager has no token to hand out (HTTP 404 or empty access_token).
// Surfacing this as a typed error lets callers and observability hooks distinguish
// "token source is empty / waiting for re-auth" from generic transport errors.
var ErrTokenUnavailable = errors.New("token unavailable from auth-token-manager")

// Clock abstracts time.Now for deterministic tests.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// degradedWindow is the rolling window for the failure counter that drives IsDegraded().
const degradedWindow = 60 * time.Second

// degradedThreshold is how many failures within the window flip IsDegraded() to true.
const degradedThreshold = 3

// RemoteTokenService implements TokenProvider utilizing RemoteTokenRepository
type RemoteTokenService struct {
	repo   *repository.RemoteTokenRepository
	logger *slog.Logger
	clock  Clock

	mu       sync.Mutex
	failures []time.Time
}

// NewRemoteTokenService creates a new remote token service with the wall clock.
func NewRemoteTokenService(repo *repository.RemoteTokenRepository, logger *slog.Logger) *RemoteTokenService {
	return NewRemoteTokenServiceWithClock(repo, logger, realClock{})
}

// NewRemoteTokenServiceWithClock creates a new remote token service with an injectable clock.
func NewRemoteTokenServiceWithClock(repo *repository.RemoteTokenRepository, logger *slog.Logger, clock Clock) *RemoteTokenService {
	if clock == nil {
		clock = realClock{}
	}
	return &RemoteTokenService{
		repo:   repo,
		logger: logger,
		clock:  clock,
	}
}

// GetValidToken retrieves a valid token from the remote service
// The remote service (auth-token-manager) is responsible for ensuring validity and refreshing
func (s *RemoteTokenService) GetValidToken(ctx context.Context) (*models.OAuth2Token, error) {
	token, err := s.repo.GetCurrentToken(ctx)
	if err != nil {
		s.logger.Error("Failed to fetch token from remote service", "error", err)
		s.recordFailure()
		if errors.Is(err, repository.ErrTokenNotFound) {
			return nil, fmt.Errorf("%w: %v", ErrTokenUnavailable, err)
		}
		return nil, fmt.Errorf("remote token fetch failed: %w", err)
	}
	s.recordSuccess()
	return token, nil
}

// EnsureValidToken alias for GetValidToken as remote service handles validation
func (s *RemoteTokenService) EnsureValidToken(ctx context.Context) (*models.OAuth2Token, error) {
	return s.GetValidToken(ctx)
}

// IsDegraded reports whether the token source has produced at least degradedThreshold
// failures inside the most recent degradedWindow. Used by /admin/health to surface a
// silent-failure signal without polling internal repository state.
func (s *RemoteTokenService) IsDegraded() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpiredLocked()
	return len(s.failures) >= degradedThreshold
}

// TokenAvailable is the inverse signal used by /admin/health. It returns true while the
// token source is healthy (i.e., not in degraded state).
func (s *RemoteTokenService) TokenAvailable() bool {
	return !s.IsDegraded()
}

func (s *RemoteTokenService) recordFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = append(s.failures, s.clock.Now())
	s.evictExpiredLocked()
}

func (s *RemoteTokenService) recordSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = nil
}

func (s *RemoteTokenService) evictExpiredLocked() {
	if len(s.failures) == 0 {
		return
	}
	cutoff := s.clock.Now().Add(-degradedWindow)
	keep := s.failures[:0]
	for _, t := range s.failures {
		if t.After(cutoff) {
			keep = append(keep, t)
		}
	}
	s.failures = keep
}
