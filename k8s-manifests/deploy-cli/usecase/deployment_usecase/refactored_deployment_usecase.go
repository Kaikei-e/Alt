// PHASE R1: Refactored deployment usecase with improved architecture
package deployment_usecase

import (
	"context"
	"fmt"
	"time"

	"deploy-cli/domain"
	"deploy-cli/usecase/deployment_usecase/core"
	"deploy-cli/usecase/deployment_usecase/orchestration"
	"deploy-cli/usecase/deployment_usecase/monitoring"
	"deploy-cli/usecase/deployment_usecase/recovery"
	"deploy-cli/port/logger_port"
)

// RefactoredDeploymentUsecase represents the new, well-structured deployment usecase
type RefactoredDeploymentUsecase struct {
	// Core functionality - essential deployment operations
	core         core.CoreDeploymentUsecasePort
	executor     core.ChartDeploymentExecutorPort
	namespaceManager core.NamespaceManagerPort

	// Orchestration - complex workflow management
	orchestrator   orchestration.DeploymentOrchestratorPort
	layerManager   orchestration.LayerManagerPort
	dependencyResolver orchestration.DependencyResolverPort

	// Monitoring - health checking and metrics
	monitor       monitoring.DeploymentMonitorPort
	healthChecker monitoring.HealthCheckerPort

	// Recovery - rollback and repair operations
	recovery      recovery.DeploymentRecoveryPort

	logger logger_port.LoggerPort
}

// CleanupOptions represents cleanup options for the interface
type CleanupOptions struct {
	DeploymentID string
	Namespace    string
	Force        bool
}

// LegacyDeploymentUsecase maintains backward compatibility
type LegacyDeploymentUsecase interface {
	Deploy(ctx context.Context, options *domain.DeploymentOptions) (*domain.DeploymentProgress, error)
	Monitor(ctx context.Context, namespace string, timeout time.Duration) error
	Rollback(ctx context.Context, options *domain.RollbackOptions) error
	Diagnose(ctx context.Context, namespace string) (*domain.DiagnosisResult, error)
	Cleanup(ctx context.Context, options *CleanupOptions) error
}

// NewRefactoredDeploymentUsecase creates the new refactored deployment usecase
func NewRefactoredDeploymentUsecase(
	core core.CoreDeploymentUsecasePort,
	executor core.ChartDeploymentExecutorPort,
	namespaceManager core.NamespaceManagerPort,
	orchestrator orchestration.DeploymentOrchestratorPort,
	layerManager orchestration.LayerManagerPort,
	dependencyResolver orchestration.DependencyResolverPort,
	monitor monitoring.DeploymentMonitorPort,
	healthChecker monitoring.HealthCheckerPort,
	recovery recovery.DeploymentRecoveryPort,
	logger logger_port.LoggerPort,
) *RefactoredDeploymentUsecase {
	return &RefactoredDeploymentUsecase{
		core:               core,
		executor:           executor,
		namespaceManager:   namespaceManager,
		orchestrator:       orchestrator,
		layerManager:       layerManager,
		dependencyResolver: dependencyResolver,
		monitor:            monitor,
		healthChecker:      healthChecker,
		recovery:           recovery,
		logger:             logger,
	}
}

// Deploy executes a deployment using the new architecture
func (r *RefactoredDeploymentUsecase) Deploy(ctx context.Context, options *domain.DeploymentOptions) (*domain.DeploymentProgress, error) {
	r.logger.InfoWithContext("PHASE R1: executing deployment with refactored architecture", map[string]interface{}{
		"strategy":     "default",
		"charts_dir":   options.ChartsDir,
	})

	// Use the orchestrator for complex deployment workflows
	return r.orchestrator.ExecuteDeployment(ctx, options)
}

// Monitor monitors a deployment using the specialized monitoring component
func (r *RefactoredDeploymentUsecase) Monitor(ctx context.Context, namespace string, timeout time.Duration) error {
	r.logger.InfoWithContext("PHASE R1: monitoring deployment with refactored architecture", map[string]interface{}{
		"namespace": namespace,
		"timeout":   timeout.String(),
	})

	monitoringOptions := &domain.MonitoringOptions{
		Timeout:       timeout,
		CheckInterval: 10 * time.Second,
		EnableAlerts:  false,
	}

	// Start monitoring
	_, err := r.monitor.MonitorDeployment(ctx, fmt.Sprintf("deployment-%s", namespace), monitoringOptions)
	return err
}

// Rollback executes a rollback using the specialized recovery component
func (r *RefactoredDeploymentUsecase) Rollback(ctx context.Context, options *domain.RollbackOptions) error {
	r.logger.InfoWithContext("PHASE R1: executing rollback with refactored architecture", map[string]interface{}{
		"revision": options.Revision,
	})

	return r.recovery.RollbackDeployment(ctx, fmt.Sprintf("rollback-%d", time.Now().Unix()), fmt.Sprintf("%d", options.Revision))
}

// Diagnose performs deployment diagnosis using the recovery component
func (r *RefactoredDeploymentUsecase) Diagnose(ctx context.Context, namespace string) (*domain.DiagnosisResult, error) {
	r.logger.InfoWithContext("PHASE R1: diagnosing deployment with refactored architecture", map[string]interface{}{
		"namespace": namespace,
	})

	// Create a deployment ID for diagnosis
	deploymentID := fmt.Sprintf("diagnosis-%s-%d", namespace, time.Now().Unix())
	
	return r.recovery.DiagnoseDeploymentIssues(ctx, deploymentID)
}

// Cleanup performs cleanup using the recovery component
func (r *RefactoredDeploymentUsecase) Cleanup(ctx context.Context, options *CleanupOptions) error {
	r.logger.InfoWithContext("PHASE R1: cleaning up deployment with refactored architecture", map[string]interface{}{
		"deployment_id": options.DeploymentID,
		"namespace":     options.Namespace,
	})

	return r.recovery.CleanupFailedDeployment(ctx, options.DeploymentID)
}

// DeployChart deploys a single chart using the core component
func (r *RefactoredDeploymentUsecase) DeployChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	r.logger.InfoWithContext("PHASE R1: deploying single chart with refactored architecture", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
	})

	return r.core.DeployChart(ctx, chart, options)
}

// GetDeploymentHealth gets deployment health using the monitoring component
func (r *RefactoredDeploymentUsecase) GetDeploymentHealth(ctx context.Context, deploymentID string) (*domain.HealthStatus, error) {
	r.logger.DebugWithContext("PHASE R1: getting deployment health with refactored architecture", map[string]interface{}{
		"deployment_id": deploymentID,
	})

	return r.monitor.GetDeploymentHealth(ctx, deploymentID)
}

// ValidateDependencies validates chart dependencies using the orchestration component
func (r *RefactoredDeploymentUsecase) ValidateDependencies(ctx context.Context, charts []domain.Chart) error {
	r.logger.DebugWithContext("PHASE R1: validating dependencies with refactored architecture", map[string]interface{}{
		"chart_count": len(charts),
	})

	return r.dependencyResolver.ValidateDependencies(charts)
}

// GetLayerConfigurations gets layer configurations using the orchestration component
func (r *RefactoredDeploymentUsecase) GetLayerConfigurations(chartConfig *domain.ChartConfig, chartsDir string) []domain.LayerConfiguration {
	r.logger.DebugWithContext("PHASE R1: getting layer configurations with refactored architecture", map[string]interface{}{
		"charts_dir": chartsDir,
	})

	return r.layerManager.GetLayerConfigurations(chartConfig, chartsDir)
}

// RecoverFailedDeployment recovers a failed deployment using the recovery component
func (r *RefactoredDeploymentUsecase) RecoverFailedDeployment(ctx context.Context, deploymentID string, options *domain.RecoveryOptions) (*domain.RecoveryResult, error) {
	r.logger.InfoWithContext("PHASE R1: recovering failed deployment with refactored architecture", map[string]interface{}{
		"deployment_id": deploymentID,
		"auto_rollback": options.AutoRollback,
	})

	return r.recovery.RecoverFailedDeployment(ctx, deploymentID, options)
}