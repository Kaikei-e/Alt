// PHASE R1: Deployment monitoring functionality
package monitoring

import (
	"context"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
	"deploy-cli/port"
)

// DeploymentMonitor handles real-time monitoring of deployments
type DeploymentMonitor struct {
	healthChecker     HealthCheckerPort
	metricsCollector  port.MetricsCollectorPort
	logger            logger_port.LoggerPort
}

// DeploymentMonitorPort defines the interface for deployment monitoring
type DeploymentMonitorPort interface {
	MonitorDeployment(ctx context.Context, deploymentID string, options *domain.MonitoringOptions) (*domain.MonitoringResult, error)
	StartContinuousMonitoring(ctx context.Context, namespace string, options *domain.MonitoringOptions) (<-chan *domain.MonitoringEvent, error)
	GetDeploymentHealth(ctx context.Context, deploymentID string) (*domain.HealthStatus, error)
	CollectMetrics(ctx context.Context, namespace string) (*domain.DeploymentMetrics, error)
}

// NewDeploymentMonitor creates a new deployment monitor
func NewDeploymentMonitor(
	healthChecker HealthCheckerPort,
	metricsCollector port.MetricsCollectorPort,
	logger logger_port.LoggerPort,
) *DeploymentMonitor {
	return &DeploymentMonitor{
		healthChecker:    healthChecker,
		metricsCollector: metricsCollector,
		logger:           logger,
	}
}

// MonitorDeployment monitors a specific deployment
func (m *DeploymentMonitor) MonitorDeployment(ctx context.Context, deploymentID string, options *domain.MonitoringOptions) (*domain.MonitoringResult, error) {
	m.logger.InfoWithContext("starting deployment monitoring", map[string]interface{}{
		"deployment_id": deploymentID,
		"timeout":       options.Timeout.String(),
	})

	result := &domain.MonitoringResult{
		DeploymentID: deploymentID,
		StartTime:    time.Now(),
		Status:       domain.MonitoringStatusActive,
		Events:       make([]*domain.MonitoringEvent, 0),
	}

	// Monitor until timeout or completion
	ticker := time.NewTicker(options.CheckInterval)
	defer ticker.Stop()

	timeoutTimer := time.NewTimer(options.Timeout)
	defer timeoutTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			result.Status = domain.MonitoringStatusCancelled
			result.EndTime = time.Now()
			return result, ctx.Err()

		case <-timeoutTimer.C:
			result.Status = domain.MonitoringStatusTimeout
			result.EndTime = time.Now()
			m.logger.WarnWithContext("deployment monitoring timed out", map[string]interface{}{
				"deployment_id": deploymentID,
				"duration":      result.EndTime.Sub(result.StartTime).String(),
			})
			return result, nil

		case <-ticker.C:
			// Check health status
			health, err := m.healthChecker.CheckDeploymentHealth(ctx, deploymentID)
			if err != nil {
				event := &domain.MonitoringEvent{
					Timestamp:    time.Now(),
					Level:        domain.EventLevelError,
					Message:      "Health check failed",
					Details:      "error: " + err.Error(),
					DeploymentID: deploymentID,
				}
				result.Events = append(result.Events, event)
				continue
			}

			// Create monitoring event
			event := &domain.MonitoringEvent{
				Timestamp:    time.Now(),
				Level:        domain.EventLevelInfo,
				Message:      "Health check completed",
				Details:      "status: " + health.Status,
				DeploymentID: deploymentID,
			}
			result.Events = append(result.Events, event)

			// Check if deployment is complete
			if health.Status == domain.HealthStatusHealthy {
				result.Status = domain.MonitoringStatusCompleted
				result.EndTime = time.Now()
				m.logger.InfoWithContext("deployment monitoring completed successfully", map[string]interface{}{
					"deployment_id": deploymentID,
					"duration":      result.EndTime.Sub(result.StartTime).String(),
					"events":        len(result.Events),
				})
				return result, nil
			}

			// Check for failure conditions
			if health.Status == domain.HealthStatusUnhealthy && options.EnableAlerts {
				result.Status = domain.MonitoringStatusFailed
				result.EndTime = time.Now()
				m.logger.ErrorWithContext("deployment monitoring failed due to unhealthy status", map[string]interface{}{
					"deployment_id": deploymentID,
					"duration":      result.EndTime.Sub(result.StartTime).String(),
				})
				return result, nil
			}
		}
	}
}

// StartContinuousMonitoring starts continuous monitoring and returns an event channel
func (m *DeploymentMonitor) StartContinuousMonitoring(ctx context.Context, namespace string, options *domain.MonitoringOptions) (<-chan *domain.MonitoringEvent, error) {
	m.logger.InfoWithContext("starting continuous monitoring", map[string]interface{}{
		"namespace":      namespace,
		"check_interval": options.CheckInterval.String(),
	})

	eventChan := make(chan *domain.MonitoringEvent, 100) // Buffer for events

	go func() {
		defer close(eventChan)
		ticker := time.NewTicker(options.CheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				m.logger.InfoWithContext("continuous monitoring stopped", map[string]interface{}{
					"namespace": namespace,
				})
				return

			case <-ticker.C:
				// Collect metrics
				_, err := m.metricsCollector.CollectMetrics(ctx, namespace)
				if err != nil {
					event := &domain.MonitoringEvent{
						Timestamp: time.Now(),
						Level:     domain.EventLevelError,
						Message:   "Metrics collection failed",
						Details:   "error: " + err.Error() + ", namespace: " + namespace,
					}
					select {
					case eventChan <- event:
					default:
						m.logger.WarnWithContext("monitoring event channel full", map[string]interface{}{
							"namespace": namespace,
						})
					}
					continue
				}

				// Create metrics event
				event := &domain.MonitoringEvent{
					Timestamp: time.Now(),
					Level:     domain.EventLevelInfo,
					Message:   "Metrics collected",
					Details:   "namespace: " + namespace + ", metrics collected successfully",
				}

				select {
				case eventChan <- event:
				default:
					m.logger.WarnWithContext("monitoring event channel full", map[string]interface{}{
						"namespace": namespace,
					})
				}
			}
		}
	}()

	return eventChan, nil
}

// GetDeploymentHealth gets the current health status of a deployment
func (m *DeploymentMonitor) GetDeploymentHealth(ctx context.Context, deploymentID string) (*domain.HealthStatus, error) {
	m.logger.DebugWithContext("getting deployment health", map[string]interface{}{
		"deployment_id": deploymentID,
	})

	health, err := m.healthChecker.CheckDeploymentHealth(ctx, deploymentID)
	if err != nil {
		m.logger.ErrorWithContext("deployment health check failed", map[string]interface{}{
			"deployment_id": deploymentID,
			"error":         err.Error(),
		})
		return nil, err
	}

	return health, nil
}

// CollectMetrics collects deployment metrics for a namespace
func (m *DeploymentMonitor) CollectMetrics(ctx context.Context, namespace string) (*domain.DeploymentMetrics, error) {
	m.logger.DebugWithContext("collecting deployment metrics", map[string]interface{}{
		"namespace": namespace,
	})

	metrics, err := m.metricsCollector.CollectMetrics(ctx, namespace)
	if err != nil {
		m.logger.ErrorWithContext("metrics collection failed", map[string]interface{}{
			"namespace": namespace,
			"error":     err.Error(),
		})
		return nil, err
	}

	return metrics, nil
}