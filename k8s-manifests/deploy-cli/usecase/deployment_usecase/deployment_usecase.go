package deployment_usecase

import (
	"context"
	"fmt"
	"time"

	"deploy-cli/domain"
	"deploy-cli/gateway/filesystem_gateway"
	"deploy-cli/gateway/helm_gateway"
	"deploy-cli/gateway/kubectl_gateway"
	"deploy-cli/gateway/system_gateway"
	"deploy-cli/port/filesystem_port"
	"deploy-cli/port/logger_port"
	"deploy-cli/usecase/dependency_usecase"
	"deploy-cli/usecase/secret_usecase"
)

// SSL certificate management functionality moved to ssl_management_usecase.go
// GeneratedCertificates moved to port/ssl_manager_port.go

// DeploymentUsecase handles deployment operations
type DeploymentUsecase struct {
	helmGateway                  *helm_gateway.HelmGateway
	kubectlGateway               *kubectl_gateway.KubectlGateway
	filesystemGateway            *filesystem_gateway.FileSystemGateway
	systemGateway                *system_gateway.SystemGateway
	secretUsecase                *secret_usecase.SecretUsecase
	sslUsecase                   *secret_usecase.SSLCertificateUsecase
	logger                       logger_port.LoggerPort
	parallelDeployer             *ParallelChartDeployer
	cache                        *DeploymentCache
	dependencyScanner            *dependency_usecase.DependencyScanner
	healthChecker                *HealthChecker
	dependencyWaiter             *DependencyWaiter
	strategyFactory              *StrategyFactory
	metricsCollector             *MetricsCollector
	layerMonitor                 *LayerHealthMonitor
	dependencyDetector           *DependencyFailureDetector
	progressTracker              *ProgressTracker
	helmOperationManager         *HelmOperationManager
	sslCertificateUsecase        *SSLCertificateUsecase
	secretManagementUsecase      *SecretManagementUsecase
	healthCheckUsecase           *HealthCheckUsecase
	infrastructureSetupUsecase   *InfrastructureSetupUsecase
	statefulSetManagementUsecase *StatefulSetManagementUsecase
	deploymentStrategyUsecase    *DeploymentStrategyUsecase
	sslManagementUsecase         *SSLManagementUsecase
	enableParallel               bool
	enableCache                  bool
	enableDependencyAware        bool
	enableMonitoring             bool
	chartsDir                    string
}

// NewDeploymentUsecase creates a new deployment usecase
func NewDeploymentUsecase(
	helmGateway *helm_gateway.HelmGateway,
	kubectlGateway *kubectl_gateway.KubectlGateway,
	filesystemGateway *filesystem_gateway.FileSystemGateway,
	systemGateway *system_gateway.SystemGateway,
	secretUsecase *secret_usecase.SecretUsecase,
	sslUsecase *secret_usecase.SSLCertificateUsecase,
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

	// Initialize helm operation manager
	helmOperationManager := NewHelmOperationManager(logger)

	// Initialize SSL certificate usecase
	sslCertificateUsecase := NewSSLCertificateUsecase(logger, secretUsecase, sslUsecase)

	// Initialize secret management usecase
	secretManagementUsecase := NewSecretManagementUsecase(kubectlGateway, secretUsecase, logger)

	// Initialize health check usecase
	healthCheckUsecase := NewHealthCheckUsecase(logger, healthChecker)

	// Initialize infrastructure setup usecase
	infrastructureSetupUsecase := NewInfrastructureSetupUsecase(kubectlGateway, systemGateway, logger, strategyFactory)

	// Initialize StatefulSet management usecase
	statefulSetManagementUsecase := NewStatefulSetManagementUsecase(systemGateway, logger)

	// Initialize deployment strategy usecase
	deploymentStrategyUsecase := NewDeploymentStrategyUsecase(strategyFactory, logger)

	// Initialize SSL management usecase
	sslManagementUsecase := NewSSLManagementUsecase(secretUsecase, sslUsecase, logger)

	return &DeploymentUsecase{
		helmGateway:                  helmGateway,
		kubectlGateway:               kubectlGateway,
		filesystemGateway:            filesystemGateway,
		systemGateway:                systemGateway,
		secretUsecase:                secretUsecase,
		sslUsecase:                   sslUsecase,
		logger:                       logger,
		dependencyScanner:            dependencyScanner,
		healthChecker:                healthChecker,
		dependencyWaiter:             dependencyWaiter,
		strategyFactory:              strategyFactory,
		metricsCollector:             metricsCollector,
		layerMonitor:                 layerMonitor,
		dependencyDetector:           dependencyDetector,
		progressTracker:              nil, // Will be initialized per deployment
		helmOperationManager:         helmOperationManager,
		sslCertificateUsecase:        sslCertificateUsecase,
		secretManagementUsecase:      secretManagementUsecase,
		healthCheckUsecase:           healthCheckUsecase,
		infrastructureSetupUsecase:   infrastructureSetupUsecase,
		statefulSetManagementUsecase: statefulSetManagementUsecase,
		deploymentStrategyUsecase:    deploymentStrategyUsecase,
		sslManagementUsecase:         sslManagementUsecase,
		enableParallel:               false, // Will be configurable
		enableCache:                  false, // Will be configurable
		enableDependencyAware:        true,  // Enable by default
		enableMonitoring:             true,  // Enable monitoring by default
	}
}

// Deploy executes the deployment process
func (u *DeploymentUsecase) Deploy(ctx context.Context, options *domain.DeploymentOptions) (*domain.DeploymentProgress, error) {
	// Setup deployment strategy if not already set
	if err := u.deploymentStrategyUsecase.setupDeploymentStrategy(options); err != nil {
		return nil, fmt.Errorf("failed to setup deployment strategy: %w", err)
	}

	// Initialize monitoring for this deployment
	deploymentID := fmt.Sprintf("deployment-%d", time.Now().Unix())
	if u.enableMonitoring {
		if err := u.initializeMonitoring(ctx, deploymentID, options); err != nil {
			u.logger.WarnWithContext("failed to initialize monitoring", map[string]interface{}{
				"deployment_id": deploymentID,
				"error":         err.Error(),
			})
		}
	}

	u.logger.InfoWithContext("starting deployment process", map[string]interface{}{
		"deployment_id":      deploymentID,
		"environment":        options.Environment.String(),
		"strategy":           options.GetStrategyName(),
		"dry_run":            options.DryRun,
		"monitoring_enabled": u.enableMonitoring,
	})

	// Step 1: Pre-deployment validation
	if err := u.infrastructureSetupUsecase.preDeploymentValidation(ctx, options); err != nil {
		return nil, fmt.Errorf("pre-deployment validation failed: %w", err)
	}

	// Step 1.4: Ensure namespaces exist (moved from Step 3 for SSL certificate validation)
	if err := u.infrastructureSetupUsecase.ensureNamespaces(ctx, options); err != nil {
		return nil, fmt.Errorf("namespace setup failed: %w", err)
	}

	// Step 1.5: SSL certificate validation and auto-generation (after namespace creation)
	if err := u.sslCertificateUsecase.PreDeploymentSSLCheck(ctx, options); err != nil {
		return nil, fmt.Errorf("SSL certificate validation failed: %w", err)
	}

	// Step 1.6: Pre-deployment secret validation
	charts := u.deploymentStrategyUsecase.getAllCharts(options)
	if err := u.secretManagementUsecase.ValidateSecretsBeforeDeployment(ctx, charts); err != nil {
		return nil, fmt.Errorf("secret validation failed: %w", err)
	}

	// Step 1.7: Comprehensive secret provisioning
	if err := u.secretManagementUsecase.provisionAllRequiredSecrets(ctx, charts); err != nil {
		return nil, fmt.Errorf("secret provisioning failed: %w", err)
	}

	// Step 1.8: SSL Certificate Management (NEW!)
	if err := u.sslManagementUsecase.ManageCertificateLifecycle(ctx, options.Environment, options.ChartsDir); err != nil {
		return nil, fmt.Errorf("SSL certificate management failed: %w", err)
	}

	// Step 1.9: StatefulSet Recovery Preparation (NEW!)
	if options.SkipStatefulSetRecovery {
		u.logger.InfoWithContext("skipping StatefulSet recovery (emergency deployment mode)", map[string]interface{}{
			"environment": options.Environment.String(),
			"reason":      "skip_option_enabled",
		})
	} else {
		if err := u.statefulSetManagementUsecase.prepareStatefulSetRecovery(ctx, options); err != nil {
			return nil, fmt.Errorf("StatefulSet recovery preparation failed: %w\n\nTroubleshooting:\n- For emergency deployments, use --skip-statefulset-recovery flag\n- Check if kubectl is properly configured and accessible\n- Verify that the specified namespaces exist", err)
		}
	}

	// Step 2: Setup storage infrastructure
	if err := u.infrastructureSetupUsecase.setupStorageInfrastructure(ctx, options); err != nil {
		return nil, fmt.Errorf("storage infrastructure setup failed: %w", err)
	}

	// Step 3: Deploy charts (namespaces already created in Step 1.4)
	progress, err := u.deployCharts(ctx, options)
	if err != nil {
		return progress, fmt.Errorf("chart deployment failed: %w", err)
	}

	// Step 4: Post-deployment operations
	if err := u.infrastructureSetupUsecase.postDeploymentOperations(ctx, options); err != nil {
		return progress, fmt.Errorf("post-deployment operations failed: %w", err)
	}

	u.logger.InfoWithContext("deployment process completed successfully", map[string]interface{}{
		"environment":       options.Environment.String(),
		"successful_charts": progress.GetSuccessCount(),
		"failed_charts":     progress.GetFailedCount(),
		"skipped_charts":    progress.GetSkippedCount(),
	})

	return progress, nil
}

// deployCharts deploys all charts in the correct order
func (u *DeploymentUsecase) deployCharts(ctx context.Context, options *domain.DeploymentOptions) (*domain.DeploymentProgress, error) {
	u.logger.InfoWithContext("deploying charts", map[string]interface{}{
		"environment":      options.Environment.String(),
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
		if err := u.healthCheckUsecase.validatePodUpdates(ctx, options); err != nil {
			u.logger.WarnWithContext("pod update validation failed", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}

	u.logger.InfoWithContext("charts deployment completed", map[string]interface{}{
		"environment":       options.Environment.String(),
		"successful_charts": progress.GetSuccessCount(),
		"failed_charts":     progress.GetFailedCount(),
		"skipped_charts":    progress.GetSkippedCount(),
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
		"group":         groupName,
		"chart_count":   len(charts),
		"failed_count":  len(failedCharts),
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

	// Use longer timeout for StatefulSet database charts that need time to initialize
	// This overrides the command line timeout for database charts
	if chart.Name == "postgres" || chart.Name == "auth-postgres" || chart.Name == "kratos-postgres" || chart.Name == "clickhouse" || chart.Name == "meilisearch" {
		chartTimeout = 10 * time.Minute // Extended timeout for database initialization
		u.logger.InfoWithContext("using extended timeout for database chart", map[string]interface{}{
			"chart":                           chart.Name,
			"timeout":                         chartTimeout,
			"overriding_command_line_timeout": options.Timeout,
		})
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
		chartNamespace := u.getNamespaceForChart(chart)
		err = u.helmOperationManager.ExecuteWithLock(chart.Name, chartNamespace, "deploy", func() error {
			return u.helmGateway.DeployChart(chartCtx, chart, &nsOptions)
		})
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
		"chart":       chart.Name,
		"namespace":   namespace,
		"status":      result.Status,
		"duration":    result.Duration,
		"values_file": valuesFile,
		"timeout":     chartTimeout,
	})

	return result
}

// getAllCharts gets all charts that will be deployed based on deployment options (delegated to DeploymentStrategyUsecase)
func (u *DeploymentUsecase) getAllCharts(options *domain.DeploymentOptions) []domain.Chart {
	return u.deploymentStrategyUsecase.getAllCharts(options)
}

// getNamespaceForChart returns the appropriate namespace for a chart
func (u *DeploymentUsecase) getNamespaceForChart(chart domain.Chart) string {
	// For multi-namespace charts, return the primary namespace
	if chart.MultiNamespace && len(chart.TargetNamespaces) > 0 {
		return chart.TargetNamespaces[0]
	}

	// Use domain layer to determine namespace consistently
	return domain.DetermineNamespace(chart.Name, domain.Production)
}

// getEnvironmentFromNamespace determines the environment based on namespace
func (u *DeploymentUsecase) getEnvironmentFromNamespace(namespace string) domain.Environment {
	switch namespace {
	case "alt-production", "alt-apps", "alt-auth", "alt-database", "alt-ingress", "alt-search":
		return domain.Production
	case "alt-staging":
		return domain.Staging
	case "alt-dev":
		return domain.Development
	default:
		// Default to development for unknown namespaces
		return domain.Development
	}
}

// DeployWithRollback deploys charts with automatic rollback on failure
func (u *DeploymentUsecase) DeployWithRollback(ctx context.Context, options *domain.DeploymentOptions) (*domain.DeploymentProgress, error) {
	u.logger.InfoWithContext("starting deployment with rollback capability", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// Create deployment checkpoint
	checkpoint, err := u.createDeploymentCheckpoint(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment checkpoint: %w", err)
	}

	u.logger.InfoWithContext("deployment checkpoint created", map[string]interface{}{
		"checkpoint_id": checkpoint.ID,
		"timestamp":     checkpoint.Timestamp,
	})

	// Attempt deployment
	result, err := u.Deploy(ctx, options)
	if err != nil {
		u.logger.ErrorWithContext("deployment failed, initiating rollback", map[string]interface{}{
			"error":         err.Error(),
			"checkpoint_id": checkpoint.ID,
		})

		// Attempt rollback
		rollbackErr := u.rollbackToCheckpoint(ctx, checkpoint, options)
		if rollbackErr != nil {
			u.logger.ErrorWithContext("rollback failed", map[string]interface{}{
				"deploy_error":   err.Error(),
				"rollback_error": rollbackErr.Error(),
				"checkpoint_id":  checkpoint.ID,
			})
			return nil, fmt.Errorf("deployment failed and rollback failed: deploy=%w, rollback=%w", err, rollbackErr)
		}

		u.logger.InfoWithContext("rollback completed successfully", map[string]interface{}{
			"checkpoint_id": checkpoint.ID,
		})
		return nil, fmt.Errorf("deployment failed, rolled back to checkpoint %s: %w", checkpoint.ID, err)
	}

	u.logger.InfoWithContext("deployment completed successfully", map[string]interface{}{
		"checkpoint_id": checkpoint.ID,
	})

	return result, nil
}

// DeployWithRetry deploys a chart with retry logic and cleanup between attempts
func (u *DeploymentUsecase) DeployWithRetry(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions, maxRetries int) error {
	u.logger.InfoWithContext("starting chart deployment with retry", map[string]interface{}{
		"chart":       chart.Name,
		"max_retries": maxRetries,
	})

	for attempt := 1; attempt <= maxRetries; attempt++ {
		u.logger.InfoWithContext("attempting chart deployment", map[string]interface{}{
			"chart":       chart.Name,
			"attempt":     attempt,
			"max_retries": maxRetries,
		})

		err := u.deployChart(ctx, chart, options)
		if err == nil {
			u.logger.InfoWithContext("chart deployment successful", map[string]interface{}{
				"chart":   chart.Name,
				"attempt": attempt,
			})
			return nil
		}

		u.logger.WarnWithContext("chart deployment failed", map[string]interface{}{
			"chart":   chart.Name,
			"attempt": attempt,
			"error":   err.Error(),
		})

		// Cleanup failed deployment before next attempt
		if attempt < maxRetries {
			cleanupErr := u.cleanupFailedDeployment(ctx, chart, options)
			if cleanupErr != nil {
				u.logger.WarnWithContext("cleanup failed", map[string]interface{}{
					"chart":         chart.Name,
					"attempt":       attempt,
					"cleanup_error": cleanupErr.Error(),
				})
			} else {
				u.logger.InfoWithContext("cleanup completed", map[string]interface{}{
					"chart":   chart.Name,
					"attempt": attempt,
				})
			}

			// Exponential backoff
			backoffDuration := time.Duration(attempt) * 10 * time.Second
			u.logger.InfoWithContext("waiting before retry", map[string]interface{}{
				"chart":            chart.Name,
				"attempt":          attempt,
				"backoff_duration": backoffDuration.String(),
			})
			time.Sleep(backoffDuration)
		}
	}

	return fmt.Errorf("chart deployment failed after %d attempts: %s", maxRetries, chart.Name)
}

// createDeploymentCheckpoint creates a checkpoint of the current deployment state
func (u *DeploymentUsecase) createDeploymentCheckpoint(ctx context.Context, options *domain.DeploymentOptions) (*domain.DeploymentCheckpoint, error) {
	checkpointID := fmt.Sprintf("checkpoint-%d", time.Now().Unix())

	u.logger.InfoWithContext("creating deployment checkpoint", map[string]interface{}{
		"checkpoint_id": checkpointID,
		"environment":   options.Environment.String(),
	})

	// Get current Helm releases
	namespaces := domain.GetNamespacesForEnvironment(options.Environment)
	var releases []domain.HelmReleaseInfo

	for _, namespace := range namespaces {
		nsReleases, err := u.helmGateway.ListReleases(ctx, namespace)
		if err != nil {
			u.logger.WarnWithContext("failed to list releases for checkpoint", map[string]interface{}{
				"namespace": namespace,
				"error":     err.Error(),
			})
			continue
		}
		releases = append(releases, nsReleases...)
	}

	checkpoint := &domain.DeploymentCheckpoint{
		ID:          checkpointID,
		Timestamp:   time.Now(),
		Environment: options.Environment,
		Releases:    releases,
		Namespaces:  namespaces,
	}

	u.logger.InfoWithContext("deployment checkpoint created", map[string]interface{}{
		"checkpoint_id":    checkpointID,
		"releases_count":   len(releases),
		"namespaces_count": len(namespaces),
	})

	return checkpoint, nil
}

// rollbackToCheckpoint rolls back deployment to a previous checkpoint
func (u *DeploymentUsecase) rollbackToCheckpoint(ctx context.Context, checkpoint *domain.DeploymentCheckpoint, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("starting rollback to checkpoint", map[string]interface{}{
		"checkpoint_id":        checkpoint.ID,
		"checkpoint_timestamp": checkpoint.Timestamp,
		"environment":          options.Environment.String(),
	})

	// Get current releases
	var currentReleases []domain.HelmReleaseInfo
	for _, namespace := range checkpoint.Namespaces {
		nsReleases, err := u.helmGateway.ListReleases(ctx, namespace)
		if err != nil {
			u.logger.WarnWithContext("failed to list current releases for rollback", map[string]interface{}{
				"namespace": namespace,
				"error":     err.Error(),
			})
			continue
		}
		currentReleases = append(currentReleases, nsReleases...)
	}

	// Identify releases to rollback or remove
	checkpointReleaseMap := make(map[string]domain.HelmReleaseInfo)
	for _, release := range checkpoint.Releases {
		key := fmt.Sprintf("%s/%s", release.Namespace, release.Name)
		checkpointReleaseMap[key] = release
	}

	// Process each current release
	for _, currentRelease := range currentReleases {
		key := fmt.Sprintf("%s/%s", currentRelease.Namespace, currentRelease.Name)
		checkpointRelease, existedInCheckpoint := checkpointReleaseMap[key]

		if existedInCheckpoint {
			// Rollback to previous revision if different
			if currentRelease.Revision != checkpointRelease.Revision {
				u.logger.InfoWithContext("rolling back release", map[string]interface{}{
					"release":          currentRelease.Name,
					"namespace":        currentRelease.Namespace,
					"current_revision": currentRelease.Revision,
					"target_revision":  checkpointRelease.Revision,
				})

				err := u.helmGateway.RollbackRelease(ctx, currentRelease.Name, currentRelease.Namespace, checkpointRelease.Revision)
				if err != nil {
					return fmt.Errorf("failed to rollback release %s in namespace %s: %w", currentRelease.Name, currentRelease.Namespace, err)
				}
			}
		} else {
			// Release didn't exist in checkpoint, uninstall it
			u.logger.InfoWithContext("uninstalling new release", map[string]interface{}{
				"release":   currentRelease.Name,
				"namespace": currentRelease.Namespace,
			})

			err := u.helmGateway.UninstallRelease(ctx, currentRelease.Name, currentRelease.Namespace)
			if err != nil {
				return fmt.Errorf("failed to uninstall release %s in namespace %s: %w", currentRelease.Name, currentRelease.Namespace, err)
			}
		}
	}

	u.logger.InfoWithContext("rollback to checkpoint completed", map[string]interface{}{
		"checkpoint_id": checkpoint.ID,
	})

	return nil
}

// cleanupFailedDeployment cleans up a failed chart deployment
func (u *DeploymentUsecase) cleanupFailedDeployment(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("starting cleanup of failed deployment", map[string]interface{}{
		"chart": chart.Name,
	})

	namespace := u.getNamespaceForChart(chart)

	// Check if release exists
	releases, err := u.helmGateway.ListReleases(ctx, namespace)
	if err != nil {
		return fmt.Errorf("failed to list releases for cleanup: %w", err)
	}

	var releaseToCleanup *domain.HelmReleaseInfo
	for _, release := range releases {
		if release.Name == chart.Name {
			releaseToCleanup = &release
			break
		}
	}

	if releaseToCleanup == nil {
		u.logger.InfoWithContext("no release found to cleanup", map[string]interface{}{
			"chart":     chart.Name,
			"namespace": namespace,
		})
		return nil
	}

	// Check release status
	if releaseToCleanup.Status == "failed" || releaseToCleanup.Status == "pending-install" || releaseToCleanup.Status == "pending-upgrade" {
		u.logger.InfoWithContext("uninstalling failed release", map[string]interface{}{
			"release":   releaseToCleanup.Name,
			"namespace": namespace,
			"status":    releaseToCleanup.Status,
		})

		err := u.helmGateway.UninstallRelease(ctx, releaseToCleanup.Name, namespace)
		if err != nil {
			return fmt.Errorf("failed to uninstall failed release %s: %w", releaseToCleanup.Name, err)
		}

		u.logger.InfoWithContext("failed release uninstalled", map[string]interface{}{
			"release":   releaseToCleanup.Name,
			"namespace": namespace,
		})
	}

	return nil
}

// deployChart deploys a single chart
func (u *DeploymentUsecase) deployChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("deploying individual chart", map[string]interface{}{
		"chart": chart.Name,
		"type":  string(chart.Type),
	})

	namespace := u.getNamespaceForChart(chart)

	// Create namespace if it doesn't exist
	if err := u.kubectlGateway.EnsureNamespace(ctx, namespace); err != nil {
		return fmt.Errorf("failed to create namespace %s: %w", namespace, err)
	}

	// Deploy chart using Helm with enhanced error recovery (Phase 4.3) and concurrent operation prevention
	err := u.helmOperationManager.ExecuteWithLock(chart.Name, namespace, "deploy", func() error {
		return u.helmGateway.DeployChart(ctx, chart, options)
	})
	if err != nil {
		// Enhanced error recovery: try to handle secret ownership errors (only if flag is enabled)
		if options.AutoFixSecrets {
			u.logger.WarnWithContext("deployment failed, attempting secret error recovery", map[string]interface{}{
				"chart": chart.Name,
				"error": err.Error(),
			})

			// Use the secret management usecase to handle the error
			if recoveryErr := u.secretManagementUsecase.handleSecretOwnershipError(err, chart.Name); recoveryErr != nil {
				return fmt.Errorf("failed to deploy chart %s: %w", chart.Name, recoveryErr)
			}

			// If error recovery succeeded, try deployment again
			u.logger.InfoWithContext("retrying chart deployment after error recovery", map[string]interface{}{
				"chart":          chart.Name,
				"original_error": err.Error(),
			})

			if retryErr := u.helmOperationManager.ExecuteWithLock(chart.Name, namespace, "deploy-retry", func() error {
				return u.helmGateway.DeployChart(ctx, chart, options)
			}); retryErr != nil {
				return fmt.Errorf("failed to deploy chart %s after error recovery: %w", chart.Name, retryErr)
			}

			u.logger.InfoWithContext("chart deployment succeeded after error recovery", map[string]interface{}{
				"chart": chart.Name,
			})
		} else {
			// No auto-fix enabled, return original error
			return fmt.Errorf("failed to deploy chart %s: %w", chart.Name, err)
		}
	}

	// Wait for readiness if required
	if chart.ShouldWaitForReadinessWithOptions(options) {
		u.logger.InfoWithContext("waiting for chart readiness", map[string]interface{}{
			"chart":     chart.Name,
			"namespace": namespace,
		})

		// Use appropriate health check based on chart type
		var err error
		switch chart.Name {
		case "postgres", "auth-postgres", "kratos-postgres":
			err = u.healthChecker.WaitForPostgreSQLReady(ctx, namespace, chart.Name)
		case "meilisearch":
			err = u.healthChecker.WaitForMeilisearchReady(ctx, namespace, chart.Name)
		default:
			err = u.healthChecker.WaitForServiceReady(ctx, chart.Name, string(chart.Type), namespace)
		}

		if err != nil {
			return fmt.Errorf("chart %s readiness check failed: %w", chart.Name, err)
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
		"environment": options.Environment.String(),
		"conflicts":   len(validationResult.Conflicts),
		"warnings":    len(validationResult.Warnings),
		"valid":       validationResult.Valid,
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
		"timeout":    options.Timeout,
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
				"timeout":           "1m",
				"fallback_strategy": "traditional_group_based",
			})
		} else {
			u.logger.ErrorWithContext("dependency scanning failed, falling back to traditional deployment", map[string]interface{}{
				"error":             err.Error(),
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
			"total_charts":      dependencyGraph.Metadata.TotalCharts,
			"fallback_strategy": "traditional_group_based",
			"reason":            "empty_deployment_order",
		})
		return u.deployChartsTraditional(ctx, options, progress)
	}

	// Handle dependency cycles
	if dependencyGraph.Metadata.HasCycles {
		u.logger.WarnWithContext("dependency cycles detected, proceeding with calculated order", map[string]interface{}{
			"cycles":   dependencyGraph.Metadata.Cycles,
			"strategy": "break_cycles_and_continue",
		})
	}

	u.logger.InfoWithContext("starting dependency-aware deployment", map[string]interface{}{
		"deployment_strategy": "dependency_aware",
		"levels_to_deploy":    len(dependencyGraph.DeployOrder),
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
				"level":                levelIndex + 1,
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
			"level":       levelIndex + 1,
			"chart_count": len(chartsInLevel),
			"duration":    levelDuration,
			"success":     deploymentErr == nil,
		})

		if deploymentErr != nil {
			u.logger.ErrorWithContext("dependency level deployment failed", map[string]interface{}{
				"level":                  levelIndex + 1,
				"error":                  deploymentErr.Error(),
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
		"charts_processed":       deployedCharts,
		"successful_charts":      progress.GetSuccessCount(),
		"failed_charts":          progress.GetFailedCount(),
		"skipped_charts":         progress.GetSkippedCount(),
	})

	// Post-deployment validation
	if options.ForceUpdate || options.ShouldOverrideImage() {
		if err := u.healthCheckUsecase.validatePodUpdates(ctx, options); err != nil {
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
		"charts_dir":          options.ChartsDir,
	})

	chartConfig := domain.NewChartConfig(options.ChartsDir)
	allCharts := chartConfig.AllCharts()

	u.logger.InfoWithContext("traditional deployment configuration", map[string]interface{}{
		"total_charts":          len(allCharts),
		"infrastructure_charts": len(chartConfig.InfrastructureCharts),
		"application_charts":    len(chartConfig.ApplicationCharts),
		"operational_charts":    len(chartConfig.OperationalCharts),
	})

	var deploymentErrors []error

	// Deploy infrastructure charts
	u.logger.InfoWithContext("deploying infrastructure charts", map[string]interface{}{
		"group":       "Infrastructure",
		"chart_count": len(chartConfig.InfrastructureCharts),
	})

	if err := u.deployChartGroup(ctx, "Infrastructure", chartConfig.InfrastructureCharts, options, progress); err != nil {
		deploymentErrors = append(deploymentErrors, fmt.Errorf("infrastructure chart deployment failed: %w", err))
		u.logger.ErrorWithContext("infrastructure chart group deployment failed", map[string]interface{}{
			"error":                    err.Error(),
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
		"group":       "Application",
		"chart_count": len(chartConfig.ApplicationCharts),
	})

	if err := u.deployChartGroup(ctx, "Application", chartConfig.ApplicationCharts, options, progress); err != nil {
		deploymentErrors = append(deploymentErrors, fmt.Errorf("application chart deployment failed: %w", err))
		u.logger.ErrorWithContext("application chart group deployment failed", map[string]interface{}{
			"error":                    err.Error(),
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
		"group":       "Operational",
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
		"total_errors":      len(deploymentErrors),
		"successful_charts": progress.GetSuccessCount(),
		"failed_charts":     progress.GetFailedCount(),
		"skipped_charts":    progress.GetSkippedCount(),
		"dry_run":           options.DryRun,
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
		"charts_dir":          options.ChartsDir,
		"strategy":            options.GetStrategyName(),
	})

	// Get layer configurations from the deployment strategy
	var layers []domain.LayerConfiguration
	if options.HasDeploymentStrategy() {
		layers = options.GetLayerConfigurations()
		u.logger.InfoWithContext("using strategy-based layer configurations", map[string]interface{}{
			"strategy":     options.GetStrategyName(),
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
			"layer":                    layer.Name,
			"layer_index":              layerIndex + 1,
			"total_layers":             len(layers),
			"chart_count":              len(layer.Charts),
			"requires_health_check":    layer.RequiresHealthCheck,
			"health_check_timeout":     layer.HealthCheckTimeout,
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

			// Deploy the chart - handle multi-namespace deployment
			if chart.MultiNamespace {
				// Deploy to multiple namespaces
				u.logger.InfoWithContext("deploying multi-namespace chart", map[string]interface{}{
					"chart":             chart.Name,
					"layer":             layer.Name,
					"target_namespaces": chart.TargetNamespaces,
				})

				for _, targetNamespace := range chart.TargetNamespaces {
					chartCopy := chart
					chartCopy.MultiNamespace = false // Disable multi-namespace for individual deployment
					result := u.deploySingleChartToNamespace(layerCtx, chartCopy, targetNamespace, options)
					progress.AddResult(result)

					if result.Status == domain.DeploymentStatusFailed {
						u.logger.ErrorWithContext("multi-namespace chart deployment failed", map[string]interface{}{
							"chart":     chart.Name,
							"layer":     layer.Name,
							"namespace": targetNamespace,
							"error":     result.Error.Error(),
						})

						layerErr = result.Error

						// Stop on first failure if not dry run
						if !options.DryRun {
							break
						}
					} else {
						u.logger.InfoWithContext("multi-namespace chart deployed successfully", map[string]interface{}{
							"chart":     chart.Name,
							"layer":     layer.Name,
							"namespace": targetNamespace,
							"duration":  result.Duration,
						})
					}
				}
			} else {
				// Deploy to single namespace
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
						"chart":    chart.Name,
						"layer":    layer.Name,
						"duration": result.Duration,
					})
				}
			}

			// Wait between charts in the same layer if specified
			if chartIndex < len(layer.Charts)-1 && layer.WaitBetweenCharts > 0 {
				u.logger.InfoWithContext("waiting between charts in layer", map[string]interface{}{
					"chart":         chart.Name,
					"layer":         layer.Name,
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

		layerDuration := time.Since(layerStartTime)

		// If layer requires health check and deployment was successful, perform comprehensive health check
		if layerErr == nil && layer.RequiresHealthCheck && !options.DryRun {
			u.logger.InfoWithContext("performing layer health check", map[string]interface{}{
				"layer":                layer.Name,
				"health_check_timeout": layer.HealthCheckTimeout,
			})

			healthCheckCtx, healthCheckCancel := context.WithTimeout(layerCtx, layer.HealthCheckTimeout)
			defer healthCheckCancel()

			if err := u.healthCheckUsecase.performLayerHealthCheck(healthCheckCtx, layer, options); err != nil {
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
			"layer":                  layer.Name,
			"layer_index":            layerIndex + 1,
			"duration":               layerDuration,
			"success":                layerErr == nil,
			"health_check_performed": layer.RequiresHealthCheck && !options.DryRun,
		})

		// If layer failed and not in dry-run mode, stop deployment
		if layerErr != nil && !options.DryRun {
			return progress, fmt.Errorf("layer deployment failed: %s - %w", layer.Name, layerErr)
		}
	}

	u.logger.InfoWithContext("layer-aware deployment completed", map[string]interface{}{
		"deployment_strategy": "layer_aware",
		"total_layers":        len(layers),
		"successful_charts":   progress.GetSuccessCount(),
		"failed_charts":       progress.GetFailedCount(),
		"skipped_charts":      progress.GetSkippedCount(),
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
		"parallel_enabled":         u.enableParallel,
		"cache_enabled":            u.enableCache,
		"dependency_aware_enabled": u.enableDependencyAware,
		"parallel_deployer_ready":  u.parallelDeployer != nil,
		"cache_ready":              u.cache != nil,
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
				"chart":   chart.Name,
				"attempt": attempt,
				"error":   ctx.Err().Error(),
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
					"chart":   chart.Name,
					"attempt": attempt,
					"timeout": "2m",
				})
			} else {
				u.logger.WarnWithContext("dependency wait failed, continuing with deployment", map[string]interface{}{
					"chart":   chart.Name,
					"attempt": attempt,
					"error":   err.Error(),
				})
			}
			// Continue with deployment even if dependencies aren't ready
			// This allows for graceful degradation
		} else {
			depCancel()
			u.logger.InfoWithContext("dependencies ready for chart", map[string]interface{}{
				"chart":   chart.Name,
				"attempt": attempt,
			})
		}

		result := u.deploySingleChart(ctx, chart, options)
		attemptDuration := time.Since(attemptStartTime)

		u.logger.InfoWithContext("chart deployment attempt completed", map[string]interface{}{
			"chart":      chart.Name,
			"attempt":    attempt,
			"status":     result.Status,
			"duration":   attemptDuration,
			"namespace":  result.Namespace,
			"error":      u.formatError(result.Error),
			"will_retry": result.Status == domain.DeploymentStatusFailed && attempt < maxRetries && u.isRetriableError(result.Error),
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
			"chart":          chart.Name,
			"failed_attempt": attempt,
			"error":          result.Error.Error(),
			"retry_delay":    retryDelay,
			"next_attempt":   attempt + 1,
		})

		// Wait for retry delay or context cancellation
		select {
		case <-ctx.Done():
			u.logger.WarnWithContext("deployment cancelled during retry wait", map[string]interface{}{
				"chart":   chart.Name,
				"attempt": attempt,
				"error":   ctx.Err().Error(),
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
		if containsInsensitive(errorMsg, pattern) {
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

// Note: Utility functions moved to shared_utils.go to avoid duplication

// isStatefulSetChart determines if a chart deploys a StatefulSet (delegated to StatefulSetManagementUsecase)
func (u *DeploymentUsecase) isStatefulSetChart(chartName string) bool {
	return u.statefulSetManagementUsecase.isStatefulSetChart(chartName)
}

// isSecretOnlyChart determines if a chart only creates secrets/configmaps
func (u *DeploymentUsecase) isSecretOnlyChart(chartName string) bool {
	secretOnlyCharts := []string{
		"common-secrets", "common-config", "common-ssl",
	}

	for _, secretChart := range secretOnlyCharts {
		if chartName == secretChart {
			return true
		}
	}
	return false
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
			WaitBetweenCharts:       30 * time.Second,
			LayerCompletionTimeout:  20 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:           true,
		},
		{
			Name: "Configuration & Secrets",
			Charts: []domain.Chart{
				{Name: "common-secrets", Type: domain.InfrastructureChart, Path: chartsDir + "/common-secrets", WaitReady: false, MultiNamespace: true, TargetNamespaces: []string{"alt-apps", "alt-auth"}},
				{Name: "common-config", Type: domain.InfrastructureChart, Path: chartsDir + "/common-config", WaitReady: false},
				{Name: "common-ssl", Type: domain.InfrastructureChart, Path: chartsDir + "/common-ssl", WaitReady: false, MultiNamespace: true, TargetNamespaces: []string{"alt-apps", "alt-database", "alt-ingress", "alt-search", "alt-auth"}},
			},
			RequiresHealthCheck:     true, // Enable health check for secret charts
			HealthCheckTimeout:      3 * time.Minute,
			WaitBetweenCharts:       10 * time.Second,
			LayerCompletionTimeout:  8 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:           true,
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
			WaitBetweenCharts:       15 * time.Second,
			LayerCompletionTimeout:  15 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:           true,
		},
		{
			Name: "Network & Ingress",
			Charts: []domain.Chart{
				{Name: "nginx", Type: domain.InfrastructureChart, Path: chartsDir + "/nginx", WaitReady: false},
				{Name: "nginx-external", Type: domain.InfrastructureChart, Path: chartsDir + "/nginx-external", WaitReady: false},
			},
			RequiresHealthCheck:     false,
			HealthCheckTimeout:      5 * time.Minute,
			WaitBetweenCharts:       10 * time.Second,
			LayerCompletionTimeout:  8 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:           false,
		},
		{
			Name: "Frontend Applications",
			Charts: []domain.Chart{
				{Name: "alt-frontend", Type: domain.ApplicationChart, Path: chartsDir + "/alt-frontend", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      8 * time.Minute,
			WaitBetweenCharts:       10 * time.Second,
			LayerCompletionTimeout:  10 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:           false,
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
			WaitBetweenCharts:       20 * time.Second,
			LayerCompletionTimeout:  15 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:           false,
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
			WaitBetweenCharts:       10 * time.Second,
			LayerCompletionTimeout:  10 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:           false,
		},
	}
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
			"deployment_id":    deploymentID,
			"current_phase":    progress.CurrentPhase,
			"current_chart":    progress.CurrentChart,
			"completed_charts": progress.CompletedCharts,
			"total_charts":     progress.TotalCharts,
			"progress_percent": float64(progress.CompletedCharts) / float64(progress.TotalCharts) * 100.0,
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
				"chart":      chart.Name,
				"operation":  operation.Type,
				"status":     operation.Status,
				"start_time": operation.StartTime,
				"namespace":  options.GetNamespace(chart.Name),
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
					"chart":     chart.Name,
					"operation": operation.Type,
					"status":    operation.Status,
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
		"total_charts":       len(allCharts),
		"charts_with_issues": len(cleanupResults),
		"cleanup_successes":  successCount,
		"cleanup_failures":   failureCount,
	})

	// Return error only if all cleanups failed
	if failureCount > 0 && successCount == 0 {
		return fmt.Errorf("all helm operation cleanups failed")
	}

	return nil
}

// secretExists checks if a secret exists in the given namespace
func (u *DeploymentUsecase) secretExists(ctx context.Context, secretName, namespace string) bool {
	_, err := u.kubectlGateway.GetSecret(ctx, secretName, namespace)
	return err == nil
}

// SSL certificate management methods moved to ssl_management_usecase.go
