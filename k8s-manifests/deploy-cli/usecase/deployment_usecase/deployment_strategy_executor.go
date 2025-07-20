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
		e.logger.InfoWithContext("🚀 STARTING layer deployment", map[string]interface{}{
			"layer":                 layer.Name,
			"layer_index":           layerIndex + 1,
			"total_layers":          len(layers),
			"chart_count":           len(layer.Charts),
			"requires_health_check": layer.RequiresHealthCheck,
			"deployment_strategy":   "layer_aware_sequential",
		})

		// Create layer-specific timeout context
		layerCtx, layerCancel := context.WithTimeout(ctx, layer.LayerCompletionTimeout)
		// Don't defer cancel here as it will be called at end of loop iteration

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
			// Check for context cancellation before each chart
			select {
			case <-layerCtx.Done():
				e.logger.WarnWithContext("layer context cancelled during chart deployment", map[string]interface{}{
					"layer": layer.Name,
					"chart": chart.Name,
					"error": layerCtx.Err().Error(),
					"charts_completed": chartIndex,
					"total_charts": len(layer.Charts),
				})
				layerErr = layerCtx.Err()
				break
			default:
			}
			
			// Check if chart directory exists
			if _, err := chartConfig.GetChart(chart.Name); err != nil {
				e.logger.WarnWithContext("chart not found in configuration, skipping", map[string]interface{}{
					"chart": chart.Name,
					"layer": layer.Name,
					"error": err.Error(),
				})
				continue
			}

			// Wait for dependencies before deploying (with context check)
			if err := e.dependencyWaiter.WaitForDependencies(layerCtx, chart.Name); err != nil {
				// Check if error is due to context cancellation
				if layerCtx.Err() != nil {
					e.logger.WarnWithContext("dependency wait cancelled due to context", map[string]interface{}{
						"chart": chart.Name,
						"layer": layer.Name,
						"context_error": layerCtx.Err().Error(),
					})
					layerErr = layerCtx.Err()
					break
				}
				e.logger.WarnWithContext("dependency wait failed, continuing with deployment", map[string]interface{}{
					"chart": chart.Name,
					"layer": layer.Name,
					"error": err.Error(),
				})
			}

			// Deploy the chart - handle multi-namespace deployment
			e.logger.InfoWithContext("🚀 Starting chart deployment", map[string]interface{}{
				"chart":       chart.Name,
				"layer":       layer.Name,
				"chart_index": chartIndex + 1,
				"total_charts_in_layer": len(layer.Charts),
				"multi_namespace": chart.MultiNamespace,
				"elapsed_time": time.Since(layerStartTime).String(),
			})
			
			if chart.MultiNamespace {
				for nsIndex, targetNamespace := range chart.TargetNamespaces {
					e.logger.InfoWithContext("📦 Deploying to namespace", map[string]interface{}{
						"chart":     chart.Name,
						"namespace": targetNamespace,
						"ns_index":  nsIndex + 1,
						"total_ns":  len(chart.TargetNamespaces),
					})
					
					chartCopy := chart
					chartCopy.MultiNamespace = false
					result := e.deploySingleChartToNamespace(layerCtx, chartCopy, targetNamespace, options)
					progress.AddResult(result)

					e.logger.InfoWithContext("📊 Chart deployment result", map[string]interface{}{
						"chart":     chart.Name,
						"namespace": targetNamespace,
						"status":    result.Status,
						"duration":  result.Duration.String(),
					})

					if result.Status == domain.DeploymentStatusFailed {
						layerErr = result.Error
						if !options.DryRun {
							break
						}
					}
				}
			} else {
				e.logger.InfoWithContext("⚡ Deploying single chart", map[string]interface{}{
					"chart": chart.Name,
					"layer": layer.Name,
					"about_to_call": "deploySingleChart",
				})
				
				e.logger.InfoWithContext("📞 CALLING deploySingleChart", map[string]interface{}{
					"chart": chart.Name,
					"layer": layer.Name,
					"function": "deploySingleChart",
				})
				
				result := e.deploySingleChart(layerCtx, chart, options)
				
				e.logger.InfoWithContext("📥 RETURNED from deploySingleChart", map[string]interface{}{
					"chart": chart.Name,
					"status": result.Status,
					"duration": result.Duration.String(),
				})
				
				progress.AddResult(result)

				e.logger.InfoWithContext("✨ Single chart deployment completed", map[string]interface{}{
					"chart":    chart.Name,
					"status":   result.Status,
					"duration": result.Duration.String(),
					"error":    func() string {
						if result.Error != nil {
							return result.Error.Error()
						}
						return "none"
					}(),
				})

				if result.Status == domain.DeploymentStatusFailed {
					layerErr = result.Error
					if !options.DryRun {
						break
					}
				}
			}

			// Wait between charts in the same layer if specified
			if chartIndex < len(layer.Charts)-1 && layer.WaitBetweenCharts > 0 {
				e.logger.DebugWithContext("waiting between charts", map[string]interface{}{
					"layer": layer.Name,
					"wait_duration": layer.WaitBetweenCharts,
					"completed_chart": chart.Name,
					"remaining_charts": len(layer.Charts) - chartIndex - 1,
				})
				
				select {
				case <-layerCtx.Done():
					e.logger.WarnWithContext("context cancelled during inter-chart wait", map[string]interface{}{
						"layer": layer.Name,
						"cancelled_after_chart": chart.Name,
						"error": layerCtx.Err().Error(),
					})
					layerErr = layerCtx.Err()
					break
				case <-time.After(layer.WaitBetweenCharts):
					// Continue to next chart
					e.logger.DebugWithContext("inter-chart wait completed", map[string]interface{}{
						"layer": layer.Name,
						"completed_chart": chart.Name,
					})
				}
			}
			
			// Exit chart loop if we have an error (including context cancellation)
			if layerErr != nil {
				break
			}
		}

		layerDuration := time.Since(layerStartTime)

		e.logger.InfoWithContext("🔄 CHECKING layer completion requirements", map[string]interface{}{
			"layer": layer.Name,
			"layer_error": layerErr == nil,
			"requires_health_check": layer.RequiresHealthCheck,
			"dry_run": options.DryRun,
			"skip_health_checks": options.SkipHealthChecks,
			"emergency_mode": options.SkipStatefulSetRecovery, // Using this as emergency mode indicator
		})

		// If layer requires health check and deployment was successful, perform health check
		if layerErr == nil && layer.RequiresHealthCheck && !options.DryRun && !options.SkipHealthChecks {
			// Check if layer context is still valid before health check
			select {
			case <-layerCtx.Done():
				e.logger.WarnWithContext("🚨 Layer context cancelled before health check", map[string]interface{}{
					"layer": layer.Name,
					"error": layerCtx.Err().Error(),
				})
				if !options.SkipStatefulSetRecovery { // Not in emergency mode
					layerErr = layerCtx.Err()
				}
			default:
				e.logger.InfoWithContext("🩺 STARTING layer health check", map[string]interface{}{
					"layer": layer.Name,
					"timeout": layer.HealthCheckTimeout.String(),
					"emergency_mode": options.SkipStatefulSetRecovery,
				})
				
				healthCheckCtx, healthCheckCancel := context.WithTimeout(layerCtx, layer.HealthCheckTimeout)
				// Use defer to ensure context is always cancelled
				defer healthCheckCancel()

				if err := e.performLayerHealthCheck(healthCheckCtx, layer, options); err != nil {
					// Check if error is due to context cancellation
					if healthCheckCtx.Err() != nil {
						e.logger.WarnWithContext("🚨 Health check cancelled due to context", map[string]interface{}{
							"layer": layer.Name,
							"context_error": healthCheckCtx.Err().Error(),
							"original_error": err.Error(),
							"emergency_mode": options.SkipStatefulSetRecovery,
						})
						if !options.SkipStatefulSetRecovery { // Not in emergency mode
							layerErr = healthCheckCtx.Err()
						}
					} else {
						e.logger.ErrorWithContext("❌ Layer health check FAILED", map[string]interface{}{
							"layer": layer.Name,
							"error": err.Error(),
							"emergency_mode": options.SkipStatefulSetRecovery,
						})
						
						if options.SkipStatefulSetRecovery { // Emergency mode
							e.logger.WarnWithContext("🚨 SKIPPING health check in emergency mode", map[string]interface{}{
								"layer": layer.Name,
							})
						} else {
							layerErr = fmt.Errorf("layer health check failed: %w", err)
						}
					}
				} else {
					e.logger.InfoWithContext("✅ Layer health check PASSED", map[string]interface{}{
						"layer": layer.Name,
					})
				}
			}
		} else if options.SkipHealthChecks {
			e.logger.InfoWithContext("⏭️ SKIPPING health check (--skip-health-checks flag)", map[string]interface{}{
				"layer": layer.Name,
				"requires_health_check": layer.RequiresHealthCheck,
			})
		} else if layerErr != nil {
			e.logger.InfoWithContext("⏭️ SKIPPING health check due to layer error", map[string]interface{}{
				"layer": layer.Name,
				"layer_error": layerErr.Error(),
			})
		}

		e.logger.InfoWithContext("🎯 LAYER COMPLETION STATUS", map[string]interface{}{
			"layer":       layer.Name,
			"layer_index": layerIndex + 1,
			"duration":    layerDuration,
			"success":     layerErr == nil,
			"about_to_complete": true,
		})

		e.logger.InfoWithContext("✅ LAYER DEPLOYMENT COMPLETED - proceeding to next layer", map[string]interface{}{
			"layer":       layer.Name,
			"layer_index": layerIndex + 1,
			"duration":    layerDuration,
			"success":     layerErr == nil,
			"next_layer_index": layerIndex + 2,
			"total_layers": len(layers),
		})

		// If layer failed and not in dry-run mode, stop deployment
		if layerErr != nil && !options.DryRun {
			return progress, fmt.Errorf("layer deployment failed: %s - %w", layer.Name, layerErr)
		}
		
		// Add explicit logging before next iteration
		if layerIndex+1 < len(layers) {
			nextLayer := layers[layerIndex+1]
			e.logger.InfoWithContext("🔄 PREPARING next layer", map[string]interface{}{
				"current_layer":    layer.Name,
				"next_layer":       nextLayer.Name,
				"next_layer_index": layerIndex + 2,
				"charts_in_next":   len(nextLayer.Charts),
			})
		} else {
			e.logger.InfoWithContext("🎯 FINAL layer completed - deployment finishing", map[string]interface{}{
				"final_layer": layer.Name,
				"total_layers": len(layers),
			})
		}
		
		// Safely cancel the layer context at the end of each iteration
		func() {
			defer func() {
				if r := recover(); r != nil {
					e.logger.WarnWithContext("panic during layer context cancellation", map[string]interface{}{
						"layer": layer.Name,
						"layer_index": layerIndex + 1,
						"panic": r,
					})
				}
			}()
			layerCancel()
			e.logger.DebugWithContext("layer context cancelled safely", map[string]interface{}{
				"layer": layer.Name,
				"layer_index": layerIndex + 1,
				"layer_success": layerErr == nil,
			})
		}()
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
	e.logger.InfoWithContext("🔍 Resolving chart namespace", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": namespace,
		"function":  "deploySingleChart",
	})
	return e.deploySingleChartToNamespace(ctx, chart, namespace, options)
}

// deploySingleChartToNamespace deploys a single chart to a specific namespace
func (e *DeploymentStrategyExecutor) deploySingleChartToNamespace(ctx context.Context, chart domain.Chart, namespace string, options *domain.DeploymentOptions) domain.DeploymentResult {
	start := time.Now()

	e.logger.InfoWithContext("🎯 ENTERING deploySingleChartToNamespace", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": namespace,
		"function":  "deploySingleChartToNamespace",
		"start_time": start.Format(time.RFC3339),
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
	e.logger.InfoWithContext("🔍 Validating chart path", map[string]interface{}{
		"chart": chart.Name,
		"path":  chart.Path,
	})
	if err := e.filesystemGateway.ValidateChartPath(chart); err != nil {
		e.logger.ErrorWithContext("❌ Chart path validation failed", map[string]interface{}{
			"chart": chart.Name,
			"error": err.Error(),
		})
		result.Error = fmt.Errorf("chart path validation failed: %w", err)
		result.Status = domain.DeploymentStatusSkipped
		result.Duration = time.Since(start)
		return result
	}

	// Validate values file
	e.logger.InfoWithContext("📄 Validating values file", map[string]interface{}{
		"chart":       chart.Name,
		"environment": options.Environment.String(),
	})
	valuesFile, err := e.filesystemGateway.ValidateValuesFile(chart, options.Environment)
	if err != nil {
		e.logger.ErrorWithContext("❌ Values file validation failed", map[string]interface{}{
			"chart": chart.Name,
			"error": err.Error(),
		})
		result.Error = fmt.Errorf("values file validation failed: %w", err)
		result.Status = domain.DeploymentStatusSkipped
		result.Duration = time.Since(start)
		return result
	}
	e.logger.InfoWithContext("✅ Values file validated", map[string]interface{}{
		"chart":       chart.Name,
		"values_file": valuesFile,
	})

	// Create namespace-specific deployment options
	nsOptions := *options
	nsOptions.TargetNamespace = namespace

	// Deploy or template chart with timeout handling
	if options.DryRun {
		e.logger.InfoWithContext("🧪 Starting dry-run templating", map[string]interface{}{
			"chart":   chart.Name,
			"timeout": chartTimeout.String(),
		})
		_, err = e.helmGateway.TemplateChart(chartCtx, chart, &nsOptions)
		if err != nil {
			if chartCtx.Err() == context.DeadlineExceeded {
				e.logger.ErrorWithContext("⏰ Chart templating TIMEOUT", map[string]interface{}{
					"chart":   chart.Name,
					"timeout": chartTimeout.String(),
				})
				result.Error = fmt.Errorf("chart templating timed out after %v", chartTimeout)
			} else {
				e.logger.ErrorWithContext("❌ Chart templating FAILED", map[string]interface{}{
					"chart": chart.Name,
					"error": err.Error(),
				})
				result.Error = fmt.Errorf("chart templating failed: %w", err)
			}
		} else {
			e.logger.InfoWithContext("✅ Chart templating SUCCESS", map[string]interface{}{
				"chart": chart.Name,
			})
			result.Status = domain.DeploymentStatusSuccess
			result.Message = "Chart templated successfully"
		}
	} else {
		chartNamespace := e.getNamespaceForChart(chart)
		e.logger.InfoWithContext("🚀 Starting HELM deployment", map[string]interface{}{
			"chart":          chart.Name,
			"namespace":      chartNamespace,
			"timeout":        chartTimeout.String(),
			"about_to_call":  "helmOperationManager.ExecuteWithLock",
		})
		
		err = e.helmOperationManager.ExecuteWithLock(chart.Name, chartNamespace, "deploy", func() error {
			e.logger.InfoWithContext("🔐 INSIDE Helm lock - about to call DeployChart", map[string]interface{}{
				"chart":     chart.Name,
				"namespace": chartNamespace,
			})
			
			helmErr := e.helmGateway.DeployChart(chartCtx, chart, &nsOptions)
			
			e.logger.InfoWithContext("🔓 Helm DeployChart returned", map[string]interface{}{
				"chart":     chart.Name,
				"namespace": chartNamespace,
				"error":     func() string {
					if helmErr != nil {
						return helmErr.Error()
					}
					return "none"
				}(),
			})
			
			return helmErr
		})
		
		if err != nil {
			if chartCtx.Err() == context.DeadlineExceeded {
				e.logger.ErrorWithContext("⏰ Chart deployment TIMEOUT", map[string]interface{}{
					"chart":   chart.Name,
					"timeout": chartTimeout.String(),
				})
				result.Error = fmt.Errorf("chart deployment timed out after %v", chartTimeout)
			} else {
				e.logger.ErrorWithContext("❌ Chart deployment FAILED", map[string]interface{}{
					"chart": chart.Name,
					"error": err.Error(),
				})
				result.Error = fmt.Errorf("chart deployment failed: %w", err)
			}
		} else {
			e.logger.InfoWithContext("🎉 Chart deployment SUCCESS", map[string]interface{}{
				"chart": chart.Name,
			})
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
	// Use domain layer to determine namespace consistently
	return domain.DetermineNamespace(chart.Name, domain.Production)
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
	e.logger.InfoWithContext("🩺 STARTING performLayerHealthCheck", map[string]interface{}{
		"layer": layer.Name,
		"charts_count": len(layer.Charts),
		"context_deadline": ctx.Err() == nil,
	})

	for chartIndex, chart := range layer.Charts {
		if chart.WaitReady {
			namespace := options.GetNamespace(chart.Name)

			e.logger.InfoWithContext("🔍 CHECKING individual chart health", map[string]interface{}{
				"chart": chart.Name,
				"namespace": namespace,
				"chart_index": chartIndex + 1,
				"total_charts": len(layer.Charts),
				"chart_type": string(chart.Type),
			})

			var err error
			switch chart.Name {
			case "postgres", "auth-postgres", "kratos-postgres":
				e.logger.InfoWithContext("🐘 PostgreSQL health check STARTING", map[string]interface{}{
					"chart": chart.Name,
					"namespace": namespace,
					"about_to_call": "WaitForPostgreSQLReady",
				})
				err = e.healthChecker.WaitForPostgreSQLReady(ctx, namespace, chart.Name)
				if err != nil {
					e.logger.ErrorWithContext("🐘 PostgreSQL health check FAILED", map[string]interface{}{
						"chart": chart.Name,
						"namespace": namespace,
						"error": err.Error(),
					})
				} else {
					e.logger.InfoWithContext("🐘 PostgreSQL health check PASSED", map[string]interface{}{
						"chart": chart.Name,
						"namespace": namespace,
					})
				}
			case "meilisearch":
				e.logger.InfoWithContext("🔍 Meilisearch health check STARTING", map[string]interface{}{
					"chart": chart.Name,
					"namespace": namespace,
				})
				err = e.healthChecker.WaitForMeilisearchReady(ctx, namespace, chart.Name)
			default:
				e.logger.InfoWithContext("⚙️ Service health check STARTING", map[string]interface{}{
					"chart": chart.Name,
					"namespace": namespace,
					"service_type": string(chart.Type),
				})
				err = e.healthChecker.WaitForServiceReady(ctx, chart.Name, string(chart.Type), namespace)
			}

			if err != nil {
				e.logger.ErrorWithContext("❌ Chart health check FAILED", map[string]interface{}{
					"chart": chart.Name,
					"namespace": namespace,
					"error": err.Error(),
				})
				return fmt.Errorf("health check failed for chart %s: %w", chart.Name, err)
			}

			e.logger.InfoWithContext("✅ Chart health check COMPLETED", map[string]interface{}{
				"chart": chart.Name,
				"namespace": namespace,
			})
		} else {
			e.logger.InfoWithContext("⏭️ SKIPPING health check (WaitReady=false)", map[string]interface{}{
				"chart": chart.Name,
			})
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
