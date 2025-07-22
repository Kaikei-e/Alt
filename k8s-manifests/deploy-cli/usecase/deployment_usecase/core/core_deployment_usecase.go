// PHASE R1: Core deployment functionality extracted from deployment_usecase.go
package core

import (
	"context"
	"fmt"

	"deploy-cli/domain"
	"deploy-cli/gateway/helm_gateway"
	"deploy-cli/port/logger_port"
)

// CoreDeploymentUsecase handles only the essential deployment operations
type CoreDeploymentUsecase struct {
	helmGateway    *helm_gateway.HelmGateway
	logger         logger_port.LoggerPort
	chartsDir      string
}

// CoreDeploymentUsecasePort defines the interface for core deployment operations
type CoreDeploymentUsecasePort interface {
	DeployChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error
	ValidateChart(ctx context.Context, chart domain.Chart) error
	GetChartStatus(ctx context.Context, chartName, namespace string) (*domain.ChartStatus, error)
	UndeployChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error
}

// NewCoreDeploymentUsecase creates a new core deployment usecase
func NewCoreDeploymentUsecase(
	helmGateway *helm_gateway.HelmGateway,
	logger logger_port.LoggerPort,
	chartsDir string,
) *CoreDeploymentUsecase {
	return &CoreDeploymentUsecase{
		helmGateway: helmGateway,
		logger:      logger,
		chartsDir:   chartsDir,
	}
}

// DeployChart deploys a single chart
func (c *CoreDeploymentUsecase) DeployChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	c.logger.InfoWithContext("deploying chart", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
		"type":      chart.Type,
	})

	// Validate chart before deployment
	if err := c.ValidateChart(ctx, chart); err != nil {
		c.logger.ErrorWithContext("chart validation failed", map[string]interface{}{
			"chart": chart.Name,
			"error": err.Error(),
		})
		return fmt.Errorf("chart validation failed for %s: %w", chart.Name, err)
	}

	// Deploy using helm gateway
	if err := c.helmGateway.DeployChart(ctx, chart, options); err != nil {
		c.logger.ErrorWithContext("chart deployment failed", map[string]interface{}{
			"chart": chart.Name,
			"error": err.Error(),
		})
		return fmt.Errorf("chart deployment failed for %s: %w", chart.Name, err)
	}

	c.logger.InfoWithContext("chart deployed successfully", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
	})

	return nil
}

// ValidateChart validates a chart before deployment
func (c *CoreDeploymentUsecase) ValidateChart(ctx context.Context, chart domain.Chart) error {
	c.logger.DebugWithContext("validating chart", map[string]interface{}{
		"chart": chart.Name,
		"path":  chart.Path,
	})

	// Use helm gateway for validation
	lintResult, err := c.helmGateway.LintChart(ctx, chart, &domain.DeploymentOptions{})
	if err != nil {
		return fmt.Errorf("chart lint failed: %w", err)
	}

	if !lintResult.Success {
		return fmt.Errorf("chart lint validation failed: %d errors found", len(lintResult.Errors))
	}

	c.logger.DebugWithContext("chart validation passed", map[string]interface{}{
		"chart":    chart.Name,
		"warnings": len(lintResult.Warnings),
	})

	return nil
}

// GetChartStatus gets the status of a deployed chart
func (c *CoreDeploymentUsecase) GetChartStatus(ctx context.Context, chartName, namespace string) (*domain.ChartStatus, error) {
	c.logger.DebugWithContext("getting chart status", map[string]interface{}{
		"chart":     chartName,
		"namespace": namespace,
	})

	// Get status from helm gateway
	status, err := c.helmGateway.GetReleaseStatus(ctx, chartName, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get chart status for %s: %w", chartName, err)
	}

	// Convert helm_port.HelmStatus to *domain.ChartStatus
	chartStatus := &domain.ChartStatus{
		Name:         status.Name,
		Namespace:    status.Namespace,
		Status:       status.Status,
		Revision:     status.Revision,
		LastDeployed: status.Updated,
		// Set default values for fields not available in HelmStatus
		Version:      "",
		AppVersion:   "",
		Description:  "",
		Notes:        "",
		Values:       make(map[string]interface{}),
		Resources:    []domain.ResourceInfo{},
		Dependencies: []string{},
		Hooks:        []domain.HookInfo{},
		TestStatus:   "",
	}

	return chartStatus, nil
}

// UndeployChart undeploys a chart
func (c *CoreDeploymentUsecase) UndeployChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	c.logger.InfoWithContext("undeploying chart", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
	})

	if err := c.helmGateway.UndeployChart(ctx, chart, options); err != nil {
		c.logger.ErrorWithContext("chart undeployment failed", map[string]interface{}{
			"chart": chart.Name,
			"error": err.Error(),
		})
		return fmt.Errorf("chart undeployment failed for %s: %w", chart.Name, err)
	}

	c.logger.InfoWithContext("chart undeployed successfully", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
	})

	return nil
}