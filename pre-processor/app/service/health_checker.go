package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// HealthCheckerService implementation.
type healthCheckerService struct {
	logger         *slog.Logger
	client         *http.Client
	newsCreatorURL string
}

// NewHealthCheckerService creates a new health checker service.
func NewHealthCheckerService(newsCreatorURL string, logger *slog.Logger) HealthCheckerService {
	return &healthCheckerService{
		logger:         logger,
		newsCreatorURL: newsCreatorURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CheckNewsCreatorHealth checks if news creator service is healthy.
func (s *healthCheckerService) CheckNewsCreatorHealth(ctx context.Context) error {
	s.logger.Debug("checking news creator health", "url", s.newsCreatorURL)

	// IMPROVED: Check if models are actually loaded, not just if service is up
	healthURL := s.newsCreatorURL + "/api/tags"

	resp, err := s.client.Get(healthURL)
	if err != nil {
		s.logger.Error("failed to check news creator health", "error", err)
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.logger.Error("news creator not healthy", "status", resp.StatusCode)
		return fmt.Errorf("news creator not healthy: status %d", resp.StatusCode)
	}

	// Check if models are loaded
	var response struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		s.logger.Error("failed to decode health response", "error", err)
		return fmt.Errorf("failed to decode health response: %w", err)
	}

	// Service is healthy only if models are loaded
	if len(response.Models) == 0 {
		s.logger.Warn("news creator service is up but no models are loaded")
		return fmt.Errorf("no models loaded in news creator service")
	}

	s.logger.Debug("news creator is healthy", "models", len(response.Models))
	return nil
}

// WaitForHealthy waits for the news creator service to become healthy.
func (s *healthCheckerService) WaitForHealthy(ctx context.Context) error {
	s.logger.Debug("waiting for news creator to become healthy")

	// Simple fix: just do immediate health checks with faster retries
	// This avoids the complexity of shared state and works for the current use case

	// First check if already healthy
	if err := s.CheckNewsCreatorHealth(ctx); err == nil {
		s.logger.Debug("news creator is now healthy")
		return nil
	}

	// Poll every 10 seconds instead of 30
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Error("context canceled while waiting for health")
			return ctx.Err()
		case <-ticker.C:
			if err := s.CheckNewsCreatorHealth(ctx); err == nil {
				s.logger.Debug("news creator is now healthy")
				return nil
			}
			// Don't log "still not healthy" - too noisy
		}
	}
}
