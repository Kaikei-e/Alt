// ABOUTME: Token rotation monitoring and management service
// ABOUTME: Provides proactive monitoring, health checks, and rotation statistics

package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"pre-processor-sidecar/repository"
)

// TokenRotationManager manages token rotation monitoring and health checks
type TokenRotationManager struct {
	tokenRepo       repository.OAuth2TokenRepository
	tokenService    *TokenManagementService
	logger          *slog.Logger
	
	// Configuration
	monitorInterval   time.Duration
	healthCheckPeriod time.Duration
	alertThreshold    time.Duration // Alert if token expires within this time
	
	// State
	mu                sync.RWMutex
	lastRotationTime  time.Time
	rotationCount     int64
	healthStatus      RotationHealthStatus
	
	// Control
	stopCh    chan struct{}
	isRunning bool
}

// RotationHealthStatus represents the health status of token rotation
type RotationHealthStatus struct {
	LastCheck          time.Time     `json:"last_check"`
	TokenExists        bool          `json:"token_exists"`
	TokenValid         bool          `json:"token_valid"`
	TimeToExpiry       time.Duration `json:"time_to_expiry"`
	NeedsAttention     bool          `json:"needs_attention"`
	LastRotationTime   time.Time     `json:"last_rotation_time,omitempty"`
	RotationCount      int64         `json:"rotation_count"`
	ErrorMessage       string        `json:"error_message,omitempty"`
}

// NewTokenRotationManager creates a new token rotation manager
func NewTokenRotationManager(
	tokenRepo repository.OAuth2TokenRepository,
	tokenService *TokenManagementService,
	logger *slog.Logger,
) *TokenRotationManager {
	if logger == nil {
		logger = slog.Default()
	}

	return &TokenRotationManager{
		tokenRepo:         tokenRepo,
		tokenService:      tokenService,
		logger:            logger,
		monitorInterval:   10 * time.Minute,  // Check every 10 minutes
		healthCheckPeriod: 30 * time.Minute,  // Health check every 30 minutes (reduced from 5 min)
		alertThreshold:    30 * time.Minute,  // Alert if expires within 30 minutes
		stopCh:            make(chan struct{}),
		healthStatus: RotationHealthStatus{
			LastCheck: time.Now(),
		},
	}
}

// StartMonitoring starts the token rotation monitoring goroutines
func (m *TokenRotationManager) StartMonitoring(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRunning {
		return fmt.Errorf("token rotation monitoring is already running")
	}

	m.isRunning = true
	m.logger.Info("Starting token rotation monitoring",
		"monitor_interval", m.monitorInterval,
		"health_check_period", m.healthCheckPeriod,
		"alert_threshold", m.alertThreshold)

	// Start monitoring goroutines
	go m.monitorLoop(ctx)
	go m.healthCheckLoop(ctx)

	return nil
}

// StopMonitoring stops the token rotation monitoring
func (m *TokenRotationManager) StopMonitoring() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning {
		return
	}

	m.logger.Info("Stopping token rotation monitoring")
	close(m.stopCh)
	m.isRunning = false
}

// monitorLoop runs the main monitoring loop
func (m *TokenRotationManager) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(m.monitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Monitor loop stopped due to context cancellation")
			return
		case <-m.stopCh:
			m.logger.Info("Monitor loop stopped")
			return
		case <-ticker.C:
			m.performMonitoringCheck(ctx)
		}
	}
}

// healthCheckLoop runs the health check loop
func (m *TokenRotationManager) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(m.healthCheckPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Health check loop stopped due to context cancellation")
			return
		case <-m.stopCh:
			m.logger.Info("Health check loop stopped")
			return
		case <-ticker.C:
			m.performHealthCheck(ctx)
		}
	}
}

// performMonitoringCheck performs a monitoring check
func (m *TokenRotationManager) performMonitoringCheck(ctx context.Context) {
	m.logger.Debug("Performing token rotation monitoring check")

	// Check if proactive refresh is needed
	err := m.tokenService.RefreshTokenProactively(ctx)
	if err != nil {
		m.logger.Error("Proactive token refresh failed", "error", err)
		m.updateHealthStatus(false, false, 0, true, err.Error())
		return
	}

	// Update health status
	token, err := m.tokenRepo.GetCurrentToken(ctx)
	if err != nil {
		m.logger.Error("Failed to get token for monitoring", "error", err)
		m.updateHealthStatus(false, false, 0, true, err.Error())
		return
	}

	timeToExpiry := token.TimeUntilExpiry()
	needsAttention := timeToExpiry <= m.alertThreshold

	if needsAttention {
		m.logger.Warn("Token requires attention",
			"time_to_expiry", timeToExpiry,
			"alert_threshold", m.alertThreshold)
	}

	m.updateHealthStatus(true, token.IsValid(), timeToExpiry, needsAttention, "")
}

// performHealthCheck performs a health check
func (m *TokenRotationManager) performHealthCheck(ctx context.Context) {
	m.logger.Debug("Performing token health check")

	err := m.tokenService.ValidateAndRecoverToken(ctx)
	if err != nil {
		m.logger.Error("Token validation and recovery failed", "error", err)
		m.updateHealthStatus(false, false, 0, true, err.Error())
		return
	}

	m.logger.Debug("Token health check passed")
}

// updateHealthStatus updates the internal health status
func (m *TokenRotationManager) updateHealthStatus(
	exists, valid bool, 
	timeToExpiry time.Duration, 
	needsAttention bool, 
	errorMsg string,
) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.healthStatus = RotationHealthStatus{
		LastCheck:        time.Now(),
		TokenExists:      exists,
		TokenValid:       valid,
		TimeToExpiry:     timeToExpiry,
		NeedsAttention:   needsAttention,
		LastRotationTime: m.lastRotationTime,
		RotationCount:    m.rotationCount,
		ErrorMessage:     errorMsg,
	}
}

// GetHealthStatus returns the current health status
func (m *TokenRotationManager) GetHealthStatus() RotationHealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.healthStatus
}

// RecordRotation records a token rotation event
func (m *TokenRotationManager) RecordRotation() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastRotationTime = time.Now()
	m.rotationCount++

	m.logger.Info("Token rotation recorded",
		"rotation_count", m.rotationCount,
		"last_rotation", m.lastRotationTime)
}

// GetRotationStatistics returns rotation statistics
func (m *TokenRotationManager) GetRotationStatistics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"total_rotations":     m.rotationCount,
		"last_rotation_time":  m.lastRotationTime,
		"monitoring_uptime":   time.Since(m.healthStatus.LastCheck),
		"is_monitoring":       m.isRunning,
		"monitor_interval":    m.monitorInterval.String(),
		"health_check_period": m.healthCheckPeriod.String(),
		"alert_threshold":     m.alertThreshold.String(),
	}
}

// ForceHealthCheck forces an immediate health check
func (m *TokenRotationManager) ForceHealthCheck(ctx context.Context) error {
	m.logger.Info("Forcing immediate health check")
	m.performHealthCheck(ctx)
	return nil
}

// ForceMonitoringCheck forces an immediate monitoring check
func (m *TokenRotationManager) ForceMonitoringCheck(ctx context.Context) error {
	m.logger.Info("Forcing immediate monitoring check")
	m.performMonitoringCheck(ctx)
	return nil
}