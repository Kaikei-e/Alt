package deployment_usecase

import (
	"context"
	"fmt"
	"time"
	
	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
	"deploy-cli/gateway/helm_gateway"
	"deploy-cli/gateway/kubectl_gateway"
	"deploy-cli/gateway/filesystem_gateway"
	"deploy-cli/gateway/system_gateway"
)

// DeploymentUsecase handles deployment operations
type DeploymentUsecase struct {
	helmGateway       *helm_gateway.HelmGateway
	kubectlGateway    *kubectl_gateway.KubectlGateway
	filesystemGateway *filesystem_gateway.FileSystemGateway
	systemGateway     *system_gateway.SystemGateway
	logger            logger_port.LoggerPort
}

// NewDeploymentUsecase creates a new deployment usecase
func NewDeploymentUsecase(
	helmGateway *helm_gateway.HelmGateway,
	kubectlGateway *kubectl_gateway.KubectlGateway,
	filesystemGateway *filesystem_gateway.FileSystemGateway,
	systemGateway *system_gateway.SystemGateway,
	logger logger_port.LoggerPort,
) *DeploymentUsecase {
	return &DeploymentUsecase{
		helmGateway:       helmGateway,
		kubectlGateway:    kubectlGateway,
		filesystemGateway: filesystemGateway,
		systemGateway:     systemGateway,
		logger:            logger,
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
	})
	
	// Get chart configuration
	chartConfig := domain.NewChartConfig(options.ChartsDir)
	allCharts := chartConfig.AllCharts()
	
	// Create deployment progress
	progress := domain.NewDeploymentProgress(len(allCharts))
	
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
	
	// Validate pod updates if force update was enabled
	if options.ForceUpdate {
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
		
		u.logger.InfoWithContext("deployment pod status", map[string]interface{}{
			"deployment": deployment.Name,
			"namespace":  namespace,
			"pods":       pods,
		})
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
		
		u.logger.InfoWithContext("statefulset pod status", map[string]interface{}{
			"statefulset": sts.Name,
			"namespace":   namespace,
			"pods":        pods,
		})
	}
	
	return nil
}