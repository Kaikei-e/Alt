package deployment_usecase

import (
	"context"
	"fmt"
	"time"
	
	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
	"deploy-cli/port/filesystem_port"
	"deploy-cli/gateway/helm_gateway"
	"deploy-cli/gateway/kubectl_gateway"
	"deploy-cli/gateway/filesystem_gateway"
	"deploy-cli/gateway/system_gateway"
	"deploy-cli/usecase/secret_usecase"
	"deploy-cli/usecase/dependency_usecase"
)

// DeploymentUsecase handles deployment operations
type DeploymentUsecase struct {
	helmGateway         *helm_gateway.HelmGateway
	kubectlGateway      *kubectl_gateway.KubectlGateway
	filesystemGateway   *filesystem_gateway.FileSystemGateway
	systemGateway       *system_gateway.SystemGateway
	secretUsecase       *secret_usecase.SecretUsecase
	logger              logger_port.LoggerPort
	parallelDeployer    *ParallelChartDeployer
	cache               *DeploymentCache
	dependencyScanner   *dependency_usecase.DependencyScanner
	enableParallel      bool
	enableCache         bool
	enableDependencyAware bool
}

// NewDeploymentUsecase creates a new deployment usecase
func NewDeploymentUsecase(
	helmGateway *helm_gateway.HelmGateway,
	kubectlGateway *kubectl_gateway.KubectlGateway,
	filesystemGateway *filesystem_gateway.FileSystemGateway,
	systemGateway *system_gateway.SystemGateway,
	secretUsecase *secret_usecase.SecretUsecase,
	logger logger_port.LoggerPort,
	filesystemPort filesystem_port.FileSystemPort,
) *DeploymentUsecase {
	dependencyScanner := dependency_usecase.NewDependencyScanner(filesystemPort, logger)
	
	return &DeploymentUsecase{
		helmGateway:           helmGateway,
		kubectlGateway:        kubectlGateway,
		filesystemGateway:     filesystemGateway,
		systemGateway:         systemGateway,
		secretUsecase:         secretUsecase,
		logger:                logger,
		dependencyScanner:     dependencyScanner,
		enableParallel:        false,  // Will be configurable
		enableCache:           false,  // Will be configurable
		enableDependencyAware: true,   // Enable by default
	}
}

// Deploy executes the deployment process
func (u *DeploymentUsecase) Deploy(ctx context.Context, options *domain.DeploymentOptions) (*domain.DeploymentProgress, error) {
	u.logger.InfoWithContext("starting deployment process", map[string]interface{}{
		"environment": options.Environment.String(),
		"dry_run":     options.DryRun,
	})
	
	// Step 1: Pre-deployment validation
	if err := u.preDeploymentValidation(ctx, options); err != nil {
		return nil, fmt.Errorf("pre-deployment validation failed: %w", err)
	}
	
	// Step 2: Setup storage infrastructure
	if err := u.setupStorageInfrastructure(ctx, options); err != nil {
		return nil, fmt.Errorf("storage infrastructure setup failed: %w", err)
	}
	
	// Step 3: Ensure namespaces exist
	if err := u.ensureNamespaces(ctx, options); err != nil {
		return nil, fmt.Errorf("namespace setup failed: %w", err)
	}
	
	// Step 4: Deploy charts
	progress, err := u.deployCharts(ctx, options)
	if err != nil {
		return progress, fmt.Errorf("chart deployment failed: %w", err)
	}
	
	// Step 5: Post-deployment operations
	if err := u.postDeploymentOperations(ctx, options); err != nil {
		return progress, fmt.Errorf("post-deployment operations failed: %w", err)
	}
	
	u.logger.InfoWithContext("deployment process completed successfully", map[string]interface{}{
		"environment":      options.Environment.String(),
		"successful_charts": progress.GetSuccessCount(),
		"failed_charts":    progress.GetFailedCount(),
		"skipped_charts":   progress.GetSkippedCount(),
	})
	
	return progress, nil
}

// preDeploymentValidation performs pre-deployment validation
func (u *DeploymentUsecase) preDeploymentValidation(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("performing pre-deployment validation", map[string]interface{}{
		"environment": options.Environment.String(),
	})
	
	// Validate required commands
	requiredCommands := []string{"helm", "kubectl"}
	if err := u.systemGateway.ValidateRequiredCommands(requiredCommands); err != nil {
		return fmt.Errorf("required commands validation failed: %w", err)
	}
	
	// Validate cluster access
	if err := u.kubectlGateway.ValidateClusterAccess(ctx); err != nil {
		return fmt.Errorf("cluster access validation failed: %w", err)
	}
	
	// Validate secret state and resolve conflicts
	if err := u.validateAndFixSecrets(ctx, options); err != nil {
		return fmt.Errorf("secret validation failed: %w", err)
	}
	
	u.logger.InfoWithContext("pre-deployment validation completed", map[string]interface{}{
		"environment": options.Environment.String(),
	})
	
	return nil
}

// setupStorageInfrastructure sets up storage infrastructure
func (u *DeploymentUsecase) setupStorageInfrastructure(ctx context.Context, options *domain.DeploymentOptions) error {
	if options.DryRun {
		u.logger.InfoWithContext("dry-run: skipping storage infrastructure setup", map[string]interface{}{})
		return nil
	}
	
	u.logger.InfoWithContext("setting up storage infrastructure", map[string]interface{}{
		"environment": options.Environment.String(),
	})
	
	// Create storage configuration
	storageConfig := domain.NewStorageConfig()
	
	// Validate storage paths
	if err := u.filesystemGateway.ValidateStoragePaths(storageConfig); err != nil {
		return fmt.Errorf("storage paths validation failed: %w", err)
	}
	
	// Create persistent volumes
	for _, pv := range storageConfig.PersistentVolumes {
		if err := u.kubectlGateway.CreatePersistentVolume(ctx, pv); err != nil {
			u.logger.ErrorWithContext("failed to create persistent volume - this may cause chart deployment issues", map[string]interface{}{
				"pv_name":        pv.Name,
				"pv_capacity":    pv.Capacity,
				"pv_path":        pv.HostPath,
				"storage_class":  pv.StorageClass,
				"error":          err.Error(),
				"resolution":     "Check if the storage class exists and the host path is accessible",
			})
		} else {
			u.logger.InfoWithContext("persistent volume created successfully", map[string]interface{}{
				"pv_name":     pv.Name,
				"pv_capacity": pv.Capacity,
			})
		}
	}
	
	u.logger.InfoWithContext("storage infrastructure setup completed", map[string]interface{}{
		"environment": options.Environment.String(),
	})
	
	return nil
}

// ensureNamespaces ensures that all required namespaces exist
func (u *DeploymentUsecase) ensureNamespaces(ctx context.Context, options *domain.DeploymentOptions) error {
	if options.DryRun {
		u.logger.InfoWithContext("dry-run: skipping namespace creation", map[string]interface{}{})
		return nil
	}
	
	u.logger.InfoWithContext("ensuring namespaces exist", map[string]interface{}{
		"environment": options.Environment.String(),
	})
	
	if err := u.kubectlGateway.EnsureNamespaces(ctx, options.Environment); err != nil {
		return fmt.Errorf("failed to ensure namespaces: %w", err)
	}
	
	u.logger.InfoWithContext("namespaces ensured", map[string]interface{}{
		"environment": options.Environment.String(),
	})
	
	return nil
}

// deployCharts deploys all charts in the correct order
func (u *DeploymentUsecase) deployCharts(ctx context.Context, options *domain.DeploymentOptions) (*domain.DeploymentProgress, error) {
	u.logger.InfoWithContext("deploying charts", map[string]interface{}{
		"environment": options.Environment.String(),
		"dependency_aware": u.enableDependencyAware,
	})
	
	// Get chart configuration
	chartConfig := domain.NewChartConfig(options.ChartsDir)
	allCharts := chartConfig.AllCharts()
	
	// Create deployment progress
	progress := domain.NewDeploymentProgress(len(allCharts))
	
	// Use dependency-aware deployment if enabled
	if u.enableDependencyAware {
		return u.deployChartsWithDependencyAwareness(ctx, options, progress)
	}
	
	// Fallback to traditional group-based deployment
	// Deploy infrastructure charts
	if err := u.deployChartGroup(ctx, "Infrastructure", chartConfig.InfrastructureCharts, options, progress); err != nil {
		return progress, fmt.Errorf("infrastructure chart deployment failed: %w", err)
	}
	
	// Deploy application charts
	if err := u.deployChartGroup(ctx, "Application", chartConfig.ApplicationCharts, options, progress); err != nil {
		return progress, fmt.Errorf("application chart deployment failed: %w", err)
	}
	
	// Deploy operational charts
	if err := u.deployChartGroup(ctx, "Operational", chartConfig.OperationalCharts, options, progress); err != nil {
		return progress, fmt.Errorf("operational chart deployment failed: %w", err)
	}
	
	// Validate pod updates if force update was enabled or if images were updated
	if options.ForceUpdate || options.ShouldOverrideImage() {
		if err := u.validatePodUpdates(ctx, options); err != nil {
			u.logger.WarnWithContext("pod update validation failed", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}
	
	u.logger.InfoWithContext("charts deployment completed", map[string]interface{}{
		"environment":      options.Environment.String(),
		"successful_charts": progress.GetSuccessCount(),
		"failed_charts":    progress.GetFailedCount(),
		"skipped_charts":   progress.GetSkippedCount(),
	})
	
	return progress, nil
}

// deployChartGroup deploys a group of charts
func (u *DeploymentUsecase) deployChartGroup(ctx context.Context, groupName string, charts []domain.Chart, options *domain.DeploymentOptions, progress *domain.DeploymentProgress) error {
	u.logger.InfoWithContext("deploying chart group", map[string]interface{}{
		"group":       groupName,
		"chart_count": len(charts),
	})
	
	var failedCharts []string
	
	for _, chart := range charts {
		// Check if context was cancelled
		if ctx.Err() != nil {
			u.logger.WarnWithContext("deployment cancelled", map[string]interface{}{
				"group": groupName,
				"chart": chart.Name,
				"error": ctx.Err().Error(),
			})
			return ctx.Err()
		}
		
		progress.CurrentChart = chart.Name
		progress.CurrentPhase = fmt.Sprintf("Deploying %s charts", groupName)
		
		result := u.deploySingleChart(ctx, chart, options)
		progress.AddResult(result)
		
		// Collect failed charts but continue if dry run
		if result.Status == domain.DeploymentStatusFailed {
			failedCharts = append(failedCharts, chart.Name)
			u.logger.ErrorWithContext("chart deployment failed", map[string]interface{}{
				"group": groupName,
				"chart": chart.Name,
				"error": result.Error,
			})
			
			// Stop on first failure if not dry run
			if !options.DryRun {
				return fmt.Errorf("chart deployment failed: %s", result.Error)
			}
		}
	}
	
	u.logger.InfoWithContext("chart group deployment completed", map[string]interface{}{
		"group":        groupName,
		"chart_count":  len(charts),
		"failed_count": len(failedCharts),
		"failed_charts": failedCharts,
	})
	
	return nil
}

// deploySingleChart deploys a single chart
func (u *DeploymentUsecase) deploySingleChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) domain.DeploymentResult {
	start := time.Now()
	namespace := options.GetNamespace(chart.Name)
	
	u.logger.InfoWithContext("deploying single chart", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": namespace,
	})
	
	result := domain.DeploymentResult{
		ChartName: chart.Name,
		Namespace: namespace,
		Status:    domain.DeploymentStatusFailed,
		Duration:  0,
	}
	
	// Create timeout context for individual chart deployment
	chartTimeout := options.Timeout
	if chartTimeout == 0 {
		chartTimeout = 5 * time.Minute // Default timeout
	}
	chartCtx, cancel := context.WithTimeout(ctx, chartTimeout)
	defer cancel()
	
	// Validate chart path
	if err := u.filesystemGateway.ValidateChartPath(chart); err != nil {
		result.Error = fmt.Errorf("chart path validation failed: %w", err)
		result.Status = domain.DeploymentStatusSkipped
		result.Duration = time.Since(start)
		return result
	}
	
	// Validate values file
	valuesFile, err := u.filesystemGateway.ValidateValuesFile(chart, options.Environment)
	if err != nil {
		result.Error = fmt.Errorf("values file validation failed: %w", err)
		result.Status = domain.DeploymentStatusSkipped
		result.Duration = time.Since(start)
		return result
	}
	
	// Deploy or template chart with timeout handling
	if options.DryRun {
		_, err = u.helmGateway.TemplateChart(chartCtx, chart, options)
		if err != nil {
			if chartCtx.Err() == context.DeadlineExceeded {
				result.Error = fmt.Errorf("chart templating timed out after %v", chartTimeout)
			} else {
				result.Error = fmt.Errorf("chart templating failed: %w", err)
			}
		} else {
			result.Status = domain.DeploymentStatusSuccess
			result.Message = "Chart templated successfully"
		}
	} else {
		err = u.helmGateway.DeployChart(chartCtx, chart, options)
		if err != nil {
			if chartCtx.Err() == context.DeadlineExceeded {
				result.Error = fmt.Errorf("chart deployment timed out after %v", chartTimeout)
			} else {
				result.Error = fmt.Errorf("chart deployment failed: %w", err)
			}
		} else {
			result.Status = domain.DeploymentStatusSuccess
			result.Message = "Chart deployed successfully"
		}
	}
	
	result.Duration = time.Since(start)
	
	u.logger.InfoWithContext("single chart deployment completed", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": namespace,
		"status":    result.Status,
		"duration":  result.Duration,
		"values_file": valuesFile,
		"timeout":   chartTimeout,
	})
	
	return result
}

// postDeploymentOperations performs post-deployment operations
func (u *DeploymentUsecase) postDeploymentOperations(ctx context.Context, options *domain.DeploymentOptions) error {
	if options.DryRun {
		u.logger.InfoWithContext("dry-run: skipping post-deployment operations", map[string]interface{}{})
		return nil
	}
	
	u.logger.InfoWithContext("performing post-deployment operations", map[string]interface{}{
		"environment": options.Environment.String(),
	})
	
	// Restart deployments if requested
	if options.DoRestart {
		if err := u.restartDeployments(ctx, options); err != nil {
			return fmt.Errorf("deployment restart failed: %w", err)
		}
	}
	
	u.logger.InfoWithContext("post-deployment operations completed", map[string]interface{}{
		"environment": options.Environment.String(),
	})
	
	return nil
}

// restartDeployments restarts all workload resources (deployments, statefulsets, daemonsets)
func (u *DeploymentUsecase) restartDeployments(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("restarting workload resources", map[string]interface{}{
		"environment": options.Environment.String(),
	})
	
	namespaces := domain.GetNamespacesForEnvironment(options.Environment)
	
	for _, namespace := range namespaces {
		// Restart deployments
		if err := u.restartDeploymentsInNamespace(ctx, namespace); err != nil {
			u.logger.WarnWithContext("failed to restart deployments in namespace", map[string]interface{}{
				"namespace": namespace,
				"error":     err.Error(),
			})
		}
		
		// Restart StatefulSets
		if err := u.restartStatefulSetsInNamespace(ctx, namespace); err != nil {
			u.logger.WarnWithContext("failed to restart statefulsets in namespace", map[string]interface{}{
				"namespace": namespace,
				"error":     err.Error(),
			})
		}
		
		// Restart DaemonSets (if method exists)
		if err := u.restartDaemonSetsInNamespace(ctx, namespace); err != nil {
			u.logger.WarnWithContext("failed to restart daemonsets in namespace", map[string]interface{}{
				"namespace": namespace,
				"error":     err.Error(),
			})
		}
	}
	
	u.logger.InfoWithContext("workload resource restart completed", map[string]interface{}{
		"environment": options.Environment.String(),
	})
	
	return nil
}

// restartDeploymentsInNamespace restarts deployments in a specific namespace
func (u *DeploymentUsecase) restartDeploymentsInNamespace(ctx context.Context, namespace string) error {
	deployments, err := u.kubectlGateway.GetDeployments(ctx, namespace)
	if err != nil {
		return fmt.Errorf("failed to get deployments: %w", err)
	}
	
	for _, deployment := range deployments {
		if err := u.kubectlGateway.RolloutRestart(ctx, "deployment", deployment.Name, namespace); err != nil {
			u.logger.WarnWithContext("failed to restart deployment", map[string]interface{}{
				"deployment": deployment.Name,
				"namespace":  namespace,
				"error":      err.Error(),
			})
		} else {
			u.logger.InfoWithContext("deployment restarted", map[string]interface{}{
				"deployment": deployment.Name,
				"namespace":  namespace,
			})
			
			// Wait for rollout completion
			if err := u.kubectlGateway.WaitForRollout(ctx, "deployment", deployment.Name, namespace, 5*time.Minute); err != nil {
				u.logger.WarnWithContext("deployment rollout did not complete", map[string]interface{}{
					"deployment": deployment.Name,
					"namespace":  namespace,
					"error":      err.Error(),
				})
			} else {
				u.logger.InfoWithContext("deployment rollout completed", map[string]interface{}{
					"deployment": deployment.Name,
					"namespace":  namespace,
				})
			}
		}
	}
	
	return nil
}

// restartStatefulSetsInNamespace restarts StatefulSets in a specific namespace
func (u *DeploymentUsecase) restartStatefulSetsInNamespace(ctx context.Context, namespace string) error {
	statefulSets, err := u.kubectlGateway.GetStatefulSets(ctx, namespace)
	if err != nil {
		return fmt.Errorf("failed to get statefulsets: %w", err)
	}
	
	for _, sts := range statefulSets {
		if err := u.kubectlGateway.RolloutRestart(ctx, "statefulset", sts.Name, namespace); err != nil {
			u.logger.WarnWithContext("failed to restart statefulset", map[string]interface{}{
				"statefulset": sts.Name,
				"namespace":   namespace,
				"error":       err.Error(),
			})
		} else {
			u.logger.InfoWithContext("statefulset restarted", map[string]interface{}{
				"statefulset": sts.Name,
				"namespace":   namespace,
			})
			
			// Wait for rollout completion (StatefulSets may take longer)
			if err := u.kubectlGateway.WaitForRollout(ctx, "statefulset", sts.Name, namespace, 10*time.Minute); err != nil {
				u.logger.WarnWithContext("statefulset rollout did not complete", map[string]interface{}{
					"statefulset": sts.Name,
					"namespace":   namespace,
					"error":       err.Error(),
				})
			} else {
				u.logger.InfoWithContext("statefulset rollout completed", map[string]interface{}{
					"statefulset": sts.Name,
					"namespace":   namespace,
				})
			}
		}
	}
	
	return nil
}

// restartDaemonSetsInNamespace restarts DaemonSets in a specific namespace
func (u *DeploymentUsecase) restartDaemonSetsInNamespace(ctx context.Context, namespace string) error {
	// Note: DaemonSets don't support rollout restart in the same way
	// We'll skip them for now as they typically don't need manual restarts
	// and have different update strategies
	return nil
}

// validatePodUpdates validates that pods have been updated after deployment
func (u *DeploymentUsecase) validatePodUpdates(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("validating pod updates", map[string]interface{}{
		"environment": options.Environment.String(),
	})
	
	namespaces := domain.GetNamespacesForEnvironment(options.Environment)
	
	for _, namespace := range namespaces {
		// Check deployment pods
		if err := u.validateDeploymentPods(ctx, namespace); err != nil {
			u.logger.WarnWithContext("deployment pod validation failed", map[string]interface{}{
				"namespace": namespace,
				"error":     err.Error(),
			})
		}
		
		// Check StatefulSet pods  
		if err := u.validateStatefulSetPods(ctx, namespace); err != nil {
			u.logger.WarnWithContext("statefulset pod validation failed", map[string]interface{}{
				"namespace": namespace,
				"error":     err.Error(),
			})
		}
	}
	
	u.logger.InfoWithContext("pod update validation completed", map[string]interface{}{
		"environment": options.Environment.String(),
	})
	
	return nil
}

// validateDeploymentPods validates that deployment pods are running and updated
func (u *DeploymentUsecase) validateDeploymentPods(ctx context.Context, namespace string) error {
	deployments, err := u.kubectlGateway.GetDeployments(ctx, namespace)
	if err != nil {
		return fmt.Errorf("failed to get deployments: %w", err)
	}
	
	for _, deployment := range deployments {
		// Wait for deployment rollout to complete
		if err := u.kubectlGateway.WaitForRollout(ctx, "deployment", deployment.Name, namespace, 5*time.Minute); err != nil {
			u.logger.WarnWithContext("deployment rollout did not complete within timeout", map[string]interface{}{
				"deployment": deployment.Name,
				"namespace":  namespace,
				"error":      err.Error(),
			})
		}
		
		// Check pod status for this deployment
		pods, err := u.kubectlGateway.GetPods(ctx, namespace, fmt.Sprintf("app.kubernetes.io/name=%s", deployment.Name))
		if err != nil {
			u.logger.WarnWithContext("failed to get pods for deployment", map[string]interface{}{
				"deployment": deployment.Name,
				"namespace":  namespace,
				"error":      err.Error(),
			})
			continue
		}
		
		// Count running pods
		runningPods := 0
		for i := range pods {
			if pods[i].Status == "Running" {
				runningPods++
			}
		}
		
		u.logger.InfoWithContext("deployment pod status validation", map[string]interface{}{
			"deployment":   deployment.Name,
			"namespace":    namespace,
			"total_pods":   len(pods),
			"running_pods": runningPods,
			"ready_replicas": deployment.ReadyReplicas,
			"desired_replicas": deployment.Replicas,
		})
		
		// Validate that deployment has expected number of ready replicas
		if deployment.ReadyReplicas != deployment.Replicas {
			u.logger.WarnWithContext("deployment does not have all replicas ready", map[string]interface{}{
				"deployment":       deployment.Name,
				"namespace":        namespace,
				"ready_replicas":   deployment.ReadyReplicas,
				"desired_replicas": deployment.Replicas,
			})
		}
	}
	
	return nil
}

// validateStatefulSetPods validates that StatefulSet pods are running and updated
func (u *DeploymentUsecase) validateStatefulSetPods(ctx context.Context, namespace string) error {
	statefulSets, err := u.kubectlGateway.GetStatefulSets(ctx, namespace)
	if err != nil {
		return fmt.Errorf("failed to get statefulsets: %w", err)
	}
	
	for _, sts := range statefulSets {
		// Wait for StatefulSet rollout to complete (longer timeout for StatefulSets)
		if err := u.kubectlGateway.WaitForRollout(ctx, "statefulset", sts.Name, namespace, 10*time.Minute); err != nil {
			u.logger.WarnWithContext("statefulset rollout did not complete within timeout", map[string]interface{}{
				"statefulset": sts.Name,
				"namespace":   namespace,
				"error":       err.Error(),
			})
		}
		
		// Check pod status for this StatefulSet
		pods, err := u.kubectlGateway.GetPods(ctx, namespace, fmt.Sprintf("app.kubernetes.io/name=%s", sts.Name))
		if err != nil {
			u.logger.WarnWithContext("failed to get pods for statefulset", map[string]interface{}{
				"statefulset": sts.Name,
				"namespace":   namespace,
				"error":       err.Error(),
			})
			continue
		}
		
		// Count running pods
		runningPods := 0
		for i := range pods {
			if pods[i].Status == "Running" {
				runningPods++
			}
		}
		
		u.logger.InfoWithContext("statefulset pod status validation", map[string]interface{}{
			"statefulset":    sts.Name,
			"namespace":      namespace,
			"total_pods":     len(pods),
			"running_pods":   runningPods,
			"ready_replicas": sts.ReadyReplicas,
			"desired_replicas": sts.Replicas,
		})
		
		// Validate that StatefulSet has expected number of ready replicas
		if sts.ReadyReplicas != sts.Replicas {
			u.logger.WarnWithContext("statefulset does not have all replicas ready", map[string]interface{}{
				"statefulset":      sts.Name,
				"namespace":        namespace,
				"ready_replicas":   sts.ReadyReplicas,
				"desired_replicas": sts.Replicas,
			})
		}
	}
	
	return nil
}

// validateAndFixSecrets validates secret state and automatically resolves conflicts
func (u *DeploymentUsecase) validateAndFixSecrets(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("validating secret state", map[string]interface{}{
		"environment": options.Environment.String(),
	})
	
	// Validate current secret state
	validationResult, err := u.secretUsecase.ValidateSecretState(ctx, options.Environment)
	if err != nil {
		return fmt.Errorf("failed to validate secret state: %w", err)
	}
	
	// Log validation results
	u.logger.InfoWithContext("secret validation results", map[string]interface{}{
		"environment":    options.Environment.String(),
		"conflicts":      len(validationResult.Conflicts),
		"warnings":       len(validationResult.Warnings),
		"valid":          validationResult.Valid,
	})
	
	// Handle warnings
	for _, warning := range validationResult.Warnings {
		u.logger.WarnWithContext("secret validation warning", map[string]interface{}{
			"warning": warning,
		})
	}
	
	// If conflicts exist, attempt to resolve them
	if len(validationResult.Conflicts) > 0 {
		u.logger.InfoWithContext("attempting to resolve secret conflicts", map[string]interface{}{
			"conflict_count": len(validationResult.Conflicts),
		})
		
		for _, conflict := range validationResult.Conflicts {
			u.logger.WarnWithContext("secret conflict detected", map[string]interface{}{
				"secret":           conflict.SecretName,
				"secret_namespace": conflict.SecretNamespace,
				"conflict_type":    conflict.ConflictType.String(),
				"description":      conflict.Description,
			})
		}
		
		// Resolve conflicts automatically (only for specific safe cases)
		if err := u.secretUsecase.ResolveConflicts(ctx, validationResult.Conflicts, options.DryRun); err != nil {
			return fmt.Errorf("failed to resolve secret conflicts: %w", err)
		}
		
		u.logger.InfoWithContext("secret conflicts resolved successfully", map[string]interface{}{
			"resolved_count": len(validationResult.Conflicts),
		})
	}
	
	return nil
}

// deployChartsWithDependencyAwareness deploys charts using dependency analysis
func (u *DeploymentUsecase) deployChartsWithDependencyAwareness(ctx context.Context, options *domain.DeploymentOptions, progress *domain.DeploymentProgress) (*domain.DeploymentProgress, error) {
	u.logger.InfoWithContext("performing dependency analysis", map[string]interface{}{
		"charts_dir": options.ChartsDir,
	})
	
	// Scan dependencies
	dependencyGraph, err := u.dependencyScanner.ScanDependencies(ctx, options.ChartsDir)
	if err != nil {
		u.logger.ErrorWithContext("dependency scanning failed, falling back to traditional deployment", map[string]interface{}{
			"error": err.Error(),
			"fallback_strategy": "traditional_group_based",
		})
		return u.deployChartsTraditional(ctx, options, progress)
	}
	
	u.logger.InfoWithContext("dependency analysis completed", map[string]interface{}{
		"total_charts":       dependencyGraph.Metadata.TotalCharts,
		"total_dependencies": dependencyGraph.Metadata.TotalDependencies,
		"has_cycles":         dependencyGraph.Metadata.HasCycles,
		"deployment_levels":  len(dependencyGraph.DeployOrder),
	})
	
	// Critical check: if no deployment order calculated, fallback immediately
	if len(dependencyGraph.DeployOrder) == 0 {
		u.logger.ErrorWithContext("no deployment order calculated from dependency analysis, falling back to traditional deployment", map[string]interface{}{
			"total_charts": dependencyGraph.Metadata.TotalCharts,
			"fallback_strategy": "traditional_group_based",
			"reason": "empty_deployment_order",
		})
		return u.deployChartsTraditional(ctx, options, progress)
	}
	
	// Handle dependency cycles
	if dependencyGraph.Metadata.HasCycles {
		u.logger.WarnWithContext("dependency cycles detected, proceeding with calculated order", map[string]interface{}{
			"cycles": dependencyGraph.Metadata.Cycles,
			"strategy": "break_cycles_and_continue",
		})
	}
	
	u.logger.InfoWithContext("starting dependency-aware deployment", map[string]interface{}{
		"deployment_strategy": "dependency_aware",
		"levels_to_deploy": len(dependencyGraph.DeployOrder),
	})
	
	// Deploy charts level by level according to dependency graph
	chartConfig := domain.NewChartConfig(options.ChartsDir)
	deployedCharts := 0
	
	for levelIndex, chartNames := range dependencyGraph.DeployOrder {
		u.logger.InfoWithContext("deploying dependency level", map[string]interface{}{
			"level":       levelIndex + 1,
			"chart_count": len(chartNames),
			"charts":      chartNames,
		})
		
		// Convert chart names to Chart objects
		var chartsInLevel []domain.Chart
		for _, chartName := range chartNames {
			if chart, err := chartConfig.GetChart(chartName); err == nil {
				chartsInLevel = append(chartsInLevel, *chart)
			} else {
				u.logger.WarnWithContext("chart not found in configuration, skipping", map[string]interface{}{
					"chart": chartName,
					"error": err.Error(),
					"level": levelIndex + 1,
				})
			}
		}
		
		if len(chartsInLevel) == 0 {
			u.logger.WarnWithContext("no valid charts found in level, skipping", map[string]interface{}{
				"level": levelIndex + 1,
				"original_chart_names": chartNames,
			})
			continue
		}
		
		// Deploy charts in this level
		levelStartTime := time.Now()
		var deploymentErr error
		
		if u.enableParallel && len(chartsInLevel) > 1 {
			// Deploy in parallel if enabled and multiple charts
			deploymentErr = u.deployChartsInParallel(ctx, chartsInLevel, options, progress)
		} else {
			// Deploy sequentially
			deploymentErr = u.deployChartsSequentially(ctx, chartsInLevel, options, progress)
		}
		
		levelDuration := time.Since(levelStartTime)
		deployedCharts += len(chartsInLevel)
		
		u.logger.InfoWithContext("dependency level deployment completed", map[string]interface{}{
			"level": levelIndex + 1,
			"chart_count": len(chartsInLevel),
			"duration": levelDuration,
			"success": deploymentErr == nil,
		})
		
		if deploymentErr != nil {
			u.logger.ErrorWithContext("dependency level deployment failed", map[string]interface{}{
				"level": levelIndex + 1,
				"error": deploymentErr.Error(),
				"deployed_charts_so_far": deployedCharts - len(chartsInLevel),
			})
			
			// Don't fail immediately in dry-run mode
			if !options.DryRun {
				return progress, fmt.Errorf("dependency-aware deployment failed at level %d: %w", levelIndex+1, deploymentErr)
			}
		}
	}
	
	u.logger.InfoWithContext("dependency-aware deployment completed", map[string]interface{}{
		"total_levels_processed": len(dependencyGraph.DeployOrder),
		"charts_processed": deployedCharts,
		"successful_charts": progress.GetSuccessCount(),
		"failed_charts": progress.GetFailedCount(),
		"skipped_charts": progress.GetSkippedCount(),
	})
	
	// Post-deployment validation
	if options.ForceUpdate || options.ShouldOverrideImage() {
		if err := u.validatePodUpdates(ctx, options); err != nil {
			u.logger.WarnWithContext("pod update validation failed", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}
	
	return progress, nil
}

// deployChartsTraditional deploys charts using the traditional group-based approach
func (u *DeploymentUsecase) deployChartsTraditional(ctx context.Context, options *domain.DeploymentOptions, progress *domain.DeploymentProgress) (*domain.DeploymentProgress, error) {
	u.logger.InfoWithContext("starting traditional group-based deployment", map[string]interface{}{
		"deployment_strategy": "traditional_group_based",
		"charts_dir": options.ChartsDir,
	})
	
	chartConfig := domain.NewChartConfig(options.ChartsDir)
	allCharts := chartConfig.AllCharts()
	
	u.logger.InfoWithContext("traditional deployment configuration", map[string]interface{}{
		"total_charts": len(allCharts),
		"infrastructure_charts": len(chartConfig.InfrastructureCharts),
		"application_charts": len(chartConfig.ApplicationCharts),
		"operational_charts": len(chartConfig.OperationalCharts),
	})
	
	var deploymentErrors []error
	
	// Deploy infrastructure charts
	u.logger.InfoWithContext("deploying infrastructure charts", map[string]interface{}{
		"group": "Infrastructure",
		"chart_count": len(chartConfig.InfrastructureCharts),
	})
	
	if err := u.deployChartGroup(ctx, "Infrastructure", chartConfig.InfrastructureCharts, options, progress); err != nil {
		deploymentErrors = append(deploymentErrors, fmt.Errorf("infrastructure chart deployment failed: %w", err))
		u.logger.ErrorWithContext("infrastructure chart group deployment failed", map[string]interface{}{
			"error": err.Error(),
			"continue_with_next_group": !options.DryRun,
		})
		
		// In dry-run mode, continue with other groups to see all potential issues
		if !options.DryRun {
			return progress, deploymentErrors[0]
		}
	} else {
		u.logger.InfoWithContext("infrastructure chart group deployment completed successfully", map[string]interface{}{
			"successful_charts": progress.GetSuccessCount(),
		})
	}
	
	// Deploy application charts
	u.logger.InfoWithContext("deploying application charts", map[string]interface{}{
		"group": "Application",
		"chart_count": len(chartConfig.ApplicationCharts),
	})
	
	if err := u.deployChartGroup(ctx, "Application", chartConfig.ApplicationCharts, options, progress); err != nil {
		deploymentErrors = append(deploymentErrors, fmt.Errorf("application chart deployment failed: %w", err))
		u.logger.ErrorWithContext("application chart group deployment failed", map[string]interface{}{
			"error": err.Error(),
			"continue_with_next_group": !options.DryRun,
		})
		
		if !options.DryRun {
			return progress, deploymentErrors[len(deploymentErrors)-1]
		}
	} else {
		u.logger.InfoWithContext("application chart group deployment completed successfully", map[string]interface{}{
			"successful_charts": progress.GetSuccessCount(),
		})
	}
	
	// Deploy operational charts
	u.logger.InfoWithContext("deploying operational charts", map[string]interface{}{
		"group": "Operational",
		"chart_count": len(chartConfig.OperationalCharts),
	})
	
	if err := u.deployChartGroup(ctx, "Operational", chartConfig.OperationalCharts, options, progress); err != nil {
		deploymentErrors = append(deploymentErrors, fmt.Errorf("operational chart deployment failed: %w", err))
		u.logger.ErrorWithContext("operational chart group deployment failed", map[string]interface{}{
			"error": err.Error(),
		})
		
		if !options.DryRun {
			return progress, deploymentErrors[len(deploymentErrors)-1]
		}
	} else {
		u.logger.InfoWithContext("operational chart group deployment completed successfully", map[string]interface{}{
			"successful_charts": progress.GetSuccessCount(),
		})
	}
	
	// Final assessment
	u.logger.InfoWithContext("traditional deployment completed", map[string]interface{}{
		"total_errors": len(deploymentErrors),
		"successful_charts": progress.GetSuccessCount(),
		"failed_charts": progress.GetFailedCount(),
		"skipped_charts": progress.GetSkippedCount(),
		"dry_run": options.DryRun,
	})
	
	// If there were errors in dry-run mode, log them but don't fail
	if len(deploymentErrors) > 0 && options.DryRun {
		u.logger.WarnWithContext("deployment errors detected in dry-run mode", map[string]interface{}{
			"error_count": len(deploymentErrors),
			"first_error": deploymentErrors[0].Error(),
		})
	}
	
	// Return the first error if any occurred and not in dry-run
	if len(deploymentErrors) > 0 && !options.DryRun {
		return progress, deploymentErrors[0]
	}
	
	return progress, nil
}

// deployChartsInParallel deploys multiple charts in parallel
func (u *DeploymentUsecase) deployChartsInParallel(ctx context.Context, charts []domain.Chart, options *domain.DeploymentOptions, progress *domain.DeploymentProgress) error {
	if u.parallelDeployer == nil {
		u.logger.WarnWithContext("parallel deployer not initialized, falling back to sequential", map[string]interface{}{})
		return u.deployChartsSequentially(ctx, charts, options, progress)
	}
	
	u.logger.InfoWithContext("deploying charts in parallel", map[string]interface{}{
		"chart_count": len(charts),
	})
	
	results, err := u.parallelDeployer.deployChartsParallel(ctx, "dependency-level", charts, options, u.deploySingleChart)
	if err != nil {
		return fmt.Errorf("parallel deployment failed: %w", err)
	}
	
	for _, result := range results {
		progress.AddResult(result)
		if result.Status == domain.DeploymentStatusFailed && !options.DryRun {
			return fmt.Errorf("chart deployment failed: %s", result.Error)
		}
	}
	
	return nil
}

// deployChartsSequentially deploys charts one by one
func (u *DeploymentUsecase) deployChartsSequentially(ctx context.Context, charts []domain.Chart, options *domain.DeploymentOptions, progress *domain.DeploymentProgress) error {
	for _, chart := range charts {
		// Check if context was cancelled
		if ctx.Err() != nil {
			return ctx.Err()
		}
		
		progress.CurrentChart = chart.Name
		progress.CurrentPhase = "Deploying chart"
		
		result := u.deploySingleChart(ctx, chart, options)
		progress.AddResult(result)
		
		if result.Status == domain.DeploymentStatusFailed {
			u.logger.ErrorWithContext("chart deployment failed", map[string]interface{}{
				"chart": chart.Name,
				"error": result.Error,
			})
			
			// Stop on first failure if not dry run
			if !options.DryRun {
				return fmt.Errorf("chart deployment failed: %s", result.Error)
			}
		}
	}
	
	return nil
}

// EnableParallelDeployment enables parallel deployment capabilities
func (u *DeploymentUsecase) EnableParallelDeployment(cacheDir string) error {
	if u.parallelDeployer == nil {
		u.parallelDeployer = NewParallelChartDeployer(u.logger, DefaultParallelConfig())
	}
	
	if u.cache == nil && cacheDir != "" {
		u.cache = NewDeploymentCache(cacheDir, u.logger)
		if err := u.cache.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize cache: %w", err)
		}
		u.enableCache = true
	}
	
	u.enableParallel = true
	
	u.logger.InfoWithContext("parallel deployment enabled", map[string]interface{}{
		"cache_enabled": u.enableCache,
		"cache_dir":     cacheDir,
	})
	
	return nil
}

// SetDependencyAwareDeployment enables or disables dependency-aware deployment
func (u *DeploymentUsecase) SetDependencyAwareDeployment(enabled bool) {
	u.enableDependencyAware = enabled
	u.logger.InfoWithContext("dependency-aware deployment configuration changed", map[string]interface{}{
		"enabled": enabled,
	})
}

// GetDeploymentCapabilities returns the current deployment capabilities
func (u *DeploymentUsecase) GetDeploymentCapabilities() map[string]interface{} {
	return map[string]interface{}{
		"parallel_enabled":        u.enableParallel,
		"cache_enabled":           u.enableCache,
		"dependency_aware_enabled": u.enableDependencyAware,
		"parallel_deployer_ready": u.parallelDeployer != nil,
		"cache_ready":             u.cache != nil,
		"dependency_scanner_ready": u.dependencyScanner != nil,
	}
}

// Helper functions for enhanced logging

// extractChartNames extracts chart names from a slice of Chart structs
func (u *DeploymentUsecase) extractChartNames(charts []domain.Chart) []string {
	names := make([]string, len(charts))
	for i, chart := range charts {
		names[i] = chart.Name
	}
	return names
}

// formatError safely formats an error for logging
func (u *DeploymentUsecase) formatError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// deployChartWithRetry deploys a chart with enhanced retry logic and detailed logging
func (u *DeploymentUsecase) deployChartWithRetry(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions, progress *domain.DeploymentProgress) domain.DeploymentResult {
	const maxRetries = 3
	var lastResult domain.DeploymentResult
	
	retryStartTime := time.Now()
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		attemptStartTime := time.Now()
		
		u.logger.InfoWithContext("attempting chart deployment", map[string]interface{}{
			"chart":       chart.Name,
			"attempt":     attempt,
			"max_retries": maxRetries,
			"namespace":   options.GetNamespace(chart.Name),
			"timeout":     options.Timeout,
		})
		
		result := u.deploySingleChart(ctx, chart, options)
		attemptDuration := time.Since(attemptStartTime)
		
		u.logger.InfoWithContext("chart deployment attempt completed", map[string]interface{}{
			"chart":          chart.Name,
			"attempt":        attempt,
			"status":         result.Status,
			"duration":       attemptDuration,
			"namespace":      result.Namespace,
			"error":          u.formatError(result.Error),
			"will_retry":     result.Status == domain.DeploymentStatusFailed && attempt < maxRetries && u.isRetriableError(result.Error),
		})
		
		// If successful, return immediately
		if result.Status == domain.DeploymentStatusSuccess {
			totalRetryDuration := time.Since(retryStartTime)
			u.logger.InfoWithContext("chart deployment succeeded", map[string]interface{}{
				"chart":                chart.Name,
				"successful_attempt":   attempt,
				"total_retry_duration": totalRetryDuration,
				"namespace":            result.Namespace,
			})
			return result
		}
		
		lastResult = result
		
		// Don't retry if error is not retriable or if it's the last attempt
		if !u.isRetriableError(result.Error) || attempt == maxRetries {
			break
		}
		
		// Calculate retry delay with exponential backoff
		retryDelay := time.Duration(attempt) * 5 * time.Second
		u.logger.WarnWithContext("chart deployment failed, will retry", map[string]interface{}{
			"chart":            chart.Name,
			"failed_attempt":   attempt,
			"error":            result.Error.Error(),
			"retry_delay":      retryDelay,
			"next_attempt":     attempt + 1,
		})
		
		// Wait for retry delay or context cancellation
		select {
		case <-ctx.Done():
			lastResult.Error = fmt.Errorf("deployment cancelled during retry: %w", ctx.Err())
			lastResult.Status = domain.DeploymentStatusFailed
			return lastResult
		case <-time.After(retryDelay):
			// Continue to next attempt
		}
	}
	
	totalRetryDuration := time.Since(retryStartTime)
	u.logger.ErrorWithContext("chart deployment failed after all retries", map[string]interface{}{
		"chart":                chart.Name,
		"total_attempts":       maxRetries,
		"total_retry_duration": totalRetryDuration,
		"final_error":          lastResult.Error.Error(),
		"namespace":            lastResult.Namespace,
	})
	
	return lastResult
}

// isRetriableError determines if an error is worth retrying
func (u *DeploymentUsecase) isRetriableError(err error) bool {
	if err == nil {
		return false
	}
	
	errorMsg := err.Error()
	
	// Check for retriable error patterns
	retriablePatterns := []string{
		"connection refused",
		"timeout",
		"temporary failure",
		"resource temporarily unavailable",
		"another operation in progress",
		"server error",
		"internal error",
		"network",
	}
	
	for _, pattern := range retriablePatterns {
		if contains(errorMsg, pattern) {
			u.logger.DebugWithContext("error classified as retriable", map[string]interface{}{
				"error":   errorMsg,
				"pattern": pattern,
			})
			return true
		}
	}
	
	u.logger.DebugWithContext("error classified as non-retriable", map[string]interface{}{
		"error": errorMsg,
	})
	
	return false
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || 
		    len(s) > len(substr) && 
		    (indexOfInsensitive(s, substr) >= 0))
}

// indexOfInsensitive performs case-insensitive substring search
func indexOfInsensitive(s, substr string) int {
	sLower := toLower(s)
	substrLower := toLower(substr)
	
	for i := 0; i <= len(sLower)-len(substrLower); i++ {
		if sLower[i:i+len(substrLower)] == substrLower {
			return i
		}
	}
	return -1
}

// toLower converts string to lowercase
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + ('a' - 'A')
		} else {
			result[i] = c
		}
	}
	return string(result)
}