// PHASE R1: Chart deployment execution logic
package core

import (
	"context"
	"fmt"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// ChartDeploymentExecutor handles the execution of individual chart deployments
type ChartDeploymentExecutor struct {
	coreDeployment CoreDeploymentUsecasePort
	logger         logger_port.LoggerPort
}

// ChartDeploymentExecutorPort defines the interface for chart execution
type ChartDeploymentExecutorPort interface {
	ExecuteChartDeployment(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) (*domain.ChartDeploymentResult, error)
	ExecuteChartsInSequence(ctx context.Context, charts []domain.Chart, options *domain.DeploymentOptions) ([]*domain.ChartDeploymentResult, error)
	ExecuteChartsInParallel(ctx context.Context, charts []domain.Chart, options *domain.DeploymentOptions, maxConcurrency int) ([]*domain.ChartDeploymentResult, error)
}

// NewChartDeploymentExecutor creates a new chart deployment executor
func NewChartDeploymentExecutor(
	coreDeployment CoreDeploymentUsecasePort,
	logger logger_port.LoggerPort,
) *ChartDeploymentExecutor {
	return &ChartDeploymentExecutor{
		coreDeployment: coreDeployment,
		logger:         logger,
	}
}

// ExecuteChartDeployment executes a single chart deployment with metrics
func (e *ChartDeploymentExecutor) ExecuteChartDeployment(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) (*domain.ChartDeploymentResult, error) {
	startTime := time.Now()
	
	result := &domain.ChartDeploymentResult{
		ChartName: chart.Name,
		Namespace: options.GetNamespace(chart.Name),
		StartTime: startTime,
		Status:    domain.DeploymentStatusInProgress,
	}

	e.logger.InfoWithContext("executing chart deployment", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": result.Namespace,
		"type":      chart.Type,
	})

	// Execute the deployment
	err := e.coreDeployment.DeployChart(ctx, chart, options)
	
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	if err != nil {
		result.Status = domain.DeploymentStatusFailed
		result.Error = err
		
		e.logger.ErrorWithContext("chart deployment failed", map[string]interface{}{
			"chart":    chart.Name,
			"duration": result.Duration.String(),
			"error":    err.Error(),
		})
		
		return result, fmt.Errorf("chart deployment failed: %w", err)
	}

	result.Status = domain.DeploymentStatusSuccess
	
	e.logger.InfoWithContext("chart deployment completed successfully", map[string]interface{}{
		"chart":    chart.Name,
		"duration": result.Duration.String(),
		"status":   result.Status,
	})

	return result, nil
}

// ExecuteChartsInSequence executes charts sequentially
func (e *ChartDeploymentExecutor) ExecuteChartsInSequence(ctx context.Context, charts []domain.Chart, options *domain.DeploymentOptions) ([]*domain.ChartDeploymentResult, error) {
	e.logger.InfoWithContext("executing charts in sequence", map[string]interface{}{
		"chart_count": len(charts),
	})

	results := make([]*domain.ChartDeploymentResult, 0, len(charts))
	
	for i, chart := range charts {
		e.logger.InfoWithContext("executing sequential chart", map[string]interface{}{
			"chart":    chart.Name,
			"position": i + 1,
			"total":    len(charts),
		})

		result, err := e.ExecuteChartDeployment(ctx, chart, options)
		results = append(results, result)

		if err != nil {
			e.logger.ErrorWithContext("sequential deployment failed, stopping", map[string]interface{}{
				"failed_chart": chart.Name,
				"position":     i + 1,
				"completed":    i,
			})
			return results, fmt.Errorf("sequential deployment failed at chart %s (position %d): %w", chart.Name, i+1, err)
		}

		// Add delay between charts if specified
		if options.WaitBetweenCharts > 0 && i < len(charts)-1 {
			e.logger.DebugWithContext("waiting between charts", map[string]interface{}{
				"delay":      options.WaitBetweenCharts.String(),
				"next_chart": charts[i+1].Name,
			})
			
			select {
			case <-ctx.Done():
				return results, ctx.Err()
			case <-time.After(options.WaitBetweenCharts):
				// Continue to next chart
			}
		}
	}

	e.logger.InfoWithContext("sequential chart deployment completed", map[string]interface{}{
		"chart_count":    len(charts),
		"success_count":  len(results),
		"total_duration": e.calculateTotalDuration(results).String(),
	})

	return results, nil
}

// ExecuteChartsInParallel executes charts in parallel with concurrency limit
func (e *ChartDeploymentExecutor) ExecuteChartsInParallel(ctx context.Context, charts []domain.Chart, options *domain.DeploymentOptions, maxConcurrency int) ([]*domain.ChartDeploymentResult, error) {
	e.logger.InfoWithContext("executing charts in parallel", map[string]interface{}{
		"chart_count":     len(charts),
		"max_concurrency": maxConcurrency,
	})

	// Create a semaphore to limit concurrency
	semaphore := make(chan struct{}, maxConcurrency)
	results := make([]*domain.ChartDeploymentResult, len(charts))
	errors := make([]error, len(charts))

	// Channel to collect completion signals
	done := make(chan int, len(charts))

	// Start all deployments
	for i, chart := range charts {
		go func(index int, chart domain.Chart) {
			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			e.logger.DebugWithContext("starting parallel chart deployment", map[string]interface{}{
				"chart": chart.Name,
				"index": index,
			})

			result, err := e.ExecuteChartDeployment(ctx, chart, options)
			results[index] = result
			errors[index] = err

			done <- index
		}(i, chart)
	}

	// Wait for all deployments to complete
	for i := 0; i < len(charts); i++ {
		select {
		case completedIndex := <-done:
			e.logger.DebugWithContext("parallel chart deployment completed", map[string]interface{}{
				"chart":     charts[completedIndex].Name,
				"index":     completedIndex,
				"remaining": len(charts) - i - 1,
			})
		case <-ctx.Done():
			return results, ctx.Err()
		}
	}

	// Check for errors
	var firstError error
	successCount := 0
	for i, err := range errors {
		if err != nil {
			if firstError == nil {
				firstError = err
			}
			e.logger.ErrorWithContext("parallel chart deployment failed", map[string]interface{}{
				"chart": charts[i].Name,
				"error": err.Error(),
			})
		} else {
			successCount++
		}
	}

	e.logger.InfoWithContext("parallel chart deployment completed", map[string]interface{}{
		"chart_count":   len(charts),
		"success_count": successCount,
		"failed_count":  len(charts) - successCount,
		"total_duration": e.calculateTotalDuration(results).String(),
	})

	if firstError != nil {
		return results, fmt.Errorf("parallel deployment had failures: %w", firstError)
	}

	return results, nil
}

// calculateTotalDuration calculates the total duration across all results
func (e *ChartDeploymentExecutor) calculateTotalDuration(results []*domain.ChartDeploymentResult) time.Duration {
	var total time.Duration
	for _, result := range results {
		if result != nil {
			total += result.Duration
		}
	}
	return total
}