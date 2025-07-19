package deployment_usecase

import (
	"fmt"
	"sync"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// ProgressTracker tracks deployment progress in real-time
type ProgressTracker struct {
	logger           logger_port.LoggerPort
	deploymentID     string
	progress         *domain.DeploymentProgress
	layerProgress    map[string]*domain.LayerProgress
	chartProgress    map[string]*domain.ChartProgress
	startTime        time.Time
	estimatedEndTime *time.Time
	mutex            sync.RWMutex
	progressCallback func(*domain.DeploymentProgress)
	metricsCollector *MetricsCollector
}

// NewProgressTracker creates a new progress tracker
func NewProgressTracker(logger logger_port.LoggerPort, deploymentID string, totalCharts int, metricsCollector *MetricsCollector) *ProgressTracker {
	return &ProgressTracker{
		logger:           logger,
		deploymentID:     deploymentID,
		progress:         domain.NewDeploymentProgress(totalCharts),
		layerProgress:    make(map[string]*domain.LayerProgress),
		chartProgress:    make(map[string]*domain.ChartProgress),
		startTime:        time.Now(),
		metricsCollector: metricsCollector,
	}
}

// SetProgressCallback sets a callback function for progress updates
func (p *ProgressTracker) SetProgressCallback(callback func(*domain.DeploymentProgress)) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.progressCallback = callback
}

// StartLayerProgress starts tracking progress for a layer
func (p *ProgressTracker) StartLayerProgress(layerName string, totalCharts int) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	layerProgress := &domain.LayerProgress{
		LayerName:              layerName,
		Status:                 domain.LayerStatusInProgress,
		StartTime:              time.Now(),
		TotalCharts:            totalCharts,
		CompletedCharts:        0,
		ProgressPercent:        0.0,
		EstimatedTimeRemaining: 0,
		Message:                "Starting layer deployment...",
	}

	p.layerProgress[layerName] = layerProgress
	p.progress.CurrentPhase = fmt.Sprintf("Deploying layer: %s", layerName)

	p.logger.InfoWithContext("started layer progress tracking", map[string]interface{}{
		"deployment_id": p.deploymentID,
		"layer_name":    layerName,
		"total_charts":  totalCharts,
	})

	p.notifyProgressUpdate()
	return nil
}

// StartChartProgress starts tracking progress for a chart
func (p *ProgressTracker) StartChartProgress(layerName, chartName string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	chartKey := fmt.Sprintf("%s/%s", layerName, chartName)
	chartProgress := &domain.ChartProgress{
		ChartName:       chartName,
		Status:          domain.DeploymentStatusSkipped, // Will be updated
		StartTime:       time.Now(),
		CurrentPhase:    "initializing",
		ProgressPercent: 0.0,
		Message:         "Starting chart deployment...",
		Retries:         0,
	}

	p.chartProgress[chartKey] = chartProgress
	p.progress.CurrentChart = chartName

	// Update layer progress
	if layerProgress, exists := p.layerProgress[layerName]; exists {
		layerProgress.CurrentChart = chartName
		layerProgress.Message = fmt.Sprintf("Deploying chart: %s", chartName)
	}

	p.logger.InfoWithContext("started chart progress tracking", map[string]interface{}{
		"deployment_id": p.deploymentID,
		"layer_name":    layerName,
		"chart_name":    chartName,
	})

	p.notifyProgressUpdate()
	return nil
}

// UpdateChartProgress updates the progress of a chart
func (p *ProgressTracker) UpdateChartProgress(layerName, chartName, phase string, progressPercent float64, message string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	chartKey := fmt.Sprintf("%s/%s", layerName, chartName)
	if chartProgress, exists := p.chartProgress[chartKey]; exists {
		chartProgress.CurrentPhase = phase
		chartProgress.ProgressPercent = progressPercent
		chartProgress.Message = message

		p.logger.InfoWithContext("updated chart progress", map[string]interface{}{
			"deployment_id":    p.deploymentID,
			"layer_name":       layerName,
			"chart_name":       chartName,
			"phase":            phase,
			"progress_percent": progressPercent,
			"message":          message,
		})

		p.notifyProgressUpdate()
	}

	return nil
}

// CompleteChartProgress completes progress tracking for a chart
func (p *ProgressTracker) CompleteChartProgress(layerName, chartName string, result domain.DeploymentResult) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	chartKey := fmt.Sprintf("%s/%s", layerName, chartName)
	if chartProgress, exists := p.chartProgress[chartKey]; exists {
		chartProgress.Status = result.Status
		chartProgress.ProgressPercent = 100.0
		chartProgress.Message = result.Message

		// Update overall progress
		p.progress.AddResult(result)

		// Update layer progress
		if layerProgress, exists := p.layerProgress[layerName]; exists {
			layerProgress.CompletedCharts++
			layerProgress.ProgressPercent = float64(layerProgress.CompletedCharts) / float64(layerProgress.TotalCharts) * 100.0
			layerProgress.EstimatedTimeRemaining = p.calculateEstimatedTimeRemaining(layerProgress)

			if layerProgress.CompletedCharts == layerProgress.TotalCharts {
				layerProgress.Status = domain.LayerStatusCompleted
				layerProgress.Message = "Layer deployment completed"
			}
		}

		p.logger.InfoWithContext("completed chart progress tracking", map[string]interface{}{
			"deployment_id": p.deploymentID,
			"layer_name":    layerName,
			"chart_name":    chartName,
			"status":        result.Status,
			"duration":      result.Duration,
		})

		p.notifyProgressUpdate()
	}

	return nil
}

// CompleteLayerProgress completes progress tracking for a layer
func (p *ProgressTracker) CompleteLayerProgress(layerName string, status domain.LayerStatus) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if layerProgress, exists := p.layerProgress[layerName]; exists {
		layerProgress.Status = status
		layerProgress.ProgressPercent = 100.0
		layerProgress.Message = fmt.Sprintf("Layer %s completed with status: %s", layerName, status)

		p.logger.InfoWithContext("completed layer progress tracking", map[string]interface{}{
			"deployment_id": p.deploymentID,
			"layer_name":    layerName,
			"status":        status,
			"duration":      time.Since(layerProgress.StartTime),
		})

		p.notifyProgressUpdate()
	}

	return nil
}

// CompleteDeploymentProgress completes overall deployment progress tracking
func (p *ProgressTracker) CompleteDeploymentProgress(status domain.DeploymentStatus) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.progress.CurrentPhase = "Deployment completed"
	p.progress.CurrentChart = ""

	endTime := time.Now()
	p.estimatedEndTime = &endTime

	p.logger.InfoWithContext("completed deployment progress tracking", map[string]interface{}{
		"deployment_id":    p.deploymentID,
		"status":           status,
		"duration":         endTime.Sub(p.startTime),
		"total_charts":     p.progress.TotalCharts,
		"completed_charts": p.progress.CompletedCharts,
		"success_count":    p.progress.GetSuccessCount(),
		"failed_count":     p.progress.GetFailedCount(),
	})

	p.notifyProgressUpdate()
	return nil
}

// RecordChartRetry records a chart retry
func (p *ProgressTracker) RecordChartRetry(layerName, chartName string, retryCount int, reason string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	chartKey := fmt.Sprintf("%s/%s", layerName, chartName)
	if chartProgress, exists := p.chartProgress[chartKey]; exists {
		chartProgress.Retries = retryCount
		chartProgress.Message = fmt.Sprintf("Retry %d: %s", retryCount, reason)

		p.logger.InfoWithContext("recorded chart retry", map[string]interface{}{
			"deployment_id": p.deploymentID,
			"layer_name":    layerName,
			"chart_name":    chartName,
			"retry_count":   retryCount,
			"reason":        reason,
		})

		p.notifyProgressUpdate()
	}

	return nil
}

// calculateEstimatedTimeRemaining calculates the estimated time remaining for a layer
func (p *ProgressTracker) calculateEstimatedTimeRemaining(layerProgress *domain.LayerProgress) time.Duration {
	if layerProgress.CompletedCharts == 0 {
		return 0
	}

	elapsed := time.Since(layerProgress.StartTime)
	averageTimePerChart := elapsed / time.Duration(layerProgress.CompletedCharts)
	remainingCharts := layerProgress.TotalCharts - layerProgress.CompletedCharts

	return averageTimePerChart * time.Duration(remainingCharts)
}

// notifyProgressUpdate notifies about progress updates
func (p *ProgressTracker) notifyProgressUpdate() {
	if p.progressCallback != nil {
		// Create a copy to avoid race conditions
		progressCopy := p.createProgressCopy()
		go p.progressCallback(progressCopy)
	}
}

// createProgressCopy creates a copy of the current progress
func (p *ProgressTracker) createProgressCopy() *domain.DeploymentProgress {
	return &domain.DeploymentProgress{
		TotalCharts:     p.progress.TotalCharts,
		CompletedCharts: p.progress.CompletedCharts,
		CurrentChart:    p.progress.CurrentChart,
		CurrentPhase:    p.progress.CurrentPhase,
		Results:         append([]domain.DeploymentResult(nil), p.progress.Results...),
	}
}

// GetCurrentProgress returns the current deployment progress
func (p *ProgressTracker) GetCurrentProgress() *domain.DeploymentProgress {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.createProgressCopy()
}

// GetLayerProgress returns the progress of a specific layer
func (p *ProgressTracker) GetLayerProgress(layerName string) (*domain.LayerProgress, error) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	if layerProgress, exists := p.layerProgress[layerName]; exists {
		// Create a copy to avoid race conditions
		return &domain.LayerProgress{
			LayerName:              layerProgress.LayerName,
			Status:                 layerProgress.Status,
			StartTime:              layerProgress.StartTime,
			CurrentChart:           layerProgress.CurrentChart,
			CompletedCharts:        layerProgress.CompletedCharts,
			TotalCharts:            layerProgress.TotalCharts,
			ProgressPercent:        layerProgress.ProgressPercent,
			EstimatedTimeRemaining: layerProgress.EstimatedTimeRemaining,
			Message:                layerProgress.Message,
		}, nil
	}

	return nil, fmt.Errorf("layer progress not found: %s", layerName)
}

// GetChartProgress returns the progress of a specific chart
func (p *ProgressTracker) GetChartProgress(layerName, chartName string) (*domain.ChartProgress, error) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	chartKey := fmt.Sprintf("%s/%s", layerName, chartName)
	if chartProgress, exists := p.chartProgress[chartKey]; exists {
		// Create a copy to avoid race conditions
		return &domain.ChartProgress{
			ChartName:       chartProgress.ChartName,
			Status:          chartProgress.Status,
			StartTime:       chartProgress.StartTime,
			CurrentPhase:    chartProgress.CurrentPhase,
			ProgressPercent: chartProgress.ProgressPercent,
			Message:         chartProgress.Message,
			Retries:         chartProgress.Retries,
		}, nil
	}

	return nil, fmt.Errorf("chart progress not found: %s/%s", layerName, chartName)
}

// GetAllLayerProgress returns progress for all layers
func (p *ProgressTracker) GetAllLayerProgress() map[string]*domain.LayerProgress {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	result := make(map[string]*domain.LayerProgress)
	for layerName, layerProgress := range p.layerProgress {
		result[layerName] = &domain.LayerProgress{
			LayerName:              layerProgress.LayerName,
			Status:                 layerProgress.Status,
			StartTime:              layerProgress.StartTime,
			CurrentChart:           layerProgress.CurrentChart,
			CompletedCharts:        layerProgress.CompletedCharts,
			TotalCharts:            layerProgress.TotalCharts,
			ProgressPercent:        layerProgress.ProgressPercent,
			EstimatedTimeRemaining: layerProgress.EstimatedTimeRemaining,
			Message:                layerProgress.Message,
		}
	}

	return result
}

// GetAllChartProgress returns progress for all charts
func (p *ProgressTracker) GetAllChartProgress() map[string]*domain.ChartProgress {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	result := make(map[string]*domain.ChartProgress)
	for chartKey, chartProgress := range p.chartProgress {
		result[chartKey] = &domain.ChartProgress{
			ChartName:       chartProgress.ChartName,
			Status:          chartProgress.Status,
			StartTime:       chartProgress.StartTime,
			CurrentPhase:    chartProgress.CurrentPhase,
			ProgressPercent: chartProgress.ProgressPercent,
			Message:         chartProgress.Message,
			Retries:         chartProgress.Retries,
		}
	}

	return result
}

// GetProgressSummary returns a summary of the deployment progress
func (p *ProgressTracker) GetProgressSummary() map[string]interface{} {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	totalDuration := time.Since(p.startTime)
	overallProgress := float64(p.progress.CompletedCharts) / float64(p.progress.TotalCharts) * 100.0

	// Calculate estimated time remaining
	var estimatedTimeRemaining time.Duration
	if p.progress.CompletedCharts > 0 {
		averageTimePerChart := totalDuration / time.Duration(p.progress.CompletedCharts)
		remainingCharts := p.progress.TotalCharts - p.progress.CompletedCharts
		estimatedTimeRemaining = averageTimePerChart * time.Duration(remainingCharts)
	}

	// Count layers by status
	layerStatusCount := make(map[string]int)
	for _, layerProgress := range p.layerProgress {
		layerStatusCount[layerProgress.Status.String()]++
	}

	return map[string]interface{}{
		"deployment_id":            p.deploymentID,
		"start_time":               p.startTime,
		"total_duration":           totalDuration,
		"overall_progress_percent": overallProgress,
		"estimated_time_remaining": estimatedTimeRemaining,
		"total_charts":             p.progress.TotalCharts,
		"completed_charts":         p.progress.CompletedCharts,
		"success_count":            p.progress.GetSuccessCount(),
		"failed_count":             p.progress.GetFailedCount(),
		"skipped_count":            p.progress.GetSkippedCount(),
		"current_phase":            p.progress.CurrentPhase,
		"current_chart":            p.progress.CurrentChart,
		"total_layers":             len(p.layerProgress),
		"layer_status_count":       layerStatusCount,
	}
}

// GetProgressInsights returns insights about the deployment progress
func (p *ProgressTracker) GetProgressInsights() []domain.DeploymentInsight {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	insights := make([]domain.DeploymentInsight, 0)

	// Duration insight
	totalDuration := time.Since(p.startTime)
	expectedDuration := 30 * time.Minute // Expected deployment duration

	if totalDuration > expectedDuration {
		insights = append(insights, domain.DeploymentInsight{
			InsightType: "performance",
			Title:       "Deployment Duration Exceeded",
			Description: fmt.Sprintf("Deployment is taking longer than expected: %v vs expected %v", totalDuration, expectedDuration),
			Metrics: map[string]interface{}{
				"actual_duration":   totalDuration,
				"expected_duration": expectedDuration,
				"delay":             totalDuration - expectedDuration,
			},
			Suggestions: []domain.OptimizationSuggestion{
				{
					Type:        "performance",
					Component:   "deployment",
					Description: "Consider optimizing slow deployment layers",
					Impact:      "Reduced deployment time",
					Effort:      "medium",
					Priority:    "medium",
					CreatedAt:   time.Now(),
				},
			},
			CreatedAt: time.Now(),
		})
	}

	// Failure rate insight
	if p.progress.GetFailedCount() > 0 {
		failureRate := float64(p.progress.GetFailedCount()) / float64(p.progress.TotalCharts) * 100.0
		insights = append(insights, domain.DeploymentInsight{
			InsightType: "reliability",
			Title:       "Deployment Failures Detected",
			Description: fmt.Sprintf("Deployment has %.1f%% failure rate", failureRate),
			Metrics: map[string]interface{}{
				"failed_charts": p.progress.GetFailedCount(),
				"total_charts":  p.progress.TotalCharts,
				"failure_rate":  failureRate,
			},
			Suggestions: []domain.OptimizationSuggestion{
				{
					Type:        "reliability",
					Component:   "deployment",
					Description: "Review and fix failed charts to improve reliability",
					Impact:      "Improved deployment success rate",
					Effort:      "high",
					Priority:    "high",
					CreatedAt:   time.Now(),
				},
			},
			CreatedAt: time.Now(),
		})
	}

	return insights
}
