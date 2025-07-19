package deployment_usecase

import (
	"context"
	"fmt"
	"time"

	"deploy-cli/domain"
	"deploy-cli/gateway/filesystem_gateway"
	"deploy-cli/gateway/helm_gateway"
	"deploy-cli/port/logger_port"
	"deploy-cli/usecase/dependency_usecase"
)

// DeploymentStrategyExecutor handles different deployment strategies and execution
type DeploymentStrategyExecutor struct {
	helmGateway           *helm_gateway.HelmGateway
	filesystemGateway     *filesystem_gateway.FileSystemGateway
	dependencyScanner     *dependency_usecase.DependencyScanner
	healthChecker         *HealthChecker
	dependencyWaiter      *DependencyWaiter
	strategyFactory       *StrategyFactory
	parallelDeployer      *ParallelChartDeployer
	helmOperationManager  *HelmOperationManager
	logger                logger_port.LoggerPort
	enableParallel        bool
	enableDependencyAware bool
}

// NewDeploymentStrategyExecutor creates a new deployment strategy executor
func NewDeploymentStrategyExecutor(
	helmGateway *helm_gateway.HelmGateway,
	filesystemGateway *filesystem_gateway.FileSystemGateway,
	dependencyScanner *dependency_usecase.DependencyScanner,
	healthChecker *HealthChecker,
	dependencyWaiter *DependencyWaiter,
	strategyFactory *StrategyFactory,
	helmOperationManager *HelmOperationManager,
	logger logger_port.LoggerPort,
) *DeploymentStrategyExecutor {
	return &DeploymentStrategyExecutor{
		helmGateway:           helmGateway,
		filesystemGateway:     filesystemGateway,
		dependencyScanner:     dependencyScanner,
		healthChecker:         healthChecker,
		dependencyWaiter:      dependencyWaiter,
		strategyFactory:       strategyFactory,
		helmOperationManager:  helmOperationManager,
		logger:                logger,
		enableParallel:        false,
		enableDependencyAware: true,
	}
}

// DeployCharts deploys all charts using the appropriate strategy
func (e *DeploymentStrategyExecutor) DeployCharts(ctx context.Context, options *domain.DeploymentOptions) (*domain.DeploymentProgress, error) {
	e.logger.InfoWithContext("deploying charts", map[string]interface{}{
		"environment":      options.Environment.String(),
		"dependency_aware": e.enableDependencyAware,
	})

	// Get chart configuration
	chartConfig := domain.NewChartConfig(options.ChartsDir)
	allCharts := chartConfig.AllCharts()

	// Create deployment progress
	progress := domain.NewDeploymentProgress(len(allCharts))

	// Use layer-aware deployment for correct ordering
	if e.enableDependencyAware {
		return e.deployChartsWithLayerAwareness(ctx, options, progress)
	}

	// Fallback to traditional group-based deployment
	return e.deployChartsTraditional(ctx, options, progress)
}

// deployChartsWithLayerAwareness deploys charts in predefined layers for correct ordering
func (e *DeploymentStrategyExecutor) deployChartsWithLayerAwareness(ctx context.Context, options *domain.DeploymentOptions, progress *domain.DeploymentProgress) (*domain.DeploymentProgress, error) {
	e.logger.InfoWithContext("starting layer-aware deployment", map[string]interface{}{
		"deployment_strategy": "layer_aware",
		"charts_dir":          options.ChartsDir,
		"strategy":            options.GetStrategyName(),
	})

	// Get layer configurations from the deployment strategy
	var layers []domain.LayerConfiguration
	if options.HasDeploymentStrategy() {
		layers = options.GetLayerConfigurations()
		e.logger.InfoWithContext("using strategy-based layer configurations", map[string]interface{}{
			"strategy":     options.GetStrategyName(),
			"layers_count": len(layers),
		})
	} else {
		// Fallback to default configuration
		chartConfig := domain.NewChartConfig(options.ChartsDir)
		layers = e.getDefaultLayerConfigurations(chartConfig, options.ChartsDir)
		e.logger.InfoWithContext("using default layer configurations", map[string]interface{}{
			"layers_count": len(layers),
		})
	}

	// Get chart configuration for chart validation
	chartConfig := domain.NewChartConfig(options.ChartsDir)

	// Deploy each layer sequentially
	for layerIndex, layer := range layers {
		e.logger.InfoWithContext("deploying layer", map[string]interface{}{
			"layer":                 layer.Name,
			"layer_index":           layerIndex + 1,
			"total_layers":          len(layers),
			"chart_count":           len(layer.Charts),
			"requires_health_check": layer.RequiresHealthCheck,
		})

		// Create layer-specific timeout context
		layerCtx, layerCancel := context.WithTimeout(ctx, layer.LayerCompletionTimeout)
		defer layerCancel()

		// Check for context cancellation
		select {
		case <-layerCtx.Done():
			e.logger.WarnWithContext("deployment cancelled during layer deployment", map[string]interface{}{
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
				e.logger.WarnWithContext("chart not found in configuration, skipping", map[string]interface{}{
					"chart": chart.Name,
					"layer": layer.Name,
					"error": err.Error(),
				})
				continue
			}

			// Wait for dependencies before deploying
			if err := e.dependencyWaiter.WaitForDependencies(layerCtx, chart.Name); err != nil {
				e.logger.WarnWithContext("dependency wait failed, continuing with deployment", map[string]interface{}{
					"chart": chart.Name,
					"layer": layer.Name,
					"error": err.Error(),
				})
			}

			// Deploy the chart - handle multi-namespace deployment
			if chart.MultiNamespace {
				for _, targetNamespace := range chart.TargetNamespaces {
					chartCopy := chart
					chartCopy.MultiNamespace = false
					result := e.deploySingleChartToNamespace(layerCtx, chartCopy, targetNamespace, options)
					progress.AddResult(result)

					if result.Status == domain.DeploymentStatusFailed {
						layerErr = result.Error
						if !options.DryRun {
							break
						}
					}
				}
			} else {
				result := e.deploySingleChart(layerCtx, chart, options)
				progress.AddResult(result)

				if result.Status == domain.DeploymentStatusFailed {
					layerErr = result.Error
					if !options.DryRun {
						break
					}
				}
			}

			// Wait between charts in the same layer if specified
			if chartIndex < len(layer.Charts)-1 && layer.WaitBetweenCharts > 0 {
				select {
				case <-layerCtx.Done():
					return progress, layerCtx.Err()
				case <-time.After(layer.WaitBetweenCharts):
					// Continue to next chart
				}
			}
		}

		layerDuration := time.Since(layerStartTime)

		// If layer requires health check and deployment was successful, perform health check
		if layerErr == nil && layer.RequiresHealthCheck && !options.DryRun {
			healthCheckCtx, healthCheckCancel := context.WithTimeout(layerCtx, layer.HealthCheckTimeout)
			defer healthCheckCancel()

			if err := e.performLayerHealthCheck(healthCheckCtx, layer, options); err != nil {
				e.logger.ErrorWithContext("layer health check failed", map[string]interface{}{
					"layer": layer.Name,
					"error": err.Error(),
				})
				layerErr = fmt.Errorf("layer health check failed: %w", err)
			}
		}

		e.logger.InfoWithContext("layer deployment completed", map[string]interface{}{
			"layer":       layer.Name,
			"layer_index": layerIndex + 1,
			"duration":    layerDuration,
			"success":     layerErr == nil,
		})

		// If layer failed and not in dry-run mode, stop deployment
		if layerErr != nil && !options.DryRun {
			return progress, fmt.Errorf("layer deployment failed: %s - %w", layer.Name, layerErr)
		}
	}

	e.logger.InfoWithContext("layer-aware deployment completed", map[string]interface{}{
		"deployment_strategy": "layer_aware",
		"total_layers":        len(layers),
		"successful_charts":   progress.GetSuccessCount(),
		"failed_charts":       progress.GetFailedCount(),
		"skipped_charts":      progress.GetSkippedCount(),
	})

	return progress, nil
}

// deployChartsTraditional deploys charts using the traditional group-based approach
func (e *DeploymentStrategyExecutor) deployChartsTraditional(ctx context.Context, options *domain.DeploymentOptions, progress *domain.DeploymentProgress) (*domain.DeploymentProgress, error) {
	e.logger.InfoWithContext("starting traditional group-based deployment", map[string]interface{}{
		"deployment_strategy": "traditional_group_based",
		"charts_dir":          options.ChartsDir,
	})

	chartConfig := domain.NewChartConfig(options.ChartsDir)

	var deploymentErrors []error

	// Deploy infrastructure charts
	if err := e.deployChartGroup(ctx, "Infrastructure", chartConfig.InfrastructureCharts, options, progress); err != nil {
		deploymentErrors = append(deploymentErrors, fmt.Errorf("infrastructure chart deployment failed: %w", err))
		if !options.DryRun {
			return progress, deploymentErrors[0]
		}
	}

	// Deploy application charts
	if err := e.deployChartGroup(ctx, "Application", chartConfig.ApplicationCharts, options, progress); err != nil {
		deploymentErrors = append(deploymentErrors, fmt.Errorf("application chart deployment failed: %w", err))
		if !options.DryRun {
			return progress, deploymentErrors[len(deploymentErrors)-1]
		}
	}

	// Deploy operational charts
	if err := e.deployChartGroup(ctx, "Operational", chartConfig.OperationalCharts, options, progress); err != nil {
		deploymentErrors = append(deploymentErrors, fmt.Errorf("operational chart deployment failed: %w", err))
		if !options.DryRun {
			return progress, deploymentErrors[len(deploymentErrors)-1]
		}
	}

	return progress, nil
}

// deployChartGroup deploys a group of charts
func (e *DeploymentStrategyExecutor) deployChartGroup(ctx context.Context, groupName string, charts []domain.Chart, options *domain.DeploymentOptions, progress *domain.DeploymentProgress) error {
	e.logger.InfoWithContext("deploying chart group", map[string]interface{}{
		"group":       groupName,
		"chart_count": len(charts),
	})

	for _, chart := range charts {
		// Check if context was cancelled
		if ctx.Err() != nil {
			return ctx.Err()
		}

		progress.CurrentChart = chart.Name
		progress.CurrentPhase = fmt.Sprintf("Deploying %s charts", groupName)

		// Handle multi-namespace deployment
		if chart.MultiNamespace {
			for _, targetNamespace := range chart.TargetNamespaces {
				chartCopy := chart
				chartCopy.MultiNamespace = false
				result := e.deploySingleChartToNamespace(ctx, chartCopy, targetNamespace, options)
				progress.AddResult(result)

				if result.Status == domain.DeploymentStatusFailed {
					if !options.DryRun {
						return fmt.Errorf("chart deployment failed: %s", result.Error)
					}
				}
			}
		} else {
			result := e.deploySingleChart(ctx, chart, options)
			progress.AddResult(result)

			if result.Status == domain.DeploymentStatusFailed {
				if !options.DryRun {
					return fmt.Errorf("chart deployment failed: %s", result.Error)
				}
			}
		}
	}

	return nil
}

// deploySingleChart deploys a single chart
func (e *DeploymentStrategyExecutor) deploySingleChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) domain.DeploymentResult {
	namespace := options.GetNamespace(chart.Name)
	return e.deploySingleChartToNamespace(ctx, chart, namespace, options)
}

// deploySingleChartToNamespace deploys a single chart to a specific namespace
func (e *DeploymentStrategyExecutor) deploySingleChartToNamespace(ctx context.Context, chart domain.Chart, namespace string, options *domain.DeploymentOptions) domain.DeploymentResult {
	start := time.Now()

	e.logger.InfoWithContext("deploying single chart", map[string]interface{}{
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

	// Use longer timeout for StatefulSet database charts
	if e.isStatefulSetChart(chart.Name) {
		chartTimeout = 10 * time.Minute
	}

	chartCtx, cancel := context.WithTimeout(ctx, chartTimeout)
	defer cancel()

	// Validate chart path
	if err := e.filesystemGateway.ValidateChartPath(chart); err != nil {
		result.Error = fmt.Errorf("chart path validation failed: %w", err)
		result.Status = domain.DeploymentStatusSkipped
		result.Duration = time.Since(start)
		return result
	}

	// Validate values file
	valuesFile, err := e.filesystemGateway.ValidateValuesFile(chart, options.Environment)
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
		_, err = e.helmGateway.TemplateChart(chartCtx, chart, &nsOptions)
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
		chartNamespace := e.getNamespaceForChart(chart)
		err = e.helmOperationManager.ExecuteWithLock(chart.Name, chartNamespace, "deploy", func() error {
			return e.helmGateway.DeployChart(chartCtx, chart, &nsOptions)
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

	e.logger.InfoWithContext("single chart deployment completed", map[string]interface{}{
		"chart":       chart.Name,
		"namespace":   namespace,
		"status":      result.Status,
		"duration":    result.Duration,
		"values_file": valuesFile,
	})

	return result
}

// deployChartsInParallel deploys multiple charts in parallel
func (e *DeploymentStrategyExecutor) deployChartsInParallel(ctx context.Context, charts []domain.Chart, options *domain.DeploymentOptions, progress *domain.DeploymentProgress) error {
	if e.parallelDeployer == nil {
		return e.deployChartsSequentially(ctx, charts, options, progress)
	}

	results, err := e.parallelDeployer.deployChartsParallel(ctx, "dependency-level", charts, options, e.deploySingleChart)
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
func (e *DeploymentStrategyExecutor) deployChartsSequentially(ctx context.Context, charts []domain.Chart, options *domain.DeploymentOptions, progress *domain.DeploymentProgress) error {
	for _, chart := range charts {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		progress.CurrentChart = chart.Name
		progress.CurrentPhase = "Deploying chart"

		result := e.deploySingleChart(ctx, chart, options)
		progress.AddResult(result)

		if result.Status == domain.DeploymentStatusFailed && !options.DryRun {
			return fmt.Errorf("chart deployment failed: %s", result.Error)
		}
	}

	return nil
}

// SetupDeploymentStrategy sets up the deployment strategy
func (e *DeploymentStrategyExecutor) SetupDeploymentStrategy(options *domain.DeploymentOptions) error {
	// Set up deployment strategy based on options
	if options.StrategyName != "" {
		strategy, err := e.strategyFactory.CreateStrategyByName(options.StrategyName)
		if err != nil {
			return fmt.Errorf("failed to create strategy '%s': %w", options.StrategyName, err)
		}
		options.SetDeploymentStrategy(strategy)
	} else if !options.HasDeploymentStrategy() {
		strategy, err := e.strategyFactory.CreateStrategy(options.Environment)
		if err != nil {
			return fmt.Errorf("failed to create strategy for environment '%s': %w", options.Environment.String(), err)
		}
		options.SetDeploymentStrategy(strategy)
	}

	// Validate strategy compatibility
	if err := e.strategyFactory.ValidateStrategyForEnvironment(options.GetDeploymentStrategy(), options.Environment); err != nil {
		return fmt.Errorf("strategy validation failed: %w", err)
	}

	return nil
}

// getAllCharts gets all charts that will be deployed based on deployment options
func (e *DeploymentStrategyExecutor) getAllCharts(options *domain.DeploymentOptions) []domain.Chart {
	chartConfig := domain.NewChartConfig(options.ChartsDir)
	return chartConfig.AllCharts()
}

// getNamespaceForChart returns the appropriate namespace for a chart
func (e *DeploymentStrategyExecutor) getNamespaceForChart(chart domain.Chart) string {
	// Use chart type to determine namespace
	switch chart.Type {
	case domain.InfrastructureChart:
		if chart.Name == "postgres" || chart.Name == "clickhouse" || chart.Name == "meilisearch" {
			return "alt-database"
		}
		if chart.Name == "nginx" || chart.Name == "nginx-external" {
			return "alt-ingress"
		}
		if chart.Name == "auth-postgres" || chart.Name == "kratos-postgres" || chart.Name == "kratos" {
			return "alt-auth"
		}
		return "alt-apps"
	case domain.ApplicationChart:
		if chart.Name == "auth-service" || chart.Name == "kratos" {
			return "alt-auth"
		}
		return "alt-apps"
	case domain.OperationalChart:
		return "alt-apps"
	default:
		return "alt-apps"
	}
}

// isStatefulSetChart determines if a chart deploys a StatefulSet
func (e *DeploymentStrategyExecutor) isStatefulSetChart(chartName string) bool {
	statefulSetCharts := []string{
		"postgres", "auth-postgres", "kratos-postgres",
		"clickhouse", "meilisearch",
	}

	for _, ssChart := range statefulSetCharts {
		if chartName == ssChart {
			return true
		}
	}
	return false
}

// performLayerHealthCheck performs health check for a layer
func (e *DeploymentStrategyExecutor) performLayerHealthCheck(ctx context.Context, layer domain.LayerConfiguration, options *domain.DeploymentOptions) error {
	e.logger.InfoWithContext("performing layer health check", map[string]interface{}{
		"layer": layer.Name,
	})

	for _, chart := range layer.Charts {
		if chart.WaitReady {
			namespace := options.GetNamespace(chart.Name)

			var err error
			switch chart.Name {
			case "postgres", "auth-postgres", "kratos-postgres":
				err = e.healthChecker.WaitForPostgreSQLReady(ctx, namespace, chart.Name)
			case "meilisearch":
				err = e.healthChecker.WaitForMeilisearchReady(ctx, namespace, chart.Name)
			default:
				err = e.healthChecker.WaitForServiceReady(ctx, chart.Name, string(chart.Type), namespace)
			}

			if err != nil {
				return fmt.Errorf("health check failed for chart %s: %w", chart.Name, err)
			}
		}
	}

	return nil
}

// getDefaultLayerConfigurations returns the default layer configurations
func (e *DeploymentStrategyExecutor) getDefaultLayerConfigurations(chartConfig *domain.ChartConfig, chartsDir string) []domain.LayerConfiguration {
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
			RequiresHealthCheck:     true,
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
	}
}

// EnableParallelDeployment enables parallel deployment capabilities
func (e *DeploymentStrategyExecutor) EnableParallelDeployment() {
	if e.parallelDeployer == nil {
		e.parallelDeployer = NewParallelChartDeployer(e.logger, DefaultParallelConfig())
	}
	e.enableParallel = true
}

// SetDependencyAwareDeployment enables or disables dependency-aware deployment
func (e *DeploymentStrategyExecutor) SetDependencyAwareDeployment(enabled bool) {
	e.enableDependencyAware = enabled
}
