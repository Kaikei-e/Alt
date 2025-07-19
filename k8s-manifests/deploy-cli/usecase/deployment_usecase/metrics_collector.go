package deployment_usecase

import (
	"fmt"
	"sync"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// MetricsCollector collects and manages deployment metrics
type MetricsCollector struct {
	logger            logger_port.LoggerPort
	deploymentMetrics map[string]*domain.DeploymentMetrics
	layerMetrics      map[string]map[string]*domain.LayerMetrics
	chartMetrics      map[string]map[string]map[string]*domain.ChartMetrics
	alerts            []domain.DeploymentAlert
	insights          []domain.DeploymentInsight
	mutex             sync.RWMutex
	alertThresholds   *AlertThresholds
}

// AlertThresholds defines thresholds for generating alerts
type AlertThresholds struct {
	MaxLayerDuration       time.Duration
	MaxChartDuration       time.Duration
	MaxHealthCheckDuration time.Duration
	MaxFailureRate         float64
	MaxRetryCount          int
	MinSuccessRate         float64
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(logger logger_port.LoggerPort) *MetricsCollector {
	return &MetricsCollector{
		logger:            logger,
		deploymentMetrics: make(map[string]*domain.DeploymentMetrics),
		layerMetrics:      make(map[string]map[string]*domain.LayerMetrics),
		chartMetrics:      make(map[string]map[string]map[string]*domain.ChartMetrics),
		alerts:            make([]domain.DeploymentAlert, 0),
		insights:          make([]domain.DeploymentInsight, 0),
		alertThresholds:   getDefaultAlertThresholds(),
	}
}

// getDefaultAlertThresholds returns default alert thresholds
func getDefaultAlertThresholds() *AlertThresholds {
	return &AlertThresholds{
		MaxLayerDuration:       30 * time.Minute,
		MaxChartDuration:       15 * time.Minute,
		MaxHealthCheckDuration: 5 * time.Minute,
		MaxFailureRate:         0.1, // 10%
		MaxRetryCount:          3,
		MinSuccessRate:         0.95, // 95%
	}
}

// StartDeploymentMetrics starts collecting metrics for a deployment
func (m *MetricsCollector) StartDeploymentMetrics(deploymentID string, options *domain.DeploymentOptions) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.deploymentMetrics[deploymentID] = &domain.DeploymentMetrics{
		DeploymentID: deploymentID,
		StartTime:    time.Now(),
		Status:       domain.DeploymentStatusSkipped, // Will be updated to success/failed
		Environment:  options.Environment,
		Strategy:     options.GetStrategyName(),
		LayerMetrics: make([]domain.LayerMetrics, 0),
		PerformanceMetrics: &domain.PerformanceMetrics{
			OptimizationSuggestions: make([]string, 0),
		},
		ErrorSummary: make([]domain.ErrorSummary, 0),
	}

	m.layerMetrics[deploymentID] = make(map[string]*domain.LayerMetrics)
	m.chartMetrics[deploymentID] = make(map[string]map[string]*domain.ChartMetrics)

	m.logger.InfoWithContext("started deployment metrics collection", map[string]interface{}{
		"deployment_id": deploymentID,
		"environment":   options.Environment.String(),
		"strategy":      options.GetStrategyName(),
	})

	return nil
}

// StartLayerMetrics starts collecting metrics for a layer
func (m *MetricsCollector) StartLayerMetrics(deploymentID, layerName string, totalCharts int) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.layerMetrics[deploymentID]; !exists {
		m.layerMetrics[deploymentID] = make(map[string]*domain.LayerMetrics)
	}

	m.layerMetrics[deploymentID][layerName] = &domain.LayerMetrics{
		LayerName:    layerName,
		StartTime:    time.Now(),
		Status:       domain.LayerStatusInProgress,
		TotalCharts:  totalCharts,
		ChartMetrics: make([]domain.ChartMetrics, 0),
		HealthCheckMetrics: &domain.HealthCheckMetrics{
			StartTime:          time.Now(),
			HealthCheckResults: make([]domain.HealthCheckResult, 0),
		},
		DependencyMetrics: &domain.DependencyMetrics{
			DependencyChain:      make([]string, 0),
			CircularDependencies: make([]string, 0),
			CriticalPath:         make([]string, 0),
		},
	}

	m.chartMetrics[deploymentID][layerName] = make(map[string]*domain.ChartMetrics)

	m.logger.InfoWithContext("started layer metrics collection", map[string]interface{}{
		"deployment_id": deploymentID,
		"layer_name":    layerName,
		"total_charts":  totalCharts,
	})

	return nil
}

// StartChartMetrics starts collecting metrics for a chart
func (m *MetricsCollector) StartChartMetrics(deploymentID, layerName, chartName, namespace string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.chartMetrics[deploymentID]; !exists {
		m.chartMetrics[deploymentID] = make(map[string]map[string]*domain.ChartMetrics)
	}
	if _, exists := m.chartMetrics[deploymentID][layerName]; !exists {
		m.chartMetrics[deploymentID][layerName] = make(map[string]*domain.ChartMetrics)
	}

	m.chartMetrics[deploymentID][layerName][chartName] = &domain.ChartMetrics{
		ChartName:         chartName,
		Namespace:         namespace,
		StartTime:         time.Now(),
		Status:            domain.DeploymentStatusSkipped, // Will be updated
		ResourceMetrics:   &domain.ResourceMetrics{},
		HealthCheckResult: &domain.HealthCheckResult{},
		ErrorDetails:      make([]domain.ErrorDetail, 0),
	}

	m.logger.InfoWithContext("started chart metrics collection", map[string]interface{}{
		"deployment_id": deploymentID,
		"layer_name":    layerName,
		"chart_name":    chartName,
		"namespace":     namespace,
	})

	return nil
}

// CompleteChartMetrics completes metrics collection for a chart
func (m *MetricsCollector) CompleteChartMetrics(deploymentID, layerName, chartName string, result domain.DeploymentResult) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if chartMetrics, exists := m.chartMetrics[deploymentID][layerName][chartName]; exists {
		endTime := time.Now()
		chartMetrics.EndTime = &endTime
		chartMetrics.Duration = endTime.Sub(chartMetrics.StartTime)
		chartMetrics.Status = result.Status

		if result.Error != nil {
			chartMetrics.ErrorDetails = append(chartMetrics.ErrorDetails, domain.ErrorDetail{
				Timestamp:   time.Now(),
				ErrorType:   "deployment_error",
				Message:     result.Error.Error(),
				Context:     map[string]interface{}{"chart": chartName, "layer": layerName},
				Severity:    domain.ErrorSeverityHigh,
				Recoverable: false,
			})
		}

		// Check for alerts
		m.checkChartAlerts(deploymentID, layerName, chartName, chartMetrics)

		m.logger.InfoWithContext("completed chart metrics collection", map[string]interface{}{
			"deployment_id": deploymentID,
			"layer_name":    layerName,
			"chart_name":    chartName,
			"duration":      chartMetrics.Duration,
			"status":        result.Status,
		})
	}

	return nil
}

// CompleteLayerMetrics completes metrics collection for a layer
func (m *MetricsCollector) CompleteLayerMetrics(deploymentID, layerName string, status domain.LayerStatus) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if layerMetrics, exists := m.layerMetrics[deploymentID][layerName]; exists {
		endTime := time.Now()
		layerMetrics.EndTime = &endTime
		layerMetrics.Duration = endTime.Sub(layerMetrics.StartTime)
		layerMetrics.Status = status

		// Calculate chart statistics
		for _, chartMetrics := range m.chartMetrics[deploymentID][layerName] {
			layerMetrics.ChartMetrics = append(layerMetrics.ChartMetrics, *chartMetrics)
			switch chartMetrics.Status {
			case domain.DeploymentStatusSuccess:
				layerMetrics.CompletedCharts++
			case domain.DeploymentStatusFailed:
				layerMetrics.FailedCharts++
			case domain.DeploymentStatusSkipped:
				layerMetrics.SkippedCharts++
			}
		}

		// Complete health check metrics
		if layerMetrics.HealthCheckMetrics != nil {
			layerMetrics.HealthCheckMetrics.EndTime = &endTime
			layerMetrics.HealthCheckMetrics.Duration = endTime.Sub(layerMetrics.HealthCheckMetrics.StartTime)
		}

		// Check for alerts
		m.checkLayerAlerts(deploymentID, layerName, layerMetrics)

		m.logger.InfoWithContext("completed layer metrics collection", map[string]interface{}{
			"deployment_id":    deploymentID,
			"layer_name":       layerName,
			"duration":         layerMetrics.Duration,
			"status":           status,
			"completed_charts": layerMetrics.CompletedCharts,
			"failed_charts":    layerMetrics.FailedCharts,
		})
	}

	return nil
}

// CompleteDeploymentMetrics completes metrics collection for a deployment
func (m *MetricsCollector) CompleteDeploymentMetrics(deploymentID string, result domain.DeploymentResult) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if deploymentMetrics, exists := m.deploymentMetrics[deploymentID]; exists {
		endTime := time.Now()
		deploymentMetrics.EndTime = &endTime
		deploymentMetrics.Duration = endTime.Sub(deploymentMetrics.StartTime)
		deploymentMetrics.Status = result.Status

		// Aggregate layer metrics
		for _, layerMetrics := range m.layerMetrics[deploymentID] {
			deploymentMetrics.LayerMetrics = append(deploymentMetrics.LayerMetrics, *layerMetrics)
			deploymentMetrics.CompletedCharts += layerMetrics.CompletedCharts
			deploymentMetrics.FailedCharts += layerMetrics.FailedCharts
			deploymentMetrics.SkippedCharts += layerMetrics.SkippedCharts
		}

		deploymentMetrics.TotalLayers = len(deploymentMetrics.LayerMetrics)
		deploymentMetrics.CompletedLayers = len(deploymentMetrics.LayerMetrics)
		deploymentMetrics.TotalCharts = deploymentMetrics.CompletedCharts + deploymentMetrics.FailedCharts + deploymentMetrics.SkippedCharts

		// Calculate performance metrics
		m.calculatePerformanceMetrics(deploymentMetrics)

		// Generate insights
		m.generateInsights(deploymentID, deploymentMetrics)

		m.logger.InfoWithContext("completed deployment metrics collection", map[string]interface{}{
			"deployment_id":    deploymentID,
			"duration":         deploymentMetrics.Duration,
			"status":           result.Status,
			"total_layers":     deploymentMetrics.TotalLayers,
			"total_charts":     deploymentMetrics.TotalCharts,
			"completed_charts": deploymentMetrics.CompletedCharts,
			"failed_charts":    deploymentMetrics.FailedCharts,
		})
	}

	return nil
}

// checkChartAlerts checks for chart-level alerts
func (m *MetricsCollector) checkChartAlerts(deploymentID, layerName, chartName string, chartMetrics *domain.ChartMetrics) {
	// Check for duration alert
	if chartMetrics.Duration > m.alertThresholds.MaxChartDuration {
		m.alerts = append(m.alerts, domain.DeploymentAlert{
			AlertID:   fmt.Sprintf("%s-chart-%s-duration", deploymentID, chartName),
			Timestamp: time.Now(),
			Severity:  domain.ErrorSeverityMedium,
			Type:      "chart_duration_exceeded",
			Message:   fmt.Sprintf("Chart %s in layer %s took longer than expected: %v", chartName, layerName, chartMetrics.Duration),
			Component: chartName,
			Context: map[string]interface{}{
				"layer":     layerName,
				"duration":  chartMetrics.Duration,
				"threshold": m.alertThresholds.MaxChartDuration,
			},
			Resolved: false,
		})
	}

	// Check for retry alert
	if chartMetrics.Retries > m.alertThresholds.MaxRetryCount {
		m.alerts = append(m.alerts, domain.DeploymentAlert{
			AlertID:   fmt.Sprintf("%s-chart-%s-retries", deploymentID, chartName),
			Timestamp: time.Now(),
			Severity:  domain.ErrorSeverityHigh,
			Type:      "chart_retry_exceeded",
			Message:   fmt.Sprintf("Chart %s in layer %s exceeded maximum retry count: %d", chartName, layerName, chartMetrics.Retries),
			Component: chartName,
			Context: map[string]interface{}{
				"layer":     layerName,
				"retries":   chartMetrics.Retries,
				"threshold": m.alertThresholds.MaxRetryCount,
			},
			Resolved: false,
		})
	}
}

// checkLayerAlerts checks for layer-level alerts
func (m *MetricsCollector) checkLayerAlerts(deploymentID, layerName string, layerMetrics *domain.LayerMetrics) {
	// Check for duration alert
	if layerMetrics.Duration > m.alertThresholds.MaxLayerDuration {
		m.alerts = append(m.alerts, domain.DeploymentAlert{
			AlertID:   fmt.Sprintf("%s-layer-%s-duration", deploymentID, layerName),
			Timestamp: time.Now(),
			Severity:  domain.ErrorSeverityMedium,
			Type:      "layer_duration_exceeded",
			Message:   fmt.Sprintf("Layer %s took longer than expected: %v", layerName, layerMetrics.Duration),
			Component: layerName,
			Context: map[string]interface{}{
				"duration":  layerMetrics.Duration,
				"threshold": m.alertThresholds.MaxLayerDuration,
			},
			Resolved: false,
		})
	}

	// Check for failure rate alert
	if layerMetrics.TotalCharts > 0 {
		failureRate := float64(layerMetrics.FailedCharts) / float64(layerMetrics.TotalCharts)
		if failureRate > m.alertThresholds.MaxFailureRate {
			m.alerts = append(m.alerts, domain.DeploymentAlert{
				AlertID:   fmt.Sprintf("%s-layer-%s-failure-rate", deploymentID, layerName),
				Timestamp: time.Now(),
				Severity:  domain.ErrorSeverityHigh,
				Type:      "layer_failure_rate_exceeded",
				Message:   fmt.Sprintf("Layer %s has high failure rate: %.2f%%", layerName, failureRate*100),
				Component: layerName,
				Context: map[string]interface{}{
					"failure_rate":  failureRate,
					"threshold":     m.alertThresholds.MaxFailureRate,
					"failed_charts": layerMetrics.FailedCharts,
					"total_charts":  layerMetrics.TotalCharts,
				},
				Resolved: false,
			})
		}
	}
}

// calculatePerformanceMetrics calculates performance metrics for a deployment
func (m *MetricsCollector) calculatePerformanceMetrics(deploymentMetrics *domain.DeploymentMetrics) {
	if deploymentMetrics.PerformanceMetrics == nil {
		deploymentMetrics.PerformanceMetrics = &domain.PerformanceMetrics{}
	}

	// Calculate average times
	if len(deploymentMetrics.LayerMetrics) > 0 {
		totalLayerTime := time.Duration(0)
		totalChartTime := time.Duration(0)
		totalHealthCheckTime := time.Duration(0)
		totalCharts := 0

		for _, layerMetrics := range deploymentMetrics.LayerMetrics {
			totalLayerTime += layerMetrics.Duration
			totalCharts += len(layerMetrics.ChartMetrics)

			for _, chartMetrics := range layerMetrics.ChartMetrics {
				totalChartTime += chartMetrics.Duration
			}

			if layerMetrics.HealthCheckMetrics != nil {
				totalHealthCheckTime += layerMetrics.HealthCheckMetrics.Duration
			}
		}

		deploymentMetrics.PerformanceMetrics.AverageLayerTime = totalLayerTime / time.Duration(len(deploymentMetrics.LayerMetrics))
		if totalCharts > 0 {
			deploymentMetrics.PerformanceMetrics.AverageChartTime = totalChartTime / time.Duration(totalCharts)
		}
		deploymentMetrics.PerformanceMetrics.TotalHealthCheckTime = totalHealthCheckTime
	}

	// Generate optimization suggestions
	m.generateOptimizationSuggestions(deploymentMetrics)
}

// generateOptimizationSuggestions generates optimization suggestions based on metrics
func (m *MetricsCollector) generateOptimizationSuggestions(deploymentMetrics *domain.DeploymentMetrics) {
	suggestions := make([]string, 0)

	// Check for slow layers
	for _, layerMetrics := range deploymentMetrics.LayerMetrics {
		if layerMetrics.Duration > 10*time.Minute {
			suggestions = append(suggestions, fmt.Sprintf("Consider optimizing %s layer - took %v", layerMetrics.LayerName, layerMetrics.Duration))
		}
	}

	// Check for failed charts
	if deploymentMetrics.FailedCharts > 0 {
		suggestions = append(suggestions, fmt.Sprintf("Address %d failed charts to improve deployment reliability", deploymentMetrics.FailedCharts))
	}

	// Check for deployment duration
	if deploymentMetrics.Duration > 45*time.Minute {
		suggestions = append(suggestions, "Consider using parallel deployment strategies to reduce overall deployment time")
	}

	deploymentMetrics.PerformanceMetrics.OptimizationSuggestions = suggestions
}

// generateInsights generates insights based on deployment metrics
func (m *MetricsCollector) generateInsights(deploymentID string, deploymentMetrics *domain.DeploymentMetrics) {
	insights := make([]domain.DeploymentInsight, 0)

	// Duration insight
	if deploymentMetrics.Duration > 30*time.Minute {
		insights = append(insights, domain.DeploymentInsight{
			InsightType: "performance",
			Title:       "Long Deployment Duration",
			Description: fmt.Sprintf("Deployment took %v, which is longer than recommended", deploymentMetrics.Duration),
			Metrics: map[string]interface{}{
				"duration":        deploymentMetrics.Duration,
				"recommended_max": 30 * time.Minute,
			},
			Suggestions: []domain.OptimizationSuggestion{
				{
					Type:        "performance",
					Component:   "deployment",
					Description: "Consider enabling parallel deployment for non-critical layers",
					Impact:      "Can reduce deployment time by 30-50%",
					Effort:      "medium",
					Priority:    "high",
					CreatedAt:   time.Now(),
				},
			},
			CreatedAt: time.Now(),
		})
	}

	// Reliability insight
	if deploymentMetrics.FailedCharts > 0 {
		insights = append(insights, domain.DeploymentInsight{
			InsightType: "reliability",
			Title:       "Deployment Failures Detected",
			Description: fmt.Sprintf("Deployment had %d failed charts", deploymentMetrics.FailedCharts),
			Metrics: map[string]interface{}{
				"failed_charts": deploymentMetrics.FailedCharts,
				"total_charts":  deploymentMetrics.TotalCharts,
				"failure_rate":  float64(deploymentMetrics.FailedCharts) / float64(deploymentMetrics.TotalCharts),
			},
			Suggestions: []domain.OptimizationSuggestion{
				{
					Type:        "reliability",
					Component:   "deployment",
					Description: "Review failed charts and improve their reliability",
					Impact:      "Improved deployment success rate",
					Effort:      "high",
					Priority:    "critical",
					CreatedAt:   time.Now(),
				},
			},
			CreatedAt: time.Now(),
		})
	}

	m.insights = append(m.insights, insights...)
}

// GetDeploymentMetrics returns metrics for a deployment
func (m *MetricsCollector) GetDeploymentMetrics(deploymentID string) (*domain.DeploymentMetrics, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if metrics, exists := m.deploymentMetrics[deploymentID]; exists {
		return metrics, nil
	}

	return nil, fmt.Errorf("deployment metrics not found for deployment ID: %s", deploymentID)
}

// GetActiveAlerts returns active alerts
func (m *MetricsCollector) GetActiveAlerts() []domain.DeploymentAlert {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	activeAlerts := make([]domain.DeploymentAlert, 0)
	for _, alert := range m.alerts {
		if !alert.Resolved {
			activeAlerts = append(activeAlerts, alert)
		}
	}

	return activeAlerts
}

// GetInsights returns generated insights
func (m *MetricsCollector) GetInsights() []domain.DeploymentInsight {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return m.insights
}

// RecordHealthCheck records a health check result
func (m *MetricsCollector) RecordHealthCheck(deploymentID, layerName string, result domain.HealthCheckResult) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if layerMetrics, exists := m.layerMetrics[deploymentID][layerName]; exists {
		if layerMetrics.HealthCheckMetrics != nil {
			layerMetrics.HealthCheckMetrics.HealthCheckResults = append(layerMetrics.HealthCheckMetrics.HealthCheckResults, result)
			layerMetrics.HealthCheckMetrics.TotalChecks++

			if result.Status == "success" {
				layerMetrics.HealthCheckMetrics.SuccessfulChecks++
			} else {
				layerMetrics.HealthCheckMetrics.FailedChecks++
			}

			// Calculate average check time
			if layerMetrics.HealthCheckMetrics.TotalChecks > 0 {
				totalTime := time.Duration(0)
				for _, checkResult := range layerMetrics.HealthCheckMetrics.HealthCheckResults {
					totalTime += checkResult.Duration
				}
				layerMetrics.HealthCheckMetrics.AverageCheckTime = totalTime / time.Duration(layerMetrics.HealthCheckMetrics.TotalChecks)
			}
		}
	}

	return nil
}
