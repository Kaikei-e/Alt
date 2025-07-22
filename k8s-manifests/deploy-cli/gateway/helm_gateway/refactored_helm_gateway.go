// PHASE R2: Refactored Helm gateway with improved architecture
package helm_gateway

import (
	"context"

	"deploy-cli/domain"
	"deploy-cli/gateway/helm_gateway/core"
	"deploy-cli/gateway/helm_gateway/management"
	"deploy-cli/gateway/helm_gateway/error_handling"
	"deploy-cli/port/helm_port"
	"deploy-cli/port/logger_port"
)

// RefactoredHelmGateway represents the new, well-structured Helm gateway
type RefactoredHelmGateway struct {
	// Core functionality - essential Helm operations
	deployment core.HelmDeploymentGatewayPort
	template   core.HelmTemplateGatewayPort

	// Management functionality - advanced Helm operations
	validation      management.HelmValidationGatewayPort
	releaseManager  management.HelmReleaseManagerPort
	metadataManager management.HelmMetadataManagerPort

	// Error handling - recovery and troubleshooting
	errorHandler error_handling.HelmErrorHandlerPort

	logger logger_port.LoggerPort
}

// LegacyHelmGateway maintains backward compatibility
type LegacyHelmGateway interface {
	// Core deployment operations
	DeployChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error
	UndeployChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error
	UpgradeChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error
	RollbackChart(ctx context.Context, chart domain.Chart, targetVersion string, options *domain.DeploymentOptions) error
	
	// Status and monitoring
	GetDeploymentStatus(ctx context.Context, releaseName, namespace string) (*domain.ChartStatus, error)
	ListReleases(ctx context.Context, namespace string) ([]*domain.ReleaseInfo, error)
	
	// Template operations
	RenderTemplate(ctx context.Context, chart domain.Chart, options *domain.TemplateOptions) (*domain.TemplateResult, error)
	ValidateTemplate(ctx context.Context, chart domain.Chart, options *domain.TemplateOptions) (*domain.ValidationResult, error)
}

// NewRefactoredHelmGateway creates the new refactored Helm gateway
func NewRefactoredHelmGateway(
	helmPort helm_port.HelmPort,
	logger logger_port.LoggerPort,
) *RefactoredHelmGateway {
	return &RefactoredHelmGateway{
		deployment:      core.NewHelmDeploymentGateway(helmPort, logger),
		template:        core.NewHelmTemplateGateway(helmPort, logger),
		validation:      management.NewHelmValidationGateway(helmPort, logger),
		releaseManager:  management.NewHelmReleaseManager(helmPort, logger),
		metadataManager: management.NewHelmMetadataManager(helmPort, logger),
		errorHandler:    error_handling.NewHelmErrorHandler(helmPort, logger),
		logger:          logger,
	}
}

// Legacy Interface Implementation (Backward Compatibility)

// DeployChart deploys a chart using the specialized deployment gateway
func (r *RefactoredHelmGateway) DeployChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	r.logger.InfoWithContext("PHASE R2: deploying chart with refactored architecture", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
	})

	// Use the specialized deployment component
	return r.deployment.DeployChart(ctx, chart, options)
}

// UndeployChart undeploys a chart using the specialized deployment gateway
func (r *RefactoredHelmGateway) UndeployChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	r.logger.InfoWithContext("PHASE R2: undeploying chart with refactored architecture", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
	})

	// Use the specialized deployment component
	return r.deployment.UndeployChart(ctx, chart, options)
}

// UpgradeChart upgrades a chart using the specialized deployment gateway
func (r *RefactoredHelmGateway) UpgradeChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	r.logger.InfoWithContext("PHASE R2: upgrading chart with refactored architecture", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
	})

	// Use the specialized deployment component
	return r.deployment.UpgradeChart(ctx, chart, options)
}

// RollbackChart rolls back a chart using the specialized deployment gateway
func (r *RefactoredHelmGateway) RollbackChart(ctx context.Context, chart domain.Chart, targetVersion string, options *domain.DeploymentOptions) error {
	r.logger.InfoWithContext("PHASE R2: rolling back chart with refactored architecture", map[string]interface{}{
		"chart":          chart.Name,
		"namespace":      options.GetNamespace(chart.Name),
		"target_version": targetVersion,
	})

	// Use the specialized deployment component
	return r.deployment.RollbackChart(ctx, chart, targetVersion, options)
}

// GetDeploymentStatus gets deployment status using the specialized deployment gateway
func (r *RefactoredHelmGateway) GetDeploymentStatus(ctx context.Context, releaseName, namespace string) (*domain.ChartStatus, error) {
	r.logger.DebugWithContext("PHASE R2: getting deployment status with refactored architecture", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
	})

	// Use the specialized deployment component
	return r.deployment.GetDeploymentStatus(ctx, releaseName, namespace)
}

// ListReleases lists releases using the specialized release manager
func (r *RefactoredHelmGateway) ListReleases(ctx context.Context, namespace string) ([]*domain.ReleaseInfo, error) {
	r.logger.DebugWithContext("PHASE R2: listing releases with refactored architecture", map[string]interface{}{
		"namespace": namespace,
	})

	options := &domain.ReleaseListOptions{
		Namespace:   namespace,
		MaxReleases: 100,
		SortBy:      "name",
		SortOrder:   "asc",
	}

	// Use the specialized release manager component
	return r.releaseManager.ListReleases(ctx, options)
}

// RenderTemplate renders templates using the specialized template gateway
func (r *RefactoredHelmGateway) RenderTemplate(ctx context.Context, chart domain.Chart, options *domain.TemplateOptions) (*domain.TemplateResult, error) {
	r.logger.DebugWithContext("PHASE R2: rendering template with refactored architecture", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.Namespace,
	})

	// Use the specialized template component
	return r.template.RenderTemplate(ctx, chart, options)
}

// ValidateTemplate validates templates using the specialized template gateway
func (r *RefactoredHelmGateway) ValidateTemplate(ctx context.Context, chart domain.Chart, options *domain.TemplateOptions) (*domain.ValidationResult, error) {
	r.logger.DebugWithContext("PHASE R2: validating template with refactored architecture", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.Namespace,
	})

	// Use the specialized template component
	return r.template.ValidateTemplate(ctx, chart, options)
}

// Extended Interface Methods (New Capabilities)

// ValidateChart performs comprehensive chart validation
func (r *RefactoredHelmGateway) ValidateChart(ctx context.Context, chart domain.Chart) (*domain.ValidationResult, error) {
	r.logger.InfoWithContext("PHASE R2: performing comprehensive chart validation", map[string]interface{}{
		"chart": chart.Name,
	})

	// Use the specialized validation component
	return r.validation.ValidateChartStructure(ctx, chart)
}

// ValidateChartCompliance validates chart against compliance rules
func (r *RefactoredHelmGateway) ValidateChartCompliance(ctx context.Context, chart domain.Chart, rules *domain.ComplianceRules) (*domain.ComplianceResult, error) {
	r.logger.InfoWithContext("PHASE R2: validating chart compliance", map[string]interface{}{
		"chart":       chart.Name,
		"rules_count": len(rules.Rules),
	})

	// Use the specialized validation component
	return r.validation.ValidateChartCompliance(ctx, chart, rules)
}

// ValidateChartSecurity performs security validation
func (r *RefactoredHelmGateway) ValidateChartSecurity(ctx context.Context, chart domain.Chart) (*domain.SecurityValidationResult, error) {
	r.logger.InfoWithContext("PHASE R2: validating chart security", map[string]interface{}{
		"chart": chart.Name,
	})

	// Use the specialized validation component
	return r.validation.ValidateChartSecurity(ctx, chart)
}

// GetReleaseHistory gets release history using the specialized release manager
func (r *RefactoredHelmGateway) GetReleaseHistory(ctx context.Context, releaseName, namespace string) ([]*domain.ReleaseRevision, error) {
	r.logger.DebugWithContext("PHASE R2: getting release history", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
	})

	// Use the specialized release manager component
	return r.releaseManager.GetReleaseHistory(ctx, releaseName, namespace)
}

// GetReleaseValues gets release values using the specialized release manager
func (r *RefactoredHelmGateway) GetReleaseValues(ctx context.Context, releaseName, namespace string, allValues bool) (map[string]interface{}, error) {
	r.logger.DebugWithContext("PHASE R2: getting release values", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
		"all_values":   allValues,
	})

	// Use the specialized release manager component
	return r.releaseManager.GetReleaseValues(ctx, releaseName, namespace, allValues)
}

// GetChartMetadata gets chart metadata using the specialized metadata manager
func (r *RefactoredHelmGateway) GetChartMetadata(ctx context.Context, chartPath string) (*domain.ChartMetadata, error) {
	r.logger.DebugWithContext("PHASE R2: getting chart metadata", map[string]interface{}{
		"chart_path": chartPath,
	})

	// Use the specialized metadata manager component
	return r.metadataManager.GetChartMetadata(ctx, chartPath)
}

// GenerateChartSummary generates chart summary using the specialized metadata manager
func (r *RefactoredHelmGateway) GenerateChartSummary(ctx context.Context, chartPath string) (*domain.ChartSummary, error) {
	r.logger.InfoWithContext("PHASE R2: generating chart summary", map[string]interface{}{
		"chart_path": chartPath,
	})

	// Use the specialized metadata manager component
	return r.metadataManager.GenerateChartSummary(ctx, chartPath)
}

// ClassifyError classifies errors using the specialized error handler
func (r *RefactoredHelmGateway) ClassifyError(ctx context.Context, err error, operation string) (*domain.ErrorClassification, error) {
	r.logger.DebugWithContext("PHASE R2: classifying error", map[string]interface{}{
		"operation": operation,
		"error":     err.Error(),
	})

	// Use the specialized error handler component
	return r.errorHandler.ClassifyError(ctx, err, operation)
}

// SuggestRecoveryActions suggests recovery actions using the specialized error handler
func (r *RefactoredHelmGateway) SuggestRecoveryActions(ctx context.Context, classification *domain.ErrorClassification) ([]*domain.RecoveryAction, error) {
	r.logger.DebugWithContext("PHASE R2: suggesting recovery actions", map[string]interface{}{
		"error_type": classification.Type,
		"category":   classification.Category,
	})

	// Use the specialized error handler component
	return r.errorHandler.SuggestRecoveryActions(ctx, classification)
}

// ExecuteRecoveryAction executes recovery actions using the specialized error handler
func (r *RefactoredHelmGateway) ExecuteRecoveryAction(ctx context.Context, action *domain.RecoveryAction, errorContext *domain.ErrorContext) (*domain.RecoveryResult, error) {
	r.logger.InfoWithContext("PHASE R2: executing recovery action", map[string]interface{}{
		"action_type": action.Type,
		"description": action.Description,
	})

	// Use the specialized error handler component
	return r.errorHandler.ExecuteRecoveryAction(ctx, action, errorContext)
}

// TestRelease runs tests using the specialized release manager
func (r *RefactoredHelmGateway) TestRelease(ctx context.Context, releaseName, namespace string, options *domain.TestOptions) (*domain.TestResult, error) {
	r.logger.InfoWithContext("PHASE R2: testing release", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
	})

	// Use the specialized release manager component
	return r.releaseManager.TestRelease(ctx, releaseName, namespace, options)
}

// LintChart lints charts using the specialized template gateway
func (r *RefactoredHelmGateway) LintChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) (*domain.LintResult, error) {
	r.logger.InfoWithContext("PHASE R2: linting chart", map[string]interface{}{
		"chart": chart.Name,
	})

	// Use the specialized template component
	return r.template.LintChart(ctx, chart, options)
}

// DryRunChart performs dry run using the specialized template gateway
func (r *RefactoredHelmGateway) DryRunChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) (*domain.DryRunResult, error) {
	r.logger.InfoWithContext("PHASE R2: performing dry run", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
	})

	// Use the specialized template component
	return r.template.DryRunChart(ctx, chart, options)
}