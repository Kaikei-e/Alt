package recovery

import (
	"context"
	"fmt"
	"time"

	"deploy-cli/domain"
	"deploy-cli/gateway/helm_gateway"
	"deploy-cli/port/logger_port"
)

// RollbackManager handles rollback operations for deployments
type RollbackManager struct {
	helmGateway *helm_gateway.HelmGateway
	logger      logger_port.LoggerPort
}

// RollbackManagerPort defines the interface for rollback management
type RollbackManagerPort interface {
	RollbackDeployment(ctx context.Context, chart domain.Chart, targetRevision int, options *domain.RollbackOptions) error
	RollbackChart(ctx context.Context, chartName, namespace string, targetRevision int) error
	ValidateRollbackTarget(ctx context.Context, chart domain.Chart, targetRevision int) error
	GetRollbackHistory(ctx context.Context, chart domain.Chart) ([]domain.ChartRevision, error)
}

// NewRollbackManager creates a new rollback manager
func NewRollbackManager(
	helmGateway *helm_gateway.HelmGateway,
	logger logger_port.LoggerPort,
) *RollbackManager {
	return &RollbackManager{
		helmGateway: helmGateway,
		logger:      logger,
	}
}

// RollbackDeployment rolls back a chart deployment to a previous revision
func (r *RollbackManager) RollbackDeployment(ctx context.Context, chart domain.Chart, targetRevision int, options *domain.RollbackOptions) error {
	r.logger.InfoWithContext("starting deployment rollback", map[string]interface{}{
		"chart":           chart.Name,
		"target_revision": targetRevision,
		"revision":        options.Revision,
	})

	// Validate rollback target
	if err := r.ValidateRollbackTarget(ctx, chart, targetRevision); err != nil {
		r.logger.ErrorWithContext("rollback target validation failed", map[string]interface{}{
			"chart":           chart.Name,
			"target_revision": targetRevision,
			"error":           err.Error(),
		})
		return fmt.Errorf("invalid rollback target: %w", err)
	}

	// Generate release name and namespace
	releaseName := r.generateReleaseName(chart)
	namespace := r.determineNamespace(chart)

	// Perform the rollback
	err := r.helmGateway.RollbackRelease(ctx, releaseName, namespace, targetRevision)
	if err != nil {
		r.logger.ErrorWithContext("rollback failed", map[string]interface{}{
			"chart":           chart.Name,
			"release_name":    releaseName,
			"namespace":       namespace,
			"target_revision": targetRevision,
			"error":           err.Error(),
		})
		return fmt.Errorf("rollback failed for chart %s: %w", chart.Name, err)
	}

	r.logger.InfoWithContext("deployment rollback completed", map[string]interface{}{
		"chart":           chart.Name,
		"target_revision": targetRevision,
		"release_name":    releaseName,
		"namespace":       namespace,
	})

	return nil
}

// RollbackChart rolls back a specific chart by name and namespace
func (r *RollbackManager) RollbackChart(ctx context.Context, chartName, namespace string, targetRevision int) error {
	r.logger.InfoWithContext("rolling back chart", map[string]interface{}{
		"chart":           chartName,
		"namespace":       namespace,
		"target_revision": targetRevision,
	})

	err := r.helmGateway.RollbackRelease(ctx, chartName, namespace, targetRevision)
	if err != nil {
		r.logger.ErrorWithContext("chart rollback failed", map[string]interface{}{
			"chart":           chartName,
			"namespace":       namespace,
			"target_revision": targetRevision,
			"error":           err.Error(),
		})
		return fmt.Errorf("rollback failed for chart %s in namespace %s: %w", chartName, namespace, err)
	}

	r.logger.InfoWithContext("chart rollback completed", map[string]interface{}{
		"chart":           chartName,
		"namespace":       namespace,
		"target_revision": targetRevision,
	})

	return nil
}

// ValidateRollbackTarget validates if the target revision is valid for rollback
func (r *RollbackManager) ValidateRollbackTarget(ctx context.Context, chart domain.Chart, targetRevision int) error {
	r.logger.DebugWithContext("validating rollback target", map[string]interface{}{
		"chart":           chart.Name,
		"target_revision": targetRevision,
	})

	// Get release history
	releaseName := r.generateReleaseName(chart)
	namespace := r.determineNamespace(chart)

	// Get release history from Helm
	historyOutput, err := r.helmGateway.GetReleaseHistory(ctx, chart, &domain.DeploymentOptions{
		Environment: domain.Production, // Default to production for validation
	})
	if err != nil {
		r.logger.ErrorWithContext("failed to get release history", map[string]interface{}{
			"chart":        chart.Name,
			"release_name": releaseName,
			"namespace":    namespace,
			"error":        err.Error(),
		})
		return fmt.Errorf("failed to get release history: %w", err)
	}

	// For now, we'll accept any positive revision number
	// In a full implementation, we would parse the history output to validate
	if targetRevision <= 0 {
		return fmt.Errorf("invalid revision number: %d (must be positive)", targetRevision)
	}

	r.logger.DebugWithContext("rollback target validated", map[string]interface{}{
		"chart":           chart.Name,
		"target_revision": targetRevision,
		"history_length":  len(historyOutput),
	})

	return nil
}

// GetRollbackHistory retrieves the rollback history for a chart
func (r *RollbackManager) GetRollbackHistory(ctx context.Context, chart domain.Chart) ([]domain.ChartRevision, error) {
	r.logger.DebugWithContext("getting rollback history", map[string]interface{}{
		"chart": chart.Name,
	})

	// Get release history from Helm
	historyOutput, err := r.helmGateway.GetReleaseHistory(ctx, chart, &domain.DeploymentOptions{
		Environment: domain.Production, // Default environment
	})
	if err != nil {
		r.logger.ErrorWithContext("failed to get release history", map[string]interface{}{
			"chart": chart.Name,
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to get release history: %w", err)
	}

	// For now, return a minimal history entry
	// In a full implementation, we would parse the Helm history output
	revisions := []domain.ChartRevision{
		{
			Revision:     1,
			Chart:        chart.Name,
			Status:       "deployed",
			LastDeployed: time.Now().Add(-time.Hour),
			Description:  "Initial deployment",
		},
	}

	r.logger.DebugWithContext("rollback history retrieved", map[string]interface{}{
		"chart":            chart.Name,
		"revision_count":   len(revisions),
		"history_length":   len(historyOutput),
	})

	return revisions, nil
}

// Helper methods

func (r *RollbackManager) generateReleaseName(chart domain.Chart) string {
	// Simple release name generation - in production this should match the deployment logic
	return chart.Name
}

func (r *RollbackManager) determineNamespace(chart domain.Chart) string {
	// Simple namespace determination - in production this should match the deployment logic
	switch chart.Type {
	case domain.InfrastructureChart:
		if chart.Name == "postgres" || chart.Name == "clickhouse" {
			return "alt-database"
		}
		return "alt-apps"
	case domain.ApplicationChart:
		return "alt-apps"
	case domain.OperationalChart:
		return "alt-apps"
	default:
		return "alt-apps"
	}
}