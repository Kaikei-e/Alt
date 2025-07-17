package deployment_usecase

import (
	"context"
	"fmt"
	"time"
	
	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
	"deploy-cli/port/filesystem_port"
	"deploy-cli/port/kubectl_port"
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
	healthChecker       *HealthChecker
	dependencyWaiter    *DependencyWaiter
	strategyFactory     *StrategyFactory
	metricsCollector    *MetricsCollector
	layerMonitor        *LayerHealthMonitor
	dependencyDetector  *DependencyFailureDetector
	progressTracker     *ProgressTracker
	enableParallel      bool
	enableCache         bool
	enableDependencyAware bool
	enableMonitoring    bool
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
	healthChecker := NewHealthChecker(logger)
	dependencyWaiter := NewDependencyWaiter(healthChecker, logger)
	strategyFactory := NewStrategyFactory(logger)
	
	// Initialize monitoring components
	metricsCollector := NewMetricsCollector(logger)
	layerMonitor := NewLayerHealthMonitor(logger, metricsCollector)
	dependencyDetector := NewDependencyFailureDetector(logger, metricsCollector, layerMonitor)
	
	return &DeploymentUsecase{
		helmGateway:           helmGateway,
		kubectlGateway:        kubectlGateway,
		filesystemGateway:     filesystemGateway,
		systemGateway:         systemGateway,
		secretUsecase:         secretUsecase,
		logger:                logger,
		dependencyScanner:     dependencyScanner,
		healthChecker:         healthChecker,
		dependencyWaiter:      dependencyWaiter,
		strategyFactory:       strategyFactory,
		metricsCollector:      metricsCollector,
		layerMonitor:          layerMonitor,
		dependencyDetector:    dependencyDetector,
		progressTracker:       nil, // Will be initialized per deployment
		enableParallel:        false,  // Will be configurable
		enableCache:           false,  // Will be configurable
		enableDependencyAware: true,   // Enable by default
		enableMonitoring:      true,   // Enable monitoring by default
	}
}

// Deploy executes the deployment process
func (u *DeploymentUsecase) Deploy(ctx context.Context, options *domain.DeploymentOptions) (*domain.DeploymentProgress, error) {
	// Setup deployment strategy if not already set
	if err := u.setupDeploymentStrategy(options); err != nil {
		return nil, fmt.Errorf("failed to setup deployment strategy: %w", err)
	}
	
	// Initialize monitoring for this deployment
	deploymentID := fmt.Sprintf("deployment-%d", time.Now().Unix())
	if u.enableMonitoring {
		if err := u.initializeMonitoring(ctx, deploymentID, options); err != nil {
			u.logger.WarnWithContext("failed to initialize monitoring", map[string]interface{}{
				"deployment_id": deploymentID,
				"error": err.Error(),
			})
		}
	}
	
	u.logger.InfoWithContext("starting deployment process", map[string]interface{}{
		"deployment_id": deploymentID,
		"environment": options.Environment.String(),
		"strategy": options.GetStrategyName(),
		"dry_run":     options.DryRun,
		"monitoring_enabled": u.enableMonitoring,
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
	
	// Clean up any stuck Helm operations before deployment
	if err := u.cleanupStuckHelmOperations(ctx, options); err != nil {
		u.logger.WarnWithContext("failed to clean up stuck helm operations", map[string]interface{}{
			"error": err.Error(),
		})
		// Continue with deployment - individual chart deployments will handle their own cleanup
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
	
	// Use layer-aware deployment for correct ordering
	if u.enableDependencyAware {
		return u.deployChartsWithLayerAwareness(ctx, options, progress)
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
		
		// Handle multi-namespace deployment
		if chart.MultiNamespace {
			for _, targetNamespace := range chart.TargetNamespaces {
				chartCopy := chart
				chartCopy.MultiNamespace = false // Disable multi-namespace for individual deployment
				result := u.deploySingleChartToNamespace(ctx, chartCopy, targetNamespace, options)
				progress.AddResult(result)
				
				if result.Status == domain.DeploymentStatusFailed {
					failedCharts = append(failedCharts, fmt.Sprintf("%s-%s", chart.Name, targetNamespace))
					u.logger.ErrorWithContext("chart deployment failed", map[string]interface{}{
						"group":     groupName,
						"chart":     chart.Name,
						"namespace": targetNamespace,
						"error":     result.Error,
					})
					
					// Stop on first failure if not dry run
					if !options.DryRun {
						return fmt.Errorf("chart deployment failed: %s", result.Error)
					}
				}
			}
		} else {
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
	namespace := options.GetNamespace(chart.Name)
	return u.deploySingleChartToNamespace(ctx, chart, namespace, options)
}

// deploySingleChartToNamespace deploys a single chart to a specific namespace
func (u *DeploymentUsecase) deploySingleChartToNamespace(ctx context.Context, chart domain.Chart, namespace string, options *domain.DeploymentOptions) domain.DeploymentResult {
	start := time.Now()
	
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
	
	// Create namespace-specific deployment options
	nsOptions := *options
	nsOptions.TargetNamespace = namespace
	
	// Deploy or template chart with timeout handling
	if options.DryRun {
		_, err = u.helmGateway.TemplateChart(chartCtx, chart, &nsOptions)
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
		err = u.helmGateway.DeployChart(chartCtx, chart, &nsOptions)
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
		"timeout": options.Timeout,
	})
	
	// Check for context cancellation before dependency scanning
	select {
	case <-ctx.Done():
		u.logger.WarnWithContext("deployment cancelled before dependency analysis", map[string]interface{}{
			"error": ctx.Err().Error(),
		})
		return progress, ctx.Err()
	default:
	}
	
	// Scan dependencies with timeout
	depScanCtx, depCancel := context.WithTimeout(ctx, 1*time.Minute)
	dependencyGraph, err := u.dependencyScanner.ScanDependencies(depScanCtx, options.ChartsDir)
	depCancel()
	if err != nil {
		if depScanCtx.Err() == context.DeadlineExceeded {
			u.logger.ErrorWithContext("dependency scanning timed out, falling back to traditional deployment", map[string]interface{}{
				"timeout": "1m",
				"fallback_strategy": "traditional_group_based",
			})
		} else {
			u.logger.ErrorWithContext("dependency scanning failed, falling back to traditional deployment", map[string]interface{}{
				"error": err.Error(),
				"fallback_strategy": "traditional_group_based",
			})
		}
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

// deployChartsWithLayerAwareness deploys charts in predefined layers for correct ordering
func (u *DeploymentUsecase) deployChartsWithLayerAwareness(ctx context.Context, options *domain.DeploymentOptions, progress *domain.DeploymentProgress) (*domain.DeploymentProgress, error) {
	u.logger.InfoWithContext("starting layer-aware deployment", map[string]interface{}{
		"deployment_strategy": "layer_aware",
		"charts_dir": options.ChartsDir,
		"strategy": options.GetStrategyName(),
	})

	// Get layer configurations from the deployment strategy
	var layers []domain.LayerConfiguration
	if options.HasDeploymentStrategy() {
		layers = options.GetLayerConfigurations()
		u.logger.InfoWithContext("using strategy-based layer configurations", map[string]interface{}{
			"strategy": options.GetStrategyName(),
			"layers_count": len(layers),
		})
	} else {
		// Fallback to default configuration
		chartConfig := domain.NewChartConfig(options.ChartsDir)
		layers = u.getDefaultLayerConfigurations(chartConfig, options.ChartsDir)
		u.logger.InfoWithContext("using default layer configurations", map[string]interface{}{
			"layers_count": len(layers),
		})
	}

	// Now use the layer configurations directly
	
	// Get chart configuration for chart validation
	chartConfig := domain.NewChartConfig(options.ChartsDir)

	// Deploy each layer sequentially
	for layerIndex, layer := range layers {
		u.logger.InfoWithContext("deploying layer", map[string]interface{}{
			"layer": layer.Name,
			"layer_index": layerIndex + 1,
			"total_layers": len(layers),
			"chart_count": len(layer.Charts),
			"requires_health_check": layer.RequiresHealthCheck,
			"health_check_timeout": layer.HealthCheckTimeout,
			"layer_completion_timeout": layer.LayerCompletionTimeout,
		})

		// Create layer-specific timeout context
		layerCtx, layerCancel := context.WithTimeout(ctx, layer.LayerCompletionTimeout)
		defer layerCancel()

		// Check for context cancellation
		select {
		case <-layerCtx.Done():
			u.logger.WarnWithContext("deployment cancelled during layer deployment", map[string]interface{}{
				"layer": layer.Name,
				"error": layerCtx.Err().Error(),
			})
			return progress, layerCtx.Err()
		default:
		}

		// Deploy charts in this layer
		layerStartTime := time.Now()
		var layerErr error

		for chartIndex, chart := range layer.Charts {
			// Check if chart directory exists
			if _, err := chartConfig.GetChart(chart.Name); err != nil {
				u.logger.WarnWithContext("chart not found in configuration, skipping", map[string]interface{}{
					"chart": chart.Name,
					"layer": layer.Name,
					"error": err.Error(),
				})
				continue
			}

			// Wait for dependencies before deploying
			if err := u.dependencyWaiter.WaitForDependencies(layerCtx, chart.Name); err != nil {
				u.logger.WarnWithContext("dependency wait failed, continuing with deployment", map[string]interface{}{
					"chart": chart.Name,
					"layer": layer.Name,
					"error": err.Error(),
				})
			}

			// Deploy the chart
			result := u.deploySingleChart(layerCtx, chart, options)
			progress.AddResult(result)

			if result.Status == domain.DeploymentStatusFailed {
				u.logger.ErrorWithContext("chart deployment failed in layer", map[string]interface{}{
					"chart": chart.Name,
					"layer": layer.Name,
					"error": result.Error.Error(),
				})
				
				layerErr = result.Error
				
				// Stop on first failure in layer if not dry run
				if !options.DryRun {
					break
				}
			} else {
				u.logger.InfoWithContext("chart deployed successfully in layer", map[string]interface{}{
					"chart": chart.Name,
					"layer": layer.Name,
					"duration": result.Duration,
				})

				// Wait between charts in the same layer if specified
				if chartIndex < len(layer.Charts)-1 && layer.WaitBetweenCharts > 0 {
					u.logger.InfoWithContext("waiting between charts in layer", map[string]interface{}{
						"chart": chart.Name,
						"layer": layer.Name,
						"wait_duration": layer.WaitBetweenCharts,
					})
					
					select {
					case <-layerCtx.Done():
						u.logger.WarnWithContext("deployment cancelled during inter-chart wait", map[string]interface{}{
							"layer": layer.Name,
							"chart": chart.Name,
							"error": layerCtx.Err().Error(),
						})
						return progress, layerCtx.Err()
					case <-time.After(layer.WaitBetweenCharts):
						// Continue to next chart
					}
				}
			}
		}

		layerDuration := time.Since(layerStartTime)
		
		// If layer requires health check and deployment was successful, perform comprehensive health check
		if layerErr == nil && layer.RequiresHealthCheck && !options.DryRun {
			u.logger.InfoWithContext("performing layer health check", map[string]interface{}{
				"layer": layer.Name,
				"health_check_timeout": layer.HealthCheckTimeout,
			})
			
			healthCheckCtx, healthCheckCancel := context.WithTimeout(layerCtx, layer.HealthCheckTimeout)
			defer healthCheckCancel()
			
			if err := u.performLayerHealthCheck(healthCheckCtx, layer, options); err != nil {
				u.logger.ErrorWithContext("layer health check failed", map[string]interface{}{
					"layer": layer.Name,
					"error": err.Error(),
				})
				layerErr = fmt.Errorf("layer health check failed: %w", err)
			} else {
				u.logger.InfoWithContext("layer health check passed", map[string]interface{}{
					"layer": layer.Name,
				})
			}
		}

		u.logger.InfoWithContext("layer deployment completed", map[string]interface{}{
			"layer": layer.Name,
			"layer_index": layerIndex + 1,
			"duration": layerDuration,
			"success": layerErr == nil,
			"health_check_performed": layer.RequiresHealthCheck && !options.DryRun,
		})

		// If layer failed and not in dry-run mode, stop deployment
		if layerErr != nil && !options.DryRun {
			return progress, fmt.Errorf("layer deployment failed: %s - %w", layer.Name, layerErr)
		}
	}

	u.logger.InfoWithContext("layer-aware deployment completed", map[string]interface{}{
		"deployment_strategy": "layer_aware",
		"total_layers": len(layers),
		"successful_charts": progress.GetSuccessCount(),
		"failed_charts": progress.GetFailedCount(),
		"skipped_charts": progress.GetSkippedCount(),
	})

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
		
		// Check for context cancellation before dependency wait
		select {
		case <-ctx.Done():
			u.logger.WarnWithContext("deployment cancelled before dependency wait", map[string]interface{}{
				"chart": chart.Name,
				"attempt": attempt,
				"error": ctx.Err().Error(),
			})
			lastResult.Error = fmt.Errorf("deployment cancelled: %w", ctx.Err())
			lastResult.Status = domain.DeploymentStatusFailed
			return lastResult
		default:
		}
		
		// Wait for dependencies before deploying the chart (with timeout)
		depWaitCtx, depCancel := context.WithTimeout(ctx, 2*time.Minute)
		if err := u.dependencyWaiter.WaitForDependencies(depWaitCtx, chart.Name); err != nil {
			depCancel()
			if depWaitCtx.Err() == context.DeadlineExceeded {
				u.logger.WarnWithContext("dependency wait timed out, continuing with deployment", map[string]interface{}{
					"chart": chart.Name,
					"attempt": attempt,
					"timeout": "2m",
				})
			} else {
				u.logger.WarnWithContext("dependency wait failed, continuing with deployment", map[string]interface{}{
					"chart": chart.Name,
					"attempt": attempt,
					"error": err.Error(),
				})
			}
			// Continue with deployment even if dependencies aren't ready
			// This allows for graceful degradation
		} else {
			depCancel()
			u.logger.InfoWithContext("dependencies ready for chart", map[string]interface{}{
				"chart": chart.Name,
				"attempt": attempt,
			})
		}
		
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
			u.logger.WarnWithContext("deployment cancelled during retry wait", map[string]interface{}{
				"chart": chart.Name,
				"attempt": attempt,
				"error": ctx.Err().Error(),
			})
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

// performLayerHealthCheck performs comprehensive health checks for a deployment layer
func (u *DeploymentUsecase) performLayerHealthCheck(ctx context.Context, layer domain.LayerConfiguration, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("starting layer health check", map[string]interface{}{
		"layer": layer.Name,
		"chart_count": len(layer.Charts),
		"timeout": layer.HealthCheckTimeout,
	})

	// Check health for each chart in the layer
	for _, chart := range layer.Charts {
		if err := u.performChartHealthCheck(ctx, chart, options); err != nil {
			return fmt.Errorf("chart health check failed for %s: %w", chart.Name, err)
		}
	}

	// Layer-specific health checks
	switch layer.Name {
	case "Storage & Persistent Infrastructure":
		return u.performStorageLayerHealthCheck(ctx, layer.Charts, options)
	case "Core Services":
		return u.performCoreServicesHealthCheck(ctx, layer.Charts, options)
	case "Data Processing Services":
		return u.performProcessingServicesHealthCheck(ctx, layer.Charts, options)
	default:
		// Default health check for other layers
		return u.performDefaultLayerHealthCheck(ctx, layer.Charts, options)
	}
}

// performChartHealthCheck performs health check for a single chart
func (u *DeploymentUsecase) performChartHealthCheck(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	namespace := options.GetNamespace(chart.Name)
	
	u.logger.InfoWithContext("performing chart health check", map[string]interface{}{
		"chart": chart.Name,
		"namespace": namespace,
		"wait_ready": chart.WaitReady,
	})

	// If chart doesn't require readiness check, just verify deployment exists
	if !chart.WaitReady {
		return u.verifyChartDeployment(ctx, chart, namespace)
	}

	// Determine chart type and perform appropriate health check
	if u.isStatefulSetChart(chart.Name) {
		return u.performStatefulSetHealthCheck(ctx, chart.Name, namespace)
	} else {
		return u.performDeploymentHealthCheck(ctx, chart.Name, namespace)
	}
}

// performStorageLayerHealthCheck performs specific health checks for storage layer
func (u *DeploymentUsecase) performStorageLayerHealthCheck(ctx context.Context, charts []domain.Chart, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("performing storage layer health check", map[string]interface{}{
		"chart_count": len(charts),
	})

	// Check all StatefulSets are ready
	for _, chart := range charts {
		namespace := options.GetNamespace(chart.Name)
		
		// Verify StatefulSet is ready
		if err := u.performStatefulSetHealthCheck(ctx, chart.Name, namespace); err != nil {
			return fmt.Errorf("statefulset health check failed for %s: %w", chart.Name, err)
		}
		
		// Verify database connectivity (if applicable)
		if err := u.verifyDatabaseConnectivity(ctx, chart.Name, namespace); err != nil {
			u.logger.WarnWithContext("database connectivity check failed", map[string]interface{}{
				"chart": chart.Name,
				"namespace": namespace,
				"error": err.Error(),
			})
			// Don't fail here, just warn as connectivity might not be immediately available
		}
	}

	return nil
}

// performCoreServicesHealthCheck performs health checks for core services
func (u *DeploymentUsecase) performCoreServicesHealthCheck(ctx context.Context, charts []domain.Chart, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("performing core services health check", map[string]interface{}{
		"chart_count": len(charts),
	})

	// Check all deployments are ready and health endpoints are responding
	for _, chart := range charts {
		namespace := options.GetNamespace(chart.Name)
		
		// Verify deployment is ready
		if err := u.performDeploymentHealthCheck(ctx, chart.Name, namespace); err != nil {
			return fmt.Errorf("deployment health check failed for %s: %w", chart.Name, err)
		}
		
		// Verify service health endpoints (if applicable)
		if err := u.verifyServiceHealthEndpoint(ctx, chart.Name, namespace); err != nil {
			u.logger.WarnWithContext("service health endpoint check failed", map[string]interface{}{
				"chart": chart.Name,
				"namespace": namespace,
				"error": err.Error(),
			})
			// Don't fail here, just warn as health endpoints might not be immediately available
		}
	}

	return nil
}

// performProcessingServicesHealthCheck performs health checks for processing services
func (u *DeploymentUsecase) performProcessingServicesHealthCheck(ctx context.Context, charts []domain.Chart, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("performing processing services health check", map[string]interface{}{
		"chart_count": len(charts),
	})

	// Check all deployments are ready
	for _, chart := range charts {
		namespace := options.GetNamespace(chart.Name)
		
		// Verify deployment is ready
		if err := u.performDeploymentHealthCheck(ctx, chart.Name, namespace); err != nil {
			return fmt.Errorf("deployment health check failed for %s: %w", chart.Name, err)
		}
	}

	return nil
}

// performDefaultLayerHealthCheck performs default health checks for other layers
func (u *DeploymentUsecase) performDefaultLayerHealthCheck(ctx context.Context, charts []domain.Chart, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("performing default layer health check", map[string]interface{}{
		"chart_count": len(charts),
	})

	// Basic readiness check for all charts
	for _, chart := range charts {
		if chart.WaitReady {
			namespace := options.GetNamespace(chart.Name)
			
			if u.isStatefulSetChart(chart.Name) {
				if err := u.performStatefulSetHealthCheck(ctx, chart.Name, namespace); err != nil {
					return fmt.Errorf("statefulset health check failed for %s: %w", chart.Name, err)
				}
			} else {
				if err := u.performDeploymentHealthCheck(ctx, chart.Name, namespace); err != nil {
					return fmt.Errorf("deployment health check failed for %s: %w", chart.Name, err)
				}
			}
		}
	}

	return nil
}

// performStatefulSetHealthCheck performs comprehensive health check for StatefulSets
func (u *DeploymentUsecase) performStatefulSetHealthCheck(ctx context.Context, chartName, namespace string) error {
	u.logger.InfoWithContext("performing statefulset health check", map[string]interface{}{
		"chart": chartName,
		"namespace": namespace,
	})

	// Wait for StatefulSet rollout to complete
	if err := u.kubectlGateway.WaitForRollout(ctx, "statefulset", chartName, namespace, 10*time.Minute); err != nil {
		return fmt.Errorf("statefulset rollout failed: %w", err)
	}

	// Get StatefulSet details
	statefulSets, err := u.kubectlGateway.GetStatefulSets(ctx, namespace)
	if err != nil {
		return fmt.Errorf("failed to get statefulsets: %w", err)
	}

	// Find the specific StatefulSet
	var targetSts *kubectl_port.KubernetesStatefulSet
	for i := range statefulSets {
		if statefulSets[i].Name == chartName {
			targetSts = &statefulSets[i]
			break
		}
	}

	if targetSts == nil {
		return fmt.Errorf("statefulset %s not found in namespace %s", chartName, namespace)
	}

	// Verify readiness
	if targetSts.ReadyReplicas != targetSts.Replicas {
		return fmt.Errorf("statefulset %s not ready: %d/%d replicas ready", chartName, targetSts.ReadyReplicas, targetSts.Replicas)
	}

	// Verify pods are running
	pods, err := u.kubectlGateway.GetPods(ctx, namespace, fmt.Sprintf("app.kubernetes.io/name=%s", chartName))
	if err != nil {
		return fmt.Errorf("failed to get pods for statefulset: %w", err)
	}

	runningPods := 0
	for i := range pods {
		if pods[i].Status == "Running" {
			runningPods++
		}
	}

	if runningPods < targetSts.Replicas {
		return fmt.Errorf("statefulset %s has insufficient running pods: %d/%d", chartName, runningPods, targetSts.Replicas)
	}

	u.logger.InfoWithContext("statefulset health check passed", map[string]interface{}{
		"chart": chartName,
		"namespace": namespace,
		"ready_replicas": targetSts.ReadyReplicas,
		"desired_replicas": targetSts.Replicas,
		"running_pods": runningPods,
	})

	return nil
}

// performDeploymentHealthCheck performs comprehensive health check for Deployments
func (u *DeploymentUsecase) performDeploymentHealthCheck(ctx context.Context, chartName, namespace string) error {
	u.logger.InfoWithContext("performing deployment health check", map[string]interface{}{
		"chart": chartName,
		"namespace": namespace,
	})

	// Wait for deployment rollout to complete
	if err := u.kubectlGateway.WaitForRollout(ctx, "deployment", chartName, namespace, 5*time.Minute); err != nil {
		return fmt.Errorf("deployment rollout failed: %w", err)
	}

	// Get deployment details
	deployments, err := u.kubectlGateway.GetDeployments(ctx, namespace)
	if err != nil {
		return fmt.Errorf("failed to get deployments: %w", err)
	}

	// Find the specific deployment
	var targetDep *kubectl_port.KubernetesDeployment
	for i := range deployments {
		if deployments[i].Name == chartName {
			targetDep = &deployments[i]
			break
		}
	}

	if targetDep == nil {
		return fmt.Errorf("deployment %s not found in namespace %s", chartName, namespace)
	}

	// Verify readiness
	if targetDep.ReadyReplicas != targetDep.Replicas {
		return fmt.Errorf("deployment %s not ready: %d/%d replicas ready", chartName, targetDep.ReadyReplicas, targetDep.Replicas)
	}

	u.logger.InfoWithContext("deployment health check passed", map[string]interface{}{
		"chart": chartName,
		"namespace": namespace,
		"ready_replicas": targetDep.ReadyReplicas,
		"desired_replicas": targetDep.Replicas,
	})

	return nil
}

// verifyChartDeployment verifies that a chart deployment exists
func (u *DeploymentUsecase) verifyChartDeployment(ctx context.Context, chart domain.Chart, namespace string) error {
	u.logger.InfoWithContext("verifying chart deployment", map[string]interface{}{
		"chart": chart.Name,
		"namespace": namespace,
	})

	// Check if deployment or statefulset exists
	if u.isStatefulSetChart(chart.Name) {
		statefulSets, err := u.kubectlGateway.GetStatefulSets(ctx, namespace)
		if err != nil {
			return fmt.Errorf("failed to get statefulsets: %w", err)
		}
		
		for i := range statefulSets {
			if statefulSets[i].Name == chart.Name {
				return nil // Found
			}
		}
		return fmt.Errorf("statefulset %s not found in namespace %s", chart.Name, namespace)
	} else {
		deployments, err := u.kubectlGateway.GetDeployments(ctx, namespace)
		if err != nil {
			return fmt.Errorf("failed to get deployments: %w", err)
		}
		
		for i := range deployments {
			if deployments[i].Name == chart.Name {
				return nil // Found
			}
		}
		return fmt.Errorf("deployment %s not found in namespace %s", chart.Name, namespace)
	}
}

// isStatefulSetChart determines if a chart deploys a StatefulSet
func (u *DeploymentUsecase) isStatefulSetChart(chartName string) bool {
	statefulSetCharts := []string{
		"postgres", "auth-postgres", "kratos-postgres", "clickhouse", "meilisearch",
	}
	
	for _, stsChart := range statefulSetCharts {
		if chartName == stsChart {
			return true
		}
	}
	return false
}

// verifyDatabaseConnectivity attempts to verify database connectivity
func (u *DeploymentUsecase) verifyDatabaseConnectivity(ctx context.Context, chartName, namespace string) error {
	u.logger.InfoWithContext("verifying database connectivity", map[string]interface{}{
		"chart": chartName,
		"namespace": namespace,
	})

	// This is a placeholder - in a real implementation, you would:
	// 1. Get database connection details from secrets
	// 2. Attempt to connect to the database
	// 3. Perform a simple query to verify connectivity
	
	// For now, we'll just log that this check was performed
	u.logger.InfoWithContext("database connectivity check completed", map[string]interface{}{
		"chart": chartName,
		"namespace": namespace,
		"status": "placeholder_implementation",
	})

	return nil
}

// verifyServiceHealthEndpoint attempts to verify service health endpoints
func (u *DeploymentUsecase) verifyServiceHealthEndpoint(ctx context.Context, chartName, namespace string) error {
	u.logger.InfoWithContext("verifying service health endpoint", map[string]interface{}{
		"chart": chartName,
		"namespace": namespace,
	})

	// This is a placeholder - in a real implementation, you would:
	// 1. Get service endpoint details
	// 2. Attempt to call health check endpoints
	// 3. Verify the service is responding correctly
	
	// For now, we'll just log that this check was performed
	u.logger.InfoWithContext("service health endpoint check completed", map[string]interface{}{
		"chart": chartName,
		"namespace": namespace,
		"status": "placeholder_implementation",
	})

	return nil
}

// getDefaultLayerConfigurations returns the default layer configurations for backwards compatibility
func (u *DeploymentUsecase) getDefaultLayerConfigurations(chartConfig *domain.ChartConfig, chartsDir string) []domain.LayerConfiguration {
	return []domain.LayerConfiguration{
		{
			Name: "Storage & Persistent Infrastructure",
			Charts: []domain.Chart{
				{Name: "postgres", Type: domain.InfrastructureChart, Path: chartsDir + "/postgres", WaitReady: true},
				{Name: "auth-postgres", Type: domain.InfrastructureChart, Path: chartsDir + "/auth-postgres", WaitReady: true},
				{Name: "kratos-postgres", Type: domain.InfrastructureChart, Path: chartsDir + "/kratos-postgres", WaitReady: true},
				{Name: "clickhouse", Type: domain.InfrastructureChart, Path: chartsDir + "/clickhouse", WaitReady: true},
				{Name: "meilisearch", Type: domain.InfrastructureChart, Path: chartsDir + "/meilisearch", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      15 * time.Minute,
			WaitBetweenCharts:      30 * time.Second,
			LayerCompletionTimeout: 20 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:          true,
		},
		{
			Name: "Configuration & Secrets",
			Charts: []domain.Chart{
				{Name: "common-secrets", Type: domain.InfrastructureChart, Path: chartsDir + "/common-secrets", WaitReady: false, MultiNamespace: true, TargetNamespaces: []string{"alt-apps", "alt-auth"}},
				{Name: "common-config", Type: domain.InfrastructureChart, Path: chartsDir + "/common-config", WaitReady: false},
				{Name: "common-ssl", Type: domain.InfrastructureChart, Path: chartsDir + "/common-ssl", WaitReady: false, MultiNamespace: true, TargetNamespaces: []string{"alt-apps", "alt-database", "alt-ingress", "alt-search", "alt-auth"}},
			},
			RequiresHealthCheck:     false,
			HealthCheckTimeout:      2 * time.Minute,
			WaitBetweenCharts:      5 * time.Second,
			LayerCompletionTimeout: 5 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:          true,
		},
		{
			Name: "Core Services",
			Charts: []domain.Chart{
				{Name: "alt-backend", Type: domain.ApplicationChart, Path: chartsDir + "/alt-backend", WaitReady: true},
				{Name: "auth-service", Type: domain.ApplicationChart, Path: chartsDir + "/auth-service", WaitReady: true},
				{Name: "kratos", Type: domain.ApplicationChart, Path: chartsDir + "/kratos", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      10 * time.Minute,
			WaitBetweenCharts:      15 * time.Second,
			LayerCompletionTimeout: 15 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:          true,
		},
		{
			Name: "Network & Ingress",
			Charts: []domain.Chart{
				{Name: "nginx", Type: domain.InfrastructureChart, Path: chartsDir + "/nginx", WaitReady: false},
				{Name: "nginx-external", Type: domain.InfrastructureChart, Path: chartsDir + "/nginx-external", WaitReady: false},
			},
			RequiresHealthCheck:     false,
			HealthCheckTimeout:      5 * time.Minute,
			WaitBetweenCharts:      10 * time.Second,
			LayerCompletionTimeout: 8 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:          false,
		},
		{
			Name: "Frontend Applications",
			Charts: []domain.Chart{
				{Name: "alt-frontend", Type: domain.ApplicationChart, Path: chartsDir + "/alt-frontend", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      8 * time.Minute,
			WaitBetweenCharts:      10 * time.Second,
			LayerCompletionTimeout: 10 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:          false,
		},
		{
			Name: "Data Processing Services",
			Charts: []domain.Chart{
				{Name: "pre-processor", Type: domain.ApplicationChart, Path: chartsDir + "/pre-processor", WaitReady: true},
				{Name: "search-indexer", Type: domain.ApplicationChart, Path: chartsDir + "/search-indexer", WaitReady: true},
				{Name: "tag-generator", Type: domain.ApplicationChart, Path: chartsDir + "/tag-generator", WaitReady: true},
				{Name: "news-creator", Type: domain.ApplicationChart, Path: chartsDir + "/news-creator", WaitReady: true},
				{Name: "rask-log-aggregator", Type: domain.ApplicationChart, Path: chartsDir + "/rask-log-aggregator", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      10 * time.Minute,
			WaitBetweenCharts:      20 * time.Second,
			LayerCompletionTimeout: 15 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:          false,
		},
		{
			Name: "Operations & Monitoring",
			Charts: []domain.Chart{
				{Name: "migrate", Type: domain.OperationalChart, Path: chartsDir + "/migrate", WaitReady: true},
				{Name: "backup", Type: domain.OperationalChart, Path: chartsDir + "/backup", WaitReady: true},
				{Name: "monitoring", Type: domain.OperationalChart, Path: chartsDir + "/monitoring", WaitReady: false},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      5 * time.Minute,
			WaitBetweenCharts:      10 * time.Second,
			LayerCompletionTimeout: 10 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:          false,
		},
	}
}

// setupDeploymentStrategy sets up the deployment strategy for the given options
func (u *DeploymentUsecase) setupDeploymentStrategy(options *domain.DeploymentOptions) error {
	// Skip if strategy is already set
	if options.HasDeploymentStrategy() {
		u.logger.InfoWithContext("deployment strategy already set", map[string]interface{}{
			"strategy": options.GetDeploymentStrategy().GetName(),
		})
		return nil
	}
	
	var strategy domain.DeploymentStrategy
	var err error
	
	// Use explicit strategy name if provided
	if options.StrategyName != "" {
		strategy, err = u.strategyFactory.CreateStrategyByName(options.StrategyName)
		if err != nil {
			return fmt.Errorf("failed to create strategy by name '%s': %w", options.StrategyName, err)
		}
		
		// Validate strategy compatibility with environment
		if err := u.strategyFactory.ValidateStrategyForEnvironment(strategy, options.Environment); err != nil {
			return fmt.Errorf("strategy validation failed: %w", err)
		}
	} else {
		// Use environment-based strategy selection
		strategy, err = u.strategyFactory.CreateStrategy(options.Environment)
		if err != nil {
			return fmt.Errorf("failed to create strategy for environment '%s': %w", options.Environment, err)
		}
	}
	
	// Set the strategy
	options.SetDeploymentStrategy(strategy)
	
	u.logger.InfoWithContext("deployment strategy configured", map[string]interface{}{
		"strategy": strategy.GetName(),
		"environment": options.Environment.String(),
		"global_timeout": strategy.GetGlobalTimeout(),
		"allows_parallel": strategy.AllowsParallelDeployment(),
		"health_check_retries": strategy.GetHealthCheckRetries(),
		"zero_downtime": strategy.RequiresZeroDowntime(),
	})
	
	return nil
}

// initializeMonitoring initializes monitoring components for a deployment
func (u *DeploymentUsecase) initializeMonitoring(ctx context.Context, deploymentID string, options *domain.DeploymentOptions) error {
	// Initialize metrics collection
	if err := u.metricsCollector.StartDeploymentMetrics(deploymentID, options); err != nil {
		return fmt.Errorf("failed to start deployment metrics: %w", err)
	}
	
	// Initialize dependency monitoring
	if err := u.dependencyDetector.StartDependencyMonitoring(ctx, deploymentID); err != nil {
		return fmt.Errorf("failed to start dependency monitoring: %w", err)
	}
	
	// Initialize progress tracking
	chartConfig := domain.NewChartConfig(options.ChartsDir)
	allCharts := chartConfig.AllCharts()
	u.progressTracker = NewProgressTracker(u.logger, deploymentID, len(allCharts), u.metricsCollector)
	
	// Set up progress callback for real-time updates
	u.progressTracker.SetProgressCallback(func(progress *domain.DeploymentProgress) {
		u.logger.InfoWithContext("deployment progress update", map[string]interface{}{
			"deployment_id":     deploymentID,
			"current_phase":     progress.CurrentPhase,
			"current_chart":     progress.CurrentChart,
			"completed_charts":  progress.CompletedCharts,
			"total_charts":      progress.TotalCharts,
			"progress_percent":  float64(progress.CompletedCharts) / float64(progress.TotalCharts) * 100.0,
		})
	})
	
	u.logger.InfoWithContext("monitoring initialized", map[string]interface{}{
		"deployment_id": deploymentID,
		"total_charts":  len(allCharts),
	})
	
	return nil
}

// cleanupStuckHelmOperations cleans up any stuck Helm operations before deployment
func (u *DeploymentUsecase) cleanupStuckHelmOperations(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("cleaning up stuck helm operations", map[string]interface{}{
		"environment": options.Environment.String(),
	})
	
	// Get all charts that will be deployed
	chartConfig := domain.NewChartConfig(options.ChartsDir)
	allCharts := chartConfig.AllCharts()
	
	cleanupResults := make(map[string]error)
	
	// Check each chart for stuck operations
	for _, chart := range allCharts {
		// Create a timeout context for each chart cleanup
		cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		
		// Check for pending operations
		operation, err := u.helmGateway.DetectPendingOperation(cleanupCtx, chart, options)
		if err != nil {
			cleanupResults[chart.Name] = fmt.Errorf("failed to detect pending operations: %w", err)
			cancel()
			continue
		}
		
		if operation != nil {
			u.logger.WarnWithContext("detected stuck helm operation", map[string]interface{}{
				"chart": chart.Name,
				"operation": operation.Type,
				"status": operation.Status,
				"start_time": operation.StartTime,
				"namespace": options.GetNamespace(chart.Name),
			})
			
			// Attempt to clean up the stuck operation
			if err := u.helmGateway.CleanupStuckOperations(cleanupCtx, chart, options); err != nil {
				cleanupResults[chart.Name] = fmt.Errorf("failed to cleanup stuck operation: %w", err)
				u.logger.ErrorWithContext("failed to cleanup stuck operation", map[string]interface{}{
					"chart": chart.Name,
					"error": err.Error(),
				})
			} else {
				u.logger.InfoWithContext("successfully cleaned up stuck operation", map[string]interface{}{
					"chart": chart.Name,
					"operation": operation.Type,
					"status": operation.Status,
				})
			}
		}
		
		cancel()
	}
	
	// Report cleanup results
	successCount := 0
	failureCount := 0
	
	for chartName, err := range cleanupResults {
		if err != nil {
			failureCount++
			u.logger.WarnWithContext("chart cleanup failed", map[string]interface{}{
				"chart": chartName,
				"error": err.Error(),
			})
		} else {
			successCount++
		}
	}
	
	u.logger.InfoWithContext("helm operations cleanup completed", map[string]interface{}{
		"total_charts": len(allCharts),
		"charts_with_issues": len(cleanupResults),
		"cleanup_successes": successCount,
		"cleanup_failures": failureCount,
	})
	
	// Return error only if all cleanups failed
	if failureCount > 0 && successCount == 0 {
		return fmt.Errorf("all helm operation cleanups failed")
	}
	
	return nil
}