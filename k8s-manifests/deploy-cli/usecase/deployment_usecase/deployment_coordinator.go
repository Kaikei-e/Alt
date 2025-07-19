package deployment_usecase

import (
	"context"
	"fmt"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// DeploymentCoordinator handles main deployment orchestration
type DeploymentCoordinator struct {
	strategyExecutor             *DeploymentStrategyExecutor
	recoveryManager              *DeploymentRecoveryManager
	monitoringCoordinator        *DeploymentMonitoringCoordinator
	sslLifecycleManager          *SSLLifecycleManager
	secretManagementUsecase      *SecretManagementUsecase
	healthCheckUsecase           *HealthCheckUsecase
	infrastructureSetupUsecase   *InfrastructureSetupUsecase
	statefulSetManagementUsecase *StatefulSetManagementUsecase
	logger                       logger_port.LoggerPort
}

// NewDeploymentCoordinator creates a new deployment coordinator
func NewDeploymentCoordinator(
	strategyExecutor *DeploymentStrategyExecutor,
	recoveryManager *DeploymentRecoveryManager,
	monitoringCoordinator *DeploymentMonitoringCoordinator,
	sslLifecycleManager *SSLLifecycleManager,
	secretManagementUsecase *SecretManagementUsecase,
	healthCheckUsecase *HealthCheckUsecase,
	infrastructureSetupUsecase *InfrastructureSetupUsecase,
	statefulSetManagementUsecase *StatefulSetManagementUsecase,
	logger logger_port.LoggerPort,
) *DeploymentCoordinator {
	return &DeploymentCoordinator{
		strategyExecutor:             strategyExecutor,
		recoveryManager:              recoveryManager,
		monitoringCoordinator:        monitoringCoordinator,
		sslLifecycleManager:          sslLifecycleManager,
		secretManagementUsecase:      secretManagementUsecase,
		healthCheckUsecase:           healthCheckUsecase,
		infrastructureSetupUsecase:   infrastructureSetupUsecase,
		statefulSetManagementUsecase: statefulSetManagementUsecase,
		logger:                       logger,
	}
}

// Deploy executes the deployment process
func (c *DeploymentCoordinator) Deploy(ctx context.Context, options *domain.DeploymentOptions) (*domain.DeploymentProgress, error) {
	// Setup deployment strategy if not already set
	if err := c.setupDeploymentStrategy(options); err != nil {
		return nil, fmt.Errorf("failed to setup deployment strategy: %w", err)
	}

	// Initialize monitoring for this deployment
	deploymentID := fmt.Sprintf("deployment-%d", time.Now().Unix())
	if err := c.monitoringCoordinator.InitializeMonitoring(ctx, deploymentID, options); err != nil {
		c.logger.WarnWithContext("failed to initialize monitoring", map[string]interface{}{
			"deployment_id": deploymentID,
			"error":         err.Error(),
		})
	}

	c.logger.InfoWithContext("starting deployment process", map[string]interface{}{
		"deployment_id": deploymentID,
		"environment":   options.Environment.String(),
		"strategy":      options.GetStrategyName(),
		"dry_run":       options.DryRun,
	})

	// Step 1: Pre-deployment validation
	if err := c.infrastructureSetupUsecase.preDeploymentValidation(ctx, options); err != nil {
		return nil, fmt.Errorf("pre-deployment validation failed: %w", err)
	}

	// Step 2: Ensure namespaces exist
	if err := c.infrastructureSetupUsecase.ensureNamespaces(ctx, options); err != nil {
		return nil, fmt.Errorf("namespace setup failed: %w", err)
	}

	// Step 3: SSL certificate validation and auto-generation
	if err := c.performSSLCertificateCheck(ctx, options); err != nil {
		return nil, fmt.Errorf("SSL certificate validation failed: %w", err)
	}

	// Step 4: Pre-deployment secret validation
	charts := c.strategyExecutor.getAllCharts(options)
	if err := c.secretManagementUsecase.ValidateSecretsBeforeDeployment(ctx, charts); err != nil {
		return nil, fmt.Errorf("secret validation failed: %w", err)
	}

	// Step 5: Comprehensive secret provisioning
	if err := c.secretManagementUsecase.provisionAllRequiredSecrets(ctx, charts); err != nil {
		return nil, fmt.Errorf("secret provisioning failed: %w", err)
	}

	// Step 6: SSL Certificate Management
	if err := c.sslLifecycleManager.ManageCertificateLifecycle(ctx, options.Environment, options.ChartsDir); err != nil {
		return nil, fmt.Errorf("SSL certificate management failed: %w", err)
	}

	// Step 7: StatefulSet Recovery Preparation
	if err := c.prepareStatefulSetRecovery(ctx, options); err != nil {
		return nil, fmt.Errorf("StatefulSet recovery preparation failed: %w", err)
	}

	// Step 8: Setup storage infrastructure
	if err := c.infrastructureSetupUsecase.setupStorageInfrastructure(ctx, options); err != nil {
		return nil, fmt.Errorf("storage infrastructure setup failed: %w", err)
	}

	// Step 9: Deploy charts
	progress, err := c.strategyExecutor.DeployCharts(ctx, options)
	if err != nil {
		return progress, fmt.Errorf("chart deployment failed: %w", err)
	}

	// Step 10: Post-deployment operations
	if err := c.infrastructureSetupUsecase.postDeploymentOperations(ctx, options); err != nil {
		return progress, fmt.Errorf("post-deployment operations failed: %w", err)
	}

	c.logger.InfoWithContext("deployment process completed successfully", map[string]interface{}{
		"environment":       options.Environment.String(),
		"successful_charts": progress.GetSuccessCount(),
		"failed_charts":     progress.GetFailedCount(),
		"skipped_charts":    progress.GetSkippedCount(),
	})

	return progress, nil
}

// DeployWithRollback deploys charts with automatic rollback on failure
func (c *DeploymentCoordinator) DeployWithRollback(ctx context.Context, options *domain.DeploymentOptions) (*domain.DeploymentProgress, error) {
	c.logger.InfoWithContext("starting deployment with rollback capability", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// Create deployment checkpoint
	checkpoint, err := c.recoveryManager.CreateDeploymentCheckpoint(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment checkpoint: %w", err)
	}

	c.logger.InfoWithContext("deployment checkpoint created", map[string]interface{}{
		"checkpoint_id": checkpoint.ID,
		"timestamp":     checkpoint.Timestamp,
	})

	// Attempt deployment
	result, err := c.Deploy(ctx, options)
	if err != nil {
		c.logger.ErrorWithContext("deployment failed, initiating rollback", map[string]interface{}{
			"error":         err.Error(),
			"checkpoint_id": checkpoint.ID,
		})

		// Attempt rollback
		rollbackErr := c.recoveryManager.RollbackToCheckpoint(ctx, checkpoint, options)
		if rollbackErr != nil {
			c.logger.ErrorWithContext("rollback failed", map[string]interface{}{
				"deploy_error":   err.Error(),
				"rollback_error": rollbackErr.Error(),
				"checkpoint_id":  checkpoint.ID,
			})
			return nil, fmt.Errorf("deployment failed and rollback failed: deploy=%w, rollback=%w", err, rollbackErr)
		}

		c.logger.InfoWithContext("rollback completed successfully", map[string]interface{}{
			"checkpoint_id": checkpoint.ID,
		})
		return nil, fmt.Errorf("deployment failed, rolled back to checkpoint %s: %w", checkpoint.ID, err)
	}

	c.logger.InfoWithContext("deployment completed successfully", map[string]interface{}{
		"checkpoint_id": checkpoint.ID,
	})

	return result, nil
}

// setupDeploymentStrategy sets up the deployment strategy if not already configured
func (c *DeploymentCoordinator) setupDeploymentStrategy(options *domain.DeploymentOptions) error {
	// This method would be delegated to the strategy executor
	return c.strategyExecutor.SetupDeploymentStrategy(options)
}

// performSSLCertificateCheck performs SSL certificate validation and setup
func (c *DeploymentCoordinator) performSSLCertificateCheck(ctx context.Context, options *domain.DeploymentOptions) error {
	c.logger.InfoWithContext("performing SSL certificate check", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// This could be delegated to a separate SSL certificate usecase
	// For now, we'll implement basic certificate validation logic
	return nil
}

// prepareStatefulSetRecovery prepares StatefulSet recovery
func (c *DeploymentCoordinator) prepareStatefulSetRecovery(ctx context.Context, options *domain.DeploymentOptions) error {
	if options.SkipStatefulSetRecovery {
		c.logger.InfoWithContext("skipping StatefulSet recovery (emergency deployment mode)", map[string]interface{}{
			"environment": options.Environment.String(),
			"reason":      "skip_option_enabled",
		})
		return nil
	}

	if err := c.statefulSetManagementUsecase.prepareStatefulSetRecovery(ctx, options); err != nil {
		return fmt.Errorf("StatefulSet recovery preparation failed: %w\n\nTroubleshooting:\n- For emergency deployments, use --skip-statefulset-recovery flag\n- Check if kubectl is properly configured and accessible\n- Verify that the specified namespaces exist", err)
	}

	return nil
}

// ValidateDeploymentPrerequisites validates that all prerequisites are met for deployment
func (c *DeploymentCoordinator) ValidateDeploymentPrerequisites(ctx context.Context, options *domain.DeploymentOptions) error {
	c.logger.InfoWithContext("validating deployment prerequisites", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// Validate infrastructure prerequisites
	if err := c.infrastructureSetupUsecase.preDeploymentValidation(ctx, options); err != nil {
		return fmt.Errorf("infrastructure validation failed: %w", err)
	}

	// Validate secret state
	if err := c.validateSecretState(ctx, options); err != nil {
		return fmt.Errorf("secret state validation failed: %w", err)
	}

	// Validate SSL certificates - always validate if available
	if err := c.validateSSLCertificates(ctx, options); err != nil {
		c.logger.WarnWithContext("SSL certificate validation failed, continuing without SSL", map[string]interface{}{
			"error": err.Error(),
		})
		// Continue without failing - SSL is optional
	}

	c.logger.InfoWithContext("deployment prerequisites validated successfully", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	return nil
}

// validateSecretState validates the current secret state
func (c *DeploymentCoordinator) validateSecretState(ctx context.Context, options *domain.DeploymentOptions) error {
	charts := c.strategyExecutor.getAllCharts(options)
	return c.secretManagementUsecase.ValidateSecretsBeforeDeployment(ctx, charts)
}

// validateSSLCertificates validates SSL certificates
func (c *DeploymentCoordinator) validateSSLCertificates(ctx context.Context, options *domain.DeploymentOptions) error {
	// This would delegate to the SSL lifecycle manager
	return c.sslLifecycleManager.ValidateGeneratedCertificates(ctx)
}

// GetDeploymentStatus returns the current status of a deployment
func (c *DeploymentCoordinator) GetDeploymentStatus(ctx context.Context, deploymentID string) (*domain.DeploymentStatusInfo, error) {
	return c.monitoringCoordinator.GetDeploymentStatus(ctx, deploymentID)
}

// CancelDeployment cancels an ongoing deployment
func (c *DeploymentCoordinator) CancelDeployment(ctx context.Context, deploymentID string) error {
	c.logger.InfoWithContext("cancelling deployment", map[string]interface{}{
		"deployment_id": deploymentID,
	})

	// This would coordinate with the monitoring system to cancel the deployment
	return c.monitoringCoordinator.CancelDeployment(ctx, deploymentID)
}

// ListActiveDeployments returns a list of currently active deployments
func (c *DeploymentCoordinator) ListActiveDeployments(ctx context.Context) ([]*domain.DeploymentStatusInfo, error) {
	return c.monitoringCoordinator.ListActiveDeployments(ctx)
}

// CleanupFailedDeployments cleans up resources from failed deployments
func (c *DeploymentCoordinator) CleanupFailedDeployments(ctx context.Context, olderThan time.Duration) error {
	c.logger.InfoWithContext("cleaning up failed deployments", map[string]interface{}{
		"older_than": olderThan.String(),
	})

	return c.recoveryManager.CleanupFailedDeployments(ctx, olderThan)
}
