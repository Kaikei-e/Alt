package handler

import (
	"context"
	"fmt"
	"log/slog"

	"pre-processor/service"
)

// HealthHandler implementation.
type healthHandler struct {
	healthChecker service.HealthCheckerService
	logger        *slog.Logger
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(healthChecker service.HealthCheckerService, logger *slog.Logger) HealthHandler {
	return &healthHandler{
		healthChecker: healthChecker,
		logger:        logger,
	}
}

// CheckHealth checks the health of the service.
func (h *healthHandler) CheckHealth(ctx context.Context) error {
	h.logger.Info("performing health check")

	// Check if we can perform basic operations
	// This is a simple implementation - in a real system you might check database connectivity, etc.
	h.logger.Info("health check completed - service is healthy")

	return nil
}

// CheckDependencies checks the health of external dependencies.
func (h *healthHandler) CheckDependencies(ctx context.Context) error {
	h.logger.Info("checking dependencies health")

	// Check news creator health
	if err := h.healthChecker.CheckNewsCreatorHealth(ctx); err != nil {
		h.logger.Error("news creator health check failed", "error", err)
		return fmt.Errorf("news creator health check failed: %w", err)
	}

	h.logger.Info("all dependencies are healthy")

	return nil
}
