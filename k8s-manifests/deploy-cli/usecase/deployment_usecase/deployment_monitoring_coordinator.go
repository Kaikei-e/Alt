package deployment_usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// DeploymentMonitoringCoordinator handles deployment monitoring and progress tracking
type DeploymentMonitoringCoordinator struct {
	metricsCollector   *MetricsCollector
	dependencyDetector *DependencyFailureDetector
	progressTracker    *ProgressTracker
	logger             logger_port.LoggerPort
	activeDeployments  map[string]*domain.DeploymentStatusInfo
	deploymentsMutex   sync.RWMutex
}

// NewDeploymentMonitoringCoordinator creates a new deployment monitoring coordinator
func NewDeploymentMonitoringCoordinator(
	metricsCollector *MetricsCollector,
	dependencyDetector *DependencyFailureDetector,
	logger logger_port.LoggerPort,
) *DeploymentMonitoringCoordinator {
	return &DeploymentMonitoringCoordinator{
		metricsCollector:   metricsCollector,
		dependencyDetector: dependencyDetector,
		logger:             logger,
		activeDeployments:  make(map[string]*domain.DeploymentStatusInfo),
		deploymentsMutex:   sync.RWMutex{},
	}
}

// InitializeMonitoring initializes monitoring for a deployment
func (m *DeploymentMonitoringCoordinator) InitializeMonitoring(ctx context.Context, deploymentID string, options *domain.DeploymentOptions) error {
	m.logger.InfoWithContext("initializing deployment monitoring", map[string]interface{}{
		"deployment_id": deploymentID,
		"environment":   options.Environment.String(),
	})

	m.deploymentsMutex.Lock()
	defer m.deploymentsMutex.Unlock()

	// Create deployment status
	status := &domain.DeploymentStatusInfo{
		ID:          deploymentID,
		Environment: options.Environment,
		Status:      domain.DeploymentStatusInProgress,
		StartTime:   time.Now(),
		Phase:       "Initializing",
	}

	m.activeDeployments[deploymentID] = status

	// Initialize metrics collection
	if err := m.metricsCollector.StartDeploymentMetrics(deploymentID, options); err != nil {
		return fmt.Errorf("failed to start deployment metrics: %w", err)
	}

	// Initialize dependency monitoring
	if err := m.dependencyDetector.StartDependencyMonitoring(ctx, deploymentID); err != nil {
		return fmt.Errorf("failed to start dependency monitoring: %w", err)
	}

	// Initialize progress tracking
	chartConfig := domain.NewChartConfig(options.ChartsDir)
	allCharts := chartConfig.AllCharts()
	m.progressTracker = NewProgressTracker(m.logger, deploymentID, len(allCharts), m.metricsCollector)

	// Set up progress callback for real-time updates
	m.progressTracker.SetProgressCallback(func(progress *domain.DeploymentProgress) {
		m.updateDeploymentProgress(deploymentID, progress)
	})

	m.logger.InfoWithContext("monitoring initialized", map[string]interface{}{
		"deployment_id": deploymentID,
		"total_charts":  len(allCharts),
	})

	return nil
}

// updateDeploymentProgress updates the deployment progress
func (m *DeploymentMonitoringCoordinator) updateDeploymentProgress(deploymentID string, progress *domain.DeploymentProgress) {
	m.deploymentsMutex.Lock()
	defer m.deploymentsMutex.Unlock()

	if status, exists := m.activeDeployments[deploymentID]; exists {
		status.CurrentPhase = progress.CurrentPhase
		status.CurrentChart = progress.CurrentChart
		status.CompletedCharts = progress.CompletedCharts
		status.TotalCharts = progress.TotalCharts
		status.SuccessfulCharts = progress.GetSuccessCount()
		status.FailedCharts = progress.GetFailedCount()
		status.SkippedCharts = progress.GetSkippedCount()

		if progress.TotalCharts > 0 {
			status.ProgressPercent = float64(progress.CompletedCharts) / float64(progress.TotalCharts) * 100.0
		}

		m.logger.InfoWithContext("deployment progress update", map[string]interface{}{
			"deployment_id":    deploymentID,
			"current_phase":    progress.CurrentPhase,
			"current_chart":    progress.CurrentChart,
			"completed_charts": progress.CompletedCharts,
			"total_charts":     progress.TotalCharts,
			"progress_percent": status.ProgressPercent,
		})
	}
}

// GetDeploymentStatus returns the current status of a deployment
func (m *DeploymentMonitoringCoordinator) GetDeploymentStatus(ctx context.Context, deploymentID string) (*domain.DeploymentStatusInfo, error) {
	m.deploymentsMutex.RLock()
	defer m.deploymentsMutex.RUnlock()

	status, exists := m.activeDeployments[deploymentID]
	if !exists {
		return nil, fmt.Errorf("deployment not found: %s", deploymentID)
	}

	// Create a copy to avoid race conditions
	statusCopy := *status
	return &statusCopy, nil
}

// ListActiveDeployments returns a list of currently active deployments
func (m *DeploymentMonitoringCoordinator) ListActiveDeployments(ctx context.Context) ([]*domain.DeploymentStatusInfo, error) {
	m.deploymentsMutex.RLock()
	defer m.deploymentsMutex.RUnlock()

	var activeDeployments []*domain.DeploymentStatusInfo
	for _, status := range m.activeDeployments {
		if status.Status == domain.DeploymentStatusInProgress {
			statusCopy := *status
			activeDeployments = append(activeDeployments, &statusCopy)
		}
	}

	return activeDeployments, nil
}

// CancelDeployment cancels an ongoing deployment
func (m *DeploymentMonitoringCoordinator) CancelDeployment(ctx context.Context, deploymentID string) error {
	m.deploymentsMutex.Lock()
	defer m.deploymentsMutex.Unlock()

	status, exists := m.activeDeployments[deploymentID]
	if !exists {
		return fmt.Errorf("deployment not found: %s", deploymentID)
	}

	if status.Status != domain.DeploymentStatusInProgress {
		return fmt.Errorf("deployment is not in progress: %s", deploymentID)
	}

	status.Status = domain.DeploymentStatusCancelled
	status.EndTime = time.Now()
	status.Phase = "Cancelled"

	m.logger.InfoWithContext("deployment cancelled", map[string]interface{}{
		"deployment_id": deploymentID,
	})

	return nil
}

// MarkDeploymentComplete marks a deployment as completed
func (m *DeploymentMonitoringCoordinator) MarkDeploymentComplete(deploymentID string, success bool, err error) {
	m.deploymentsMutex.Lock()
	defer m.deploymentsMutex.Unlock()

	status, exists := m.activeDeployments[deploymentID]
	if !exists {
		m.logger.WarnWithContext("attempted to mark unknown deployment as complete", map[string]interface{}{
			"deployment_id": deploymentID,
		})
		return
	}

	status.EndTime = time.Now()
	status.Duration = status.EndTime.Sub(status.StartTime)

	if success {
		status.Status = domain.DeploymentStatusCompleted
		status.Phase = "Completed Successfully"
	} else {
		status.Status = domain.DeploymentStatusFailed
		status.Phase = "Failed"
		if err != nil {
			status.Error = err.Error()
		}
	}

	m.logger.InfoWithContext("deployment marked as complete", map[string]interface{}{
		"deployment_id": deploymentID,
		"success":       success,
		"duration":      status.Duration,
		"error":         status.Error,
	})
}

// CleanupCompletedDeployments removes completed deployments from tracking
func (m *DeploymentMonitoringCoordinator) CleanupCompletedDeployments(olderThan time.Duration) {
	m.deploymentsMutex.Lock()
	defer m.deploymentsMutex.Unlock()

	cutoffTime := time.Now().Add(-olderThan)
	var cleanedDeployments []string

	for deploymentID, status := range m.activeDeployments {
		if status.Status != domain.DeploymentStatusInProgress &&
			!status.EndTime.IsZero() &&
			status.EndTime.Before(cutoffTime) {
			delete(m.activeDeployments, deploymentID)
			cleanedDeployments = append(cleanedDeployments, deploymentID)
		}
	}

	if len(cleanedDeployments) > 0 {
		m.logger.InfoWithContext("cleaned up completed deployments", map[string]interface{}{
			"cleaned_deployments": cleanedDeployments,
			"cleanup_count":       len(cleanedDeployments),
		})
	}
}

// GetDeploymentMetrics returns metrics for a specific deployment
func (m *DeploymentMonitoringCoordinator) GetDeploymentMetrics(deploymentID string) (*domain.DeploymentMetrics, error) {
	if m.metricsCollector == nil {
		return nil, fmt.Errorf("metrics collector not initialized")
	}

	return m.metricsCollector.GetDeploymentMetrics(deploymentID)
}

// GetSystemMetrics returns overall system deployment metrics
func (m *DeploymentMonitoringCoordinator) GetSystemMetrics() (map[string]interface{}, error) {
	m.deploymentsMutex.RLock()
	defer m.deploymentsMutex.RUnlock()

	var inProgress, completed, failed, cancelled int
	for _, status := range m.activeDeployments {
		switch status.Status {
		case domain.DeploymentStatusInProgress:
			inProgress++
		case domain.DeploymentStatusCompleted:
			completed++
		case domain.DeploymentStatusFailed:
			failed++
		case domain.DeploymentStatusCancelled:
			cancelled++
		}
	}

	metrics := map[string]interface{}{
		"active_deployments": len(m.activeDeployments),
		"in_progress":        inProgress,
		"completed":          completed,
		"failed":             failed,
		"cancelled":          cancelled,
	}

	// Add system metrics if available
	// Note: GetSystemMetrics method would need to be implemented in MetricsCollector

	return metrics, nil
}

// StartPeriodicCleanup starts periodic cleanup of old deployment records
func (m *DeploymentMonitoringCoordinator) StartPeriodicCleanup(ctx context.Context, interval, maxAge time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.InfoWithContext("stopping periodic cleanup", map[string]interface{}{
				"reason": "context cancelled",
			})
			return
		case <-ticker.C:
			m.CleanupCompletedDeployments(maxAge)
		}
	}
}

// MonitorDeploymentHealth monitors the health of an active deployment
func (m *DeploymentMonitoringCoordinator) MonitorDeploymentHealth(ctx context.Context, deploymentID string, healthCheckInterval time.Duration) {
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			status, err := m.GetDeploymentStatus(ctx, deploymentID)
			if err != nil {
				m.logger.WarnWithContext("failed to get deployment status for health check", map[string]interface{}{
					"deployment_id": deploymentID,
					"error":         err.Error(),
				})
				return
			}

			if status.Status != domain.DeploymentStatusInProgress {
				m.logger.InfoWithContext("deployment no longer in progress, stopping health monitoring", map[string]interface{}{
					"deployment_id": deploymentID,
					"status":        status.Status,
				})
				return
			}

			// Check if deployment has been running too long
			if time.Since(status.StartTime) > 2*time.Hour {
				m.logger.WarnWithContext("deployment running for unusually long time", map[string]interface{}{
					"deployment_id": deploymentID,
					"duration":      time.Since(status.StartTime),
					"current_phase": status.CurrentPhase,
					"current_chart": status.CurrentChart,
				})
			}

			// Check if deployment appears stuck
			if status.CurrentPhase != "" && time.Since(status.LastUpdated) > 30*time.Minute {
				m.logger.WarnWithContext("deployment appears to be stuck", map[string]interface{}{
					"deployment_id": deploymentID,
					"current_phase": status.CurrentPhase,
					"current_chart": status.CurrentChart,
					"last_updated":  status.LastUpdated,
				})
			}
		}
	}
}

// GenerateDeploymentReport generates a comprehensive report for a deployment
func (m *DeploymentMonitoringCoordinator) GenerateDeploymentReport(deploymentID string) (*domain.DeploymentReport, error) {
	status, err := m.GetDeploymentStatus(context.Background(), deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment status: %w", err)
	}

	var deploymentMetrics *domain.DeploymentMetrics
	if m.metricsCollector != nil {
		var err error
		deploymentMetrics, err = m.metricsCollector.GetDeploymentMetrics(deploymentID)
		if err != nil {
			// Log error but continue with report generation
			m.logger.WarnWithContext("failed to get deployment metrics", map[string]interface{}{
				"deployment_id": deploymentID,
				"error":         err.Error(),
			})
		}
	}

	// Convert metrics to map[string]interface{} for report
	var metricsMap map[string]interface{}
	if deploymentMetrics != nil {
		// Convert struct to map - this would require reflection or manual mapping
		metricsMap = map[string]interface{}{
			"deployment_id":    deploymentMetrics.DeploymentID,
			"start_time":       deploymentMetrics.StartTime,
			"end_time":         deploymentMetrics.EndTime,
			"duration":         deploymentMetrics.Duration,
			"status":           deploymentMetrics.Status,
			"environment":      deploymentMetrics.Environment,
			"strategy":         deploymentMetrics.Strategy,
			"total_charts":     deploymentMetrics.TotalCharts,
			"completed_charts": deploymentMetrics.CompletedCharts,
			"failed_charts":    deploymentMetrics.FailedCharts,
			"skipped_charts":   deploymentMetrics.SkippedCharts,
		}
	}

	report := &domain.DeploymentReport{
		DeploymentID: deploymentID,
		Status:       status,
		Metrics:      metricsMap,
		GeneratedAt:  time.Now(),
	}

	return report, nil
}
