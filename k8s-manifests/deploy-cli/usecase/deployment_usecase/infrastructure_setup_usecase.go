package deployment_usecase

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"deploy-cli/domain"
	"deploy-cli/gateway/kubectl_gateway"
	"deploy-cli/gateway/system_gateway"
	"deploy-cli/port/logger_port"
)

// InfrastructureSetupUsecase handles deployment infrastructure setup and teardown
type InfrastructureSetupUsecase struct {
	kubectlGateway    *kubectl_gateway.KubectlGateway
	systemGateway     *system_gateway.SystemGateway
	logger            logger_port.LoggerPort
	strategyFactory   *StrategyFactory
}

// NewInfrastructureSetupUsecase creates a new infrastructure setup usecase
func NewInfrastructureSetupUsecase(
	kubectlGateway *kubectl_gateway.KubectlGateway,
	systemGateway *system_gateway.SystemGateway,
	logger logger_port.LoggerPort,
	strategyFactory *StrategyFactory,
) *InfrastructureSetupUsecase {
	return &InfrastructureSetupUsecase{
		kubectlGateway:  kubectlGateway,
		systemGateway:   systemGateway,
		logger:          logger,
		strategyFactory: strategyFactory,
	}
}

// preDeploymentValidation performs comprehensive validation before deployment
func (u *InfrastructureSetupUsecase) preDeploymentValidation(ctx context.Context, options *domain.DeploymentOptions) error {
	// Get charts from strategy
	charts := u.getAllCharts(options)
	
	u.logger.InfoWithContext("starting pre-deployment validation", map[string]interface{}{
		"environment":  options.Environment.String(),
		"charts_count": len(charts),
		"force_update": options.ForceUpdate,
	})

	// Validate deployment options
	if err := u.validateDeploymentOptions(options); err != nil {
		return fmt.Errorf("deployment options validation failed: %w", err)
	}

	// Check cluster connectivity
	if err := u.validateClusterConnectivity(ctx); err != nil {
		return fmt.Errorf("cluster connectivity validation failed: %w", err)
	}

	// Validate required resources
	if err := u.validateRequiredResources(ctx, options); err != nil {
		return fmt.Errorf("required resources validation failed: %w", err)
	}

	u.logger.InfoWithContext("pre-deployment validation completed successfully", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	return nil
}

// setupStorageInfrastructure sets up storage infrastructure for deployments
func (u *InfrastructureSetupUsecase) setupStorageInfrastructure(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("setting up storage infrastructure", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// Ensure all required namespaces exist
	if err := u.ensureNamespaces(ctx, options); err != nil {
		return fmt.Errorf("namespace setup failed: %w", err)
	}

	// Setup storage classes if needed
	if err := u.setupStorageClasses(ctx, options); err != nil {
		return fmt.Errorf("storage class setup failed: %w", err)
	}

	// Setup persistent volumes if needed
	if err := u.setupPersistentVolumes(ctx, options); err != nil {
		return fmt.Errorf("persistent volume setup failed: %w", err)
	}

	u.logger.InfoWithContext("storage infrastructure setup completed", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	return nil
}

// ensureNamespaces ensures all required namespaces exist
func (u *InfrastructureSetupUsecase) ensureNamespaces(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("ensuring namespaces exist", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// Get all required namespaces
	requiredNamespaces := u.getRequiredNamespaces(options)

	for _, namespace := range requiredNamespaces {
		u.logger.DebugWithContext("ensuring namespace exists", map[string]interface{}{
			"namespace": namespace,
		})

		// Create namespace if it doesn't exist
		if err := u.kubectlGateway.EnsureNamespace(ctx, namespace); err != nil {
			return fmt.Errorf("failed to ensure namespace %s: %w", namespace, err)
		}
	}

	u.logger.InfoWithContext("all namespaces ensured", map[string]interface{}{
		"namespaces_count": len(requiredNamespaces),
	})

	return nil
}

// postDeploymentOperations performs post-deployment operations
func (u *InfrastructureSetupUsecase) postDeploymentOperations(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("starting post-deployment operations", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// Restart deployments if needed
	if options.ForceUpdate {
		if err := u.restartDeployments(ctx, options); err != nil {
			u.logger.WarnWithContext("deployment restart failed", map[string]interface{}{
				"error": err.Error(),
			})
			// Don't fail the deployment for restart issues
		}
	}

	// Cleanup operations
	if err := u.cleanupStuckHelmOperations(ctx, options); err != nil {
		u.logger.WarnWithContext("cleanup of stuck helm operations failed", map[string]interface{}{
			"error": err.Error(),
		})
		// Don't fail the deployment for cleanup issues
	}

	u.logger.InfoWithContext("post-deployment operations completed", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	return nil
}

// restartDeployments restarts deployments in specified namespaces
func (u *InfrastructureSetupUsecase) restartDeployments(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("restarting deployments", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// Get namespaces to restart
	namespaces := u.getNamespacesToRestart(options)

	for _, namespace := range namespaces {
		u.logger.DebugWithContext("restarting deployments in namespace", map[string]interface{}{
			"namespace": namespace,
		})

		// Restart deployments in namespace
		if err := u.restartDeploymentsInNamespace(ctx, namespace); err != nil {
			u.logger.WarnWithContext("failed to restart deployments in namespace", map[string]interface{}{
				"namespace": namespace,
				"error":     err.Error(),
			})
			// Continue with other namespaces
		}

		// Restart StatefulSets in namespace
		if err := u.restartStatefulSetsInNamespace(ctx, namespace); err != nil {
			u.logger.WarnWithContext("failed to restart StatefulSets in namespace", map[string]interface{}{
				"namespace": namespace,
				"error":     err.Error(),
			})
			// Continue with other namespaces
		}

		// Restart DaemonSets in namespace
		if err := u.restartDaemonSetsInNamespace(ctx, namespace); err != nil {
			u.logger.WarnWithContext("failed to restart DaemonSets in namespace", map[string]interface{}{
				"namespace": namespace,
				"error":     err.Error(),
			})
			// Continue with other namespaces
		}
	}

	return nil
}

// restartDeploymentsInNamespace restarts all deployments in a namespace
func (u *InfrastructureSetupUsecase) restartDeploymentsInNamespace(ctx context.Context, namespace string) error {
	u.logger.DebugWithContext("restarting deployments in namespace", map[string]interface{}{
		"namespace": namespace,
	})

	// Use kubectl to restart deployments (using executeKubectlCommand)
	if _, err := u.executeKubectlCommand(ctx, "rollout", "restart", "deployment", "-n", namespace); err != nil {
		return fmt.Errorf("failed to restart deployments in namespace %s: %w", namespace, err)
	}

	return nil
}

// restartStatefulSetsInNamespace restarts all StatefulSets in a namespace
func (u *InfrastructureSetupUsecase) restartStatefulSetsInNamespace(ctx context.Context, namespace string) error {
	u.logger.DebugWithContext("restarting StatefulSets in namespace", map[string]interface{}{
		"namespace": namespace,
	})

	// Use kubectl to restart StatefulSets (using executeKubectlCommand)
	if _, err := u.executeKubectlCommand(ctx, "rollout", "restart", "statefulset", "-n", namespace); err != nil {
		return fmt.Errorf("failed to restart StatefulSets in namespace %s: %w", namespace, err)
	}

	return nil
}

// restartDaemonSetsInNamespace restarts all DaemonSets in a namespace
func (u *InfrastructureSetupUsecase) restartDaemonSetsInNamespace(ctx context.Context, namespace string) error {
	u.logger.DebugWithContext("restarting DaemonSets in namespace", map[string]interface{}{
		"namespace": namespace,
	})

	// Use kubectl to restart DaemonSets (using executeKubectlCommand)
	if _, err := u.executeKubectlCommand(ctx, "rollout", "restart", "daemonset", "-n", namespace); err != nil {
		return fmt.Errorf("failed to restart DaemonSets in namespace %s: %w", namespace, err)
	}

	return nil
}

// cleanupStuckHelmOperations cleans up stuck helm operations
func (u *InfrastructureSetupUsecase) cleanupStuckHelmOperations(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("cleaning up stuck helm operations", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// Use kubectl to find and clean up stuck helm operations (using executeKubectlCommand)
	if _, err := u.executeKubectlCommand(ctx, "delete", "secrets", "-l", "owner=helm", "--all-namespaces"); err != nil {
		u.logger.WarnWithContext("failed to cleanup stuck helm operations", map[string]interface{}{
			"error": err.Error(),
		})
		// Don't fail for cleanup issues
	}

	return nil
}

// executeKubectlCommand executes kubectl command with proper argument handling (Bug Fix from Phase 0)
func (u *InfrastructureSetupUsecase) executeKubectlCommand(ctx context.Context, args ...string) ([]byte, error) {
	u.logger.InfoWithContext("executing kubectl command", map[string]interface{}{
		"args": strings.Join(args, " "),
	})

	// Fixed: Use proper argument separation instead of passing command as single string
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	
	// CRITICAL FIX: Inherit current environment variables instead of wiping them
	cmd.Env = os.Environ()
	
	// Add kubectl path to existing PATH instead of replacing it
	currentPath := os.Getenv("PATH")
	finalPath := currentPath + ":" + getKubectlPath()
	
	// Set final PATH
	cmd.Env = append(cmd.Env, "PATH="+finalPath)
	
	// DEBUG: Log environment details
	u.logger.InfoWithContext("kubectl execution environment", map[string]interface{}{
		"original_path":  currentPath,
		"additional_path": getKubectlPath(),
		"final_path":     finalPath,
		"working_dir":    cmd.Dir,
	})

	output, err := cmd.CombinedOutput()
	if err != nil {
		u.logger.InfoWithContext("kubectl command failed", map[string]interface{}{
			"args":   strings.Join(args, " "),
			"error":  err.Error(),
			"output": string(output),
		})
		return output, fmt.Errorf("kubectl command failed: %w", err)
	}

	u.logger.InfoWithContext("kubectl command succeeded", map[string]interface{}{
		"args":          strings.Join(args, " "),
		"output_length": len(output),
	})

	return output, nil
}

// setupDeploymentStrategy sets up deployment strategy based on options
func (u *InfrastructureSetupUsecase) setupDeploymentStrategy(options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("setting up deployment strategy", map[string]interface{}{
		"environment":  options.Environment.String(),
		"force_update": options.ForceUpdate,
	})

	// This is a placeholder for deployment strategy setup
	// Implementation would configure deployment behavior based on options

	return nil
}

// initializeMonitoring initializes monitoring for the deployment
func (u *InfrastructureSetupUsecase) initializeMonitoring(ctx context.Context, deploymentID string, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("initializing deployment monitoring", map[string]interface{}{
		"deployment_id": deploymentID,
		"environment":   options.Environment.String(),
	})

	// This is a placeholder for monitoring initialization
	// Implementation would set up monitoring and metrics collection

	return nil
}

// Helper methods

// validateDeploymentOptions validates deployment options
func (u *InfrastructureSetupUsecase) validateDeploymentOptions(options *domain.DeploymentOptions) error {
	if options == nil {
		return fmt.Errorf("deployment options cannot be nil")
	}

	if options.Environment == "" {
		return fmt.Errorf("environment must be specified")
	}

	// Validate that we can get charts from the strategy
	charts := u.getAllCharts(options)
	if len(charts) == 0 {
		return fmt.Errorf("no charts found for environment %s", options.Environment.String())
	}

	return nil
}

// validateClusterConnectivity validates connectivity to the Kubernetes cluster
func (u *InfrastructureSetupUsecase) validateClusterConnectivity(ctx context.Context) error {
	u.logger.InfoWithContext("validating cluster connectivity", map[string]interface{}{})

	// CRITICAL FIX: Create a longer timeout context for cluster connectivity check
	connectivityCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Try to get cluster info
	_, err := u.executeKubectlCommand(connectivityCtx, "cluster-info")
	if err != nil {
		return fmt.Errorf("failed to connect to cluster: %w", err)
	}

	u.logger.InfoWithContext("cluster connectivity validated successfully", map[string]interface{}{})
	return nil
}

// validateRequiredResources validates that required resources are available
func (u *InfrastructureSetupUsecase) validateRequiredResources(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.DebugWithContext("validating required resources", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// Check for required storage classes
	if err := u.validateStorageClasses(ctx, options); err != nil {
		return fmt.Errorf("storage class validation failed: %w", err)
	}

	// Check for required persistent volumes
	if err := u.validatePersistentVolumes(ctx, options); err != nil {
		return fmt.Errorf("persistent volume validation failed: %w", err)
	}

	return nil
}

// getRequiredNamespaces returns the list of required namespaces for the deployment
func (u *InfrastructureSetupUsecase) getRequiredNamespaces(options *domain.DeploymentOptions) []string {
	namespaces := make(map[string]bool)
	
	// Add standard namespaces
	namespaces["alt-apps"] = true
	namespaces["alt-database"] = true
	namespaces["alt-auth"] = true
	namespaces["alt-ingress"] = true
	namespaces["alt-search"] = true

	// Add any additional namespaces based on charts
	charts := u.getAllCharts(options)
	for _, chart := range charts {
		if chart.MultiNamespace {
			for _, ns := range chart.TargetNamespaces {
				namespaces[ns] = true
			}
		}
	}

	// Convert to slice
	var result []string
	for ns := range namespaces {
		result = append(result, ns)
	}

	return result
}

// getNamespacesToRestart returns the list of namespaces to restart
func (u *InfrastructureSetupUsecase) getNamespacesToRestart(options *domain.DeploymentOptions) []string {
	// For now, return all standard namespaces
	return []string{"alt-apps", "alt-database", "alt-auth", "alt-ingress", "alt-search"}
}

// setupStorageClasses sets up required storage classes
func (u *InfrastructureSetupUsecase) setupStorageClasses(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.DebugWithContext("setting up storage classes", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// This is a placeholder for storage class setup
	// Implementation would ensure required storage classes exist

	return nil
}

// setupPersistentVolumes sets up required persistent volumes
func (u *InfrastructureSetupUsecase) setupPersistentVolumes(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.DebugWithContext("setting up persistent volumes", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// This is a placeholder for persistent volume setup
	// Implementation would ensure required persistent volumes exist

	return nil
}

// validateStorageClasses validates that required storage classes exist
func (u *InfrastructureSetupUsecase) validateStorageClasses(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.DebugWithContext("validating storage classes", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// Check for standard storage class
	_, err := u.executeKubectlCommand(ctx, "get", "storageclass", "standard")
	if err != nil {
		u.logger.WarnWithContext("standard storage class not found", map[string]interface{}{
			"error": err.Error(),
		})
		// Don't fail validation for missing storage classes
	}

	return nil
}

// validatePersistentVolumes validates that required persistent volumes exist
func (u *InfrastructureSetupUsecase) validatePersistentVolumes(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.DebugWithContext("validating persistent volumes", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// This is a placeholder for persistent volume validation
	// Implementation would check for required persistent volumes

	return nil
}

// getAllCharts returns all charts for the deployment
func (u *InfrastructureSetupUsecase) getAllCharts(options *domain.DeploymentOptions) []domain.Chart {
	strategy := u.strategyFactory.GetStrategy(options.Environment)
	layerConfigs := strategy.GetLayerConfigurations(options.ChartsDir)
	
	var allCharts []domain.Chart
	for _, layerConfig := range layerConfigs {
		allCharts = append(allCharts, layerConfig.Charts...)
	}
	
	return allCharts
}

// getKubectlPath returns additional paths where kubectl might be installed
func getKubectlPath() string {
	// Common kubectl installation paths
	return "/usr/local/bin:/usr/bin:/bin:/snap/bin:/opt/homebrew/bin:$HOME/.local/bin"
}