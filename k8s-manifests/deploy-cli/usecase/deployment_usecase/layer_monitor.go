package deployment_usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// LayerHealthMonitor monitors the health of deployment layers in real-time
type LayerHealthMonitor struct {
	logger           logger_port.LoggerPort
	activeMonitors   map[string]*LayerMonitorState
	metricsCollector *MetricsCollector
	mutex            sync.RWMutex
}

// LayerMonitorState represents the state of a layer monitor
type LayerMonitorState struct {
	DeploymentID string
	LayerName    string
	StartTime    time.Time
	Status       domain.LayerStatus
	Charts       []string
	ActiveCharts map[string]*ChartMonitorState
	HealthChecks []domain.HealthCheckResult
	Dependencies []string
	CriticalPath []string
	Alerts       []domain.DeploymentAlert
	mutex        sync.RWMutex
}

// ChartMonitorState represents the state of a chart monitor
type ChartMonitorState struct {
	ChartName    string
	Namespace    string
	StartTime    time.Time
	Status       domain.DeploymentStatus
	Phase        string
	Retries      int
	LastUpdate   time.Time
	HealthChecks []domain.HealthCheckResult
	Dependencies []string
	Errors       []domain.ErrorDetail
}

// NewLayerHealthMonitor creates a new layer health monitor
func NewLayerHealthMonitor(logger logger_port.LoggerPort, metricsCollector *MetricsCollector) *LayerHealthMonitor {
	return &LayerHealthMonitor{
		logger:           logger,
		activeMonitors:   make(map[string]*LayerMonitorState),
		metricsCollector: metricsCollector,
	}
}

// StartLayerMonitoring starts monitoring a layer
func (m *LayerHealthMonitor) StartLayerMonitoring(ctx context.Context, deploymentID, layerName string, charts []domain.Chart) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	monitorKey := fmt.Sprintf("%s-%s", deploymentID, layerName)

	chartNames := make([]string, len(charts))
	for i, chart := range charts {
		chartNames[i] = chart.Name
	}

	monitorState := &LayerMonitorState{
		DeploymentID: deploymentID,
		LayerName:    layerName,
		StartTime:    time.Now(),
		Status:       domain.LayerStatusInProgress,
		Charts:       chartNames,
		ActiveCharts: make(map[string]*ChartMonitorState),
		HealthChecks: make([]domain.HealthCheckResult, 0),
		Dependencies: make([]string, 0),
		CriticalPath: make([]string, 0),
		Alerts:       make([]domain.DeploymentAlert, 0),
	}

	m.activeMonitors[monitorKey] = monitorState

	// Start monitoring goroutine
	go m.monitorLayerHealth(ctx, monitorKey, monitorState)

	m.logger.InfoWithContext("started layer health monitoring", map[string]interface{}{
		"deployment_id": deploymentID,
		"layer_name":    layerName,
		"charts_count":  len(charts),
	})

	return nil
}

// StartChartMonitoring starts monitoring a chart within a layer
func (m *LayerHealthMonitor) StartChartMonitoring(deploymentID, layerName, chartName, namespace string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	monitorKey := fmt.Sprintf("%s-%s", deploymentID, layerName)

	if layerMonitor, exists := m.activeMonitors[monitorKey]; exists {
		layerMonitor.mutex.Lock()
		defer layerMonitor.mutex.Unlock()

		layerMonitor.ActiveCharts[chartName] = &ChartMonitorState{
			ChartName:    chartName,
			Namespace:    namespace,
			StartTime:    time.Now(),
			Status:       domain.DeploymentStatusSkipped, // Will be updated
			Phase:        "initializing",
			Retries:      0,
			LastUpdate:   time.Now(),
			HealthChecks: make([]domain.HealthCheckResult, 0),
			Dependencies: make([]string, 0),
			Errors:       make([]domain.ErrorDetail, 0),
		}

		m.logger.InfoWithContext("started chart monitoring", map[string]interface{}{
			"deployment_id": deploymentID,
			"layer_name":    layerName,
			"chart_name":    chartName,
			"namespace":     namespace,
		})
	}

	return nil
}

// UpdateChartStatus updates the status of a chart
func (m *LayerHealthMonitor) UpdateChartStatus(deploymentID, layerName, chartName, phase string, status domain.DeploymentStatus) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	monitorKey := fmt.Sprintf("%s-%s", deploymentID, layerName)

	if layerMonitor, exists := m.activeMonitors[monitorKey]; exists {
		layerMonitor.mutex.Lock()
		defer layerMonitor.mutex.Unlock()

		if chartMonitor, exists := layerMonitor.ActiveCharts[chartName]; exists {
			chartMonitor.Status = status
			chartMonitor.Phase = phase
			chartMonitor.LastUpdate = time.Now()

			m.logger.InfoWithContext("updated chart status", map[string]interface{}{
				"deployment_id": deploymentID,
				"layer_name":    layerName,
				"chart_name":    chartName,
				"phase":         phase,
				"status":        status,
			})

			// Check for alerts
			m.checkChartStatusAlerts(deploymentID, layerName, chartName, chartMonitor)
		}
	}

	return nil
}

// RecordChartError records an error for a chart
func (m *LayerHealthMonitor) RecordChartError(deploymentID, layerName, chartName string, err error, severity domain.ErrorSeverity) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	monitorKey := fmt.Sprintf("%s-%s", deploymentID, layerName)

	if layerMonitor, exists := m.activeMonitors[monitorKey]; exists {
		layerMonitor.mutex.Lock()
		defer layerMonitor.mutex.Unlock()

		if chartMonitor, exists := layerMonitor.ActiveCharts[chartName]; exists {
			errorDetail := domain.ErrorDetail{
				Timestamp:   time.Now(),
				ErrorType:   "chart_error",
				Message:     err.Error(),
				Context:     map[string]interface{}{"chart": chartName, "layer": layerName},
				Severity:    severity,
				Recoverable: severity != domain.ErrorSeverityCritical,
			}

			chartMonitor.Errors = append(chartMonitor.Errors, errorDetail)

			// Generate alert for critical errors
			if severity == domain.ErrorSeverityCritical || severity == domain.ErrorSeverityHigh {
				alert := domain.DeploymentAlert{
					AlertID:   fmt.Sprintf("%s-chart-error-%s", deploymentID, chartName),
					Timestamp: time.Now(),
					Severity:  severity,
					Type:      "chart_error",
					Message:   fmt.Sprintf("Chart %s in layer %s encountered error: %s", chartName, layerName, err.Error()),
					Component: chartName,
					Context: map[string]interface{}{
						"layer":    layerName,
						"error":    err.Error(),
						"severity": severity,
					},
					Resolved: false,
				}

				layerMonitor.Alerts = append(layerMonitor.Alerts, alert)
			}

			m.logger.ErrorWithContext("recorded chart error", map[string]interface{}{
				"deployment_id": deploymentID,
				"layer_name":    layerName,
				"chart_name":    chartName,
				"error":         err.Error(),
				"severity":      severity,
			})
		}
	}

	return nil
}

// RecordHealthCheck records a health check result
func (m *LayerHealthMonitor) RecordHealthCheck(deploymentID, layerName string, result domain.HealthCheckResult) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	monitorKey := fmt.Sprintf("%s-%s", deploymentID, layerName)

	if layerMonitor, exists := m.activeMonitors[monitorKey]; exists {
		layerMonitor.mutex.Lock()
		defer layerMonitor.mutex.Unlock()

		layerMonitor.HealthChecks = append(layerMonitor.HealthChecks, result)

		// Also record with metrics collector
		if m.metricsCollector != nil {
			m.metricsCollector.RecordHealthCheck(deploymentID, layerName, result)
		}

		// Check for health check alerts
		m.checkHealthCheckAlerts(deploymentID, layerName, result)

		m.logger.InfoWithContext("recorded health check", map[string]interface{}{
			"deployment_id": deploymentID,
			"layer_name":    layerName,
			"check_type":    result.CheckType,
			"status":        result.Status,
			"duration":      result.Duration,
		})
	}

	return nil
}

// CompleteLayerMonitoring completes monitoring for a layer
func (m *LayerHealthMonitor) CompleteLayerMonitoring(deploymentID, layerName string, status domain.LayerStatus) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	monitorKey := fmt.Sprintf("%s-%s", deploymentID, layerName)

	if layerMonitor, exists := m.activeMonitors[monitorKey]; exists {
		layerMonitor.mutex.Lock()
		layerMonitor.Status = status
		layerMonitor.mutex.Unlock()

		m.logger.InfoWithContext("completed layer monitoring", map[string]interface{}{
			"deployment_id": deploymentID,
			"layer_name":    layerName,
			"status":        status,
			"duration":      time.Since(layerMonitor.StartTime),
			"alerts_count":  len(layerMonitor.Alerts),
		})

		// Generate layer completion insight
		m.generateLayerCompletionInsight(deploymentID, layerName, layerMonitor)

		// Clean up completed monitoring
		delete(m.activeMonitors, monitorKey)
	}

	return nil
}

// monitorLayerHealth monitors the health of a layer in real-time
func (m *LayerHealthMonitor) monitorLayerHealth(ctx context.Context, monitorKey string, layerMonitor *LayerMonitorState) {
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.InfoWithContext("layer health monitoring stopped due to context cancellation", map[string]interface{}{
				"deployment_id": layerMonitor.DeploymentID,
				"layer_name":    layerMonitor.LayerName,
			})
			return
		case <-ticker.C:
			m.performLayerHealthCheck(layerMonitor)
		}
	}
}

// performLayerHealthCheck performs periodic health checks for a layer
func (m *LayerHealthMonitor) performLayerHealthCheck(layerMonitor *LayerMonitorState) {
	layerMonitor.mutex.RLock()
	defer layerMonitor.mutex.RUnlock()

	// Check if layer is still active
	if layerMonitor.Status == domain.LayerStatusCompleted || layerMonitor.Status == domain.LayerStatusFailed {
		return
	}

	// Check for stuck charts
	for chartName, chartMonitor := range layerMonitor.ActiveCharts {
		if time.Since(chartMonitor.LastUpdate) > 5*time.Minute {
			m.logger.WarnWithContext("chart appears to be stuck", map[string]interface{}{
				"deployment_id": layerMonitor.DeploymentID,
				"layer_name":    layerMonitor.LayerName,
				"chart_name":    chartName,
				"last_update":   chartMonitor.LastUpdate,
				"phase":         chartMonitor.Phase,
			})

			// Generate stuck chart alert
			alert := domain.DeploymentAlert{
				AlertID:   fmt.Sprintf("%s-chart-stuck-%s", layerMonitor.DeploymentID, chartName),
				Timestamp: time.Now(),
				Severity:  domain.ErrorSeverityHigh,
				Type:      "chart_stuck",
				Message:   fmt.Sprintf("Chart %s in layer %s appears to be stuck in phase %s", chartName, layerMonitor.LayerName, chartMonitor.Phase),
				Component: chartName,
				Context: map[string]interface{}{
					"layer":       layerMonitor.LayerName,
					"phase":       chartMonitor.Phase,
					"last_update": chartMonitor.LastUpdate,
				},
				Resolved: false,
			}

			layerMonitor.Alerts = append(layerMonitor.Alerts, alert)
		}
	}

	// Check layer duration
	if time.Since(layerMonitor.StartTime) > 30*time.Minute {
		m.logger.WarnWithContext("layer taking longer than expected", map[string]interface{}{
			"deployment_id": layerMonitor.DeploymentID,
			"layer_name":    layerMonitor.LayerName,
			"duration":      time.Since(layerMonitor.StartTime),
		})
	}
}

// checkChartStatusAlerts checks for chart status-related alerts
func (m *LayerHealthMonitor) checkChartStatusAlerts(deploymentID, layerName, chartName string, chartMonitor *ChartMonitorState) {
	// Check for long-running phases
	if time.Since(chartMonitor.LastUpdate) > 10*time.Minute {
		m.logger.WarnWithContext("chart phase taking longer than expected", map[string]interface{}{
			"deployment_id": deploymentID,
			"layer_name":    layerName,
			"chart_name":    chartName,
			"phase":         chartMonitor.Phase,
			"duration":      time.Since(chartMonitor.LastUpdate),
		})
	}

	// Check for excessive retries
	if chartMonitor.Retries > 3 {
		m.logger.WarnWithContext("chart has excessive retries", map[string]interface{}{
			"deployment_id": deploymentID,
			"layer_name":    layerName,
			"chart_name":    chartName,
			"retries":       chartMonitor.Retries,
		})
	}
}

// checkHealthCheckAlerts checks for health check-related alerts
func (m *LayerHealthMonitor) checkHealthCheckAlerts(deploymentID, layerName string, result domain.HealthCheckResult) {
	// Check for failed health checks
	if result.Status != "success" {
		m.logger.WarnWithContext("health check failed", map[string]interface{}{
			"deployment_id": deploymentID,
			"layer_name":    layerName,
			"check_type":    result.CheckType,
			"target":        result.Target,
			"status":        result.Status,
			"message":       result.Message,
		})
	}

	// Check for slow health checks
	if result.Duration > 30*time.Second {
		m.logger.WarnWithContext("health check taking longer than expected", map[string]interface{}{
			"deployment_id": deploymentID,
			"layer_name":    layerName,
			"check_type":    result.CheckType,
			"target":        result.Target,
			"duration":      result.Duration,
		})
	}
}

// generateLayerCompletionInsight generates insights when a layer completes
func (m *LayerHealthMonitor) generateLayerCompletionInsight(deploymentID, layerName string, layerMonitor *LayerMonitorState) {
	duration := time.Since(layerMonitor.StartTime)

	// Generate performance insight
	if duration > 15*time.Minute {
		m.logger.InfoWithContext("generating layer performance insight", map[string]interface{}{
			"deployment_id": deploymentID,
			"layer_name":    layerName,
			"duration":      duration,
			"status":        layerMonitor.Status,
		})
	}

	// Generate reliability insight
	errorCount := 0
	for _, chartMonitor := range layerMonitor.ActiveCharts {
		errorCount += len(chartMonitor.Errors)
	}

	if errorCount > 0 {
		m.logger.InfoWithContext("generating layer reliability insight", map[string]interface{}{
			"deployment_id": deploymentID,
			"layer_name":    layerName,
			"error_count":   errorCount,
			"status":        layerMonitor.Status,
		})
	}
}

// GetLayerStatus returns the current status of a layer
func (m *LayerHealthMonitor) GetLayerStatus(deploymentID, layerName string) (*LayerMonitorState, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	monitorKey := fmt.Sprintf("%s-%s", deploymentID, layerName)

	if layerMonitor, exists := m.activeMonitors[monitorKey]; exists {
		layerMonitor.mutex.RLock()
		defer layerMonitor.mutex.RUnlock()

		// Create a copy to avoid race conditions
		return &LayerMonitorState{
			DeploymentID: layerMonitor.DeploymentID,
			LayerName:    layerMonitor.LayerName,
			StartTime:    layerMonitor.StartTime,
			Status:       layerMonitor.Status,
			Charts:       layerMonitor.Charts,
			Alerts:       layerMonitor.Alerts,
		}, nil
	}

	return nil, fmt.Errorf("layer monitor not found for deployment %s, layer %s", deploymentID, layerName)
}

// GetActiveMonitors returns all active monitors
func (m *LayerHealthMonitor) GetActiveMonitors() map[string]*LayerMonitorState {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Create a copy to avoid race conditions
	result := make(map[string]*LayerMonitorState)
	for key, monitor := range m.activeMonitors {
		result[key] = monitor
	}

	return result
}

// GetLayerAlerts returns alerts for a specific layer
func (m *LayerHealthMonitor) GetLayerAlerts(deploymentID, layerName string) []domain.DeploymentAlert {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	monitorKey := fmt.Sprintf("%s-%s", deploymentID, layerName)

	if layerMonitor, exists := m.activeMonitors[monitorKey]; exists {
		layerMonitor.mutex.RLock()
		defer layerMonitor.mutex.RUnlock()

		return layerMonitor.Alerts
	}

	return []domain.DeploymentAlert{}
}
