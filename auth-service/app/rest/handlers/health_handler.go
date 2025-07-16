package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// HealthHandler handles health check HTTP requests
type HealthHandler struct {
	logger *slog.Logger
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(logger *slog.Logger) *HealthHandler {
	return &HealthHandler{
		logger: logger,
	}
}

// HealthCheck performs a basic health check
// @Summary Health check
// @Description Check if the service is healthy and running
// @Tags health
// @Accept json
// @Produce json
// @Success 200 {object} HealthResponse
// @Router /v1/health [get]
func (h *HealthHandler) HealthCheck(c echo.Context) error {
	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
		Service:   "auth-service",
		Version:   "1.0.0",
		Uptime:    time.Since(startTime).String(),
	}

	return c.JSON(http.StatusOK, response)
}

// ReadinessCheck performs a readiness check
// @Summary Readiness check
// @Description Check if the service is ready to serve traffic
// @Tags health
// @Accept json
// @Produce json
// @Success 200 {object} ReadinessResponse
// @Failure 503 {object} ErrorResponse
// @Router /v1/ready [get]
func (h *HealthHandler) ReadinessCheck(c echo.Context) error {
	// TODO: Add actual dependency checks (database, Kratos, etc.)
	checks := make(map[string]HealthStatus)

	// Database check (placeholder)
	checks["database"] = HealthStatus{
		Status:  "healthy",
		Message: "connected",
		Latency: "5ms",
	}

	// Kratos check (placeholder)
	checks["kratos"] = HealthStatus{
		Status:  "healthy",
		Message: "connected",
		Latency: "10ms",
	}

	// Determine overall status
	allHealthy := true
	for _, check := range checks {
		if check.Status != "healthy" {
			allHealthy = false
			break
		}
	}

	response := ReadinessResponse{
		Status:    getOverallStatus(allHealthy),
		Timestamp: time.Now(),
		Service:   "auth-service",
		Checks:    checks,
	}

	statusCode := http.StatusOK
	if !allHealthy {
		statusCode = http.StatusServiceUnavailable
	}

	return c.JSON(statusCode, response)
}

// LivenessCheck performs a liveness check
// @Summary Liveness check
// @Description Check if the service is alive
// @Tags health
// @Accept json
// @Produce json
// @Success 200 {object} HealthResponse
// @Router /v1/live [get]
func (h *HealthHandler) LivenessCheck(c echo.Context) error {
	response := HealthResponse{
		Status:    "alive",
		Timestamp: time.Now(),
		Service:   "auth-service",
		Version:   "1.0.0",
		Uptime:    time.Since(startTime).String(),
	}

	return c.JSON(http.StatusOK, response)
}

// Helper functions
func getOverallStatus(allHealthy bool) string {
	if allHealthy {
		return "ready"
	}
	return "not_ready"
}

// Response types
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Service   string    `json:"service"`
	Version   string    `json:"version"`
	Uptime    string    `json:"uptime"`
}

type ReadinessResponse struct {
	Status    string                  `json:"status"`
	Timestamp time.Time               `json:"timestamp"`
	Service   string                  `json:"service"`
	Checks    map[string]HealthStatus `json:"checks"`
}

type HealthStatus struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Latency string `json:"latency,omitempty"`
}

// startTime is set when the service starts
var startTime = time.Now()