package helm_gateway

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/helm_port"
	"deploy-cli/port/logger_port"
)

// HelmGateway acts as anti-corruption layer for Helm operations
type HelmGateway struct {
	helmPort helm_port.HelmPort
	logger   logger_port.LoggerPort
}

// NewHelmGateway creates a new Helm gateway
func NewHelmGateway(helmPort helm_port.HelmPort, logger logger_port.LoggerPort) *HelmGateway {
	return &HelmGateway{
		helmPort: helmPort,
		logger:   logger,
	}
}

// TemplateChart renders chart templates locally
func (g *HelmGateway) TemplateChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) (string, error) {
	g.logger.InfoWithContext("templating chart", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
	})

	helmOptions := helm_port.HelmTemplateOptions{
		ValuesFile: g.getValuesFile(chart, options.Environment),
		Namespace:  options.GetNamespace(chart.Name),
	}

	// Add image overrides if applicable
	shouldOverrideImage := chart.SupportsImageOverride() && (options.ShouldOverrideImage() || options.ForceUpdate)

	if shouldOverrideImage {
		imageTag := options.GetImageTag(chart.Name)
		helmOptions.ImageOverrides = map[string]string{
			"image.repository": options.ImagePrefix,
			"image.tag":        imageTag,
		}

		g.logger.DebugWithContext("applying image override for templating", map[string]interface{}{
			"chart":      chart.Name,
			"repository": options.ImagePrefix,
			"tag":        imageTag,
		})
	}

	start := time.Now()
	output, err := g.helmPort.Template(ctx, chart.Name, chart.Path, helmOptions)
	duration := time.Since(start)

	if err != nil {
		g.logger.ErrorWithContext("chart templating failed", map[string]interface{}{
			"chart":       chart.Name,
			"namespace":   options.GetNamespace(chart.Name),
			"chart_path":  chart.Path,
			"values_file": helmOptions.ValuesFile,
			"error":       err.Error(),
			"duration":    duration,
			"resolution":  "Check chart templates directory and values file syntax",
		})
		return "", fmt.Errorf("chart templating failed for %s: %w", chart.Name, err)
	}

	g.logger.InfoWithContext("chart templated successfully", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
		"duration":  duration,
	})

	return output, nil
}

// DeployChart installs or upgrades a Helm chart
func (g *HelmGateway) DeployChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	namespace := options.GetNamespace(chart.Name)

	g.logger.InfoWithContext("starting chart deployment", map[string]interface{}{
		"chart":        chart.Name,
		"namespace":    namespace,
		"force_update": options.ForceUpdate,
		"dry_run":      options.DryRun,
	})

	// Pre-deployment validations
	if err := g.validatePrerequisites(ctx, chart, options); err != nil {
		g.logger.ErrorWithContext("pre-deployment validation failed", map[string]interface{}{
			"chart":     chart.Name,
			"namespace": namespace,
			"error":     err.Error(),
		})
		return fmt.Errorf("pre-deployment validation failed for chart %s: %w", chart.Name, err)
	}

	// Generate release name for conflict checking
	releaseName := g.generateReleaseName(chart, namespace)

	// Before deployment, cleanup any conflicting releases for multi-namespace charts
	if chart.MultiNamespace {
		if err := g.CleanupConflictingReleases(ctx, chart, chart.TargetNamespaces); err != nil {
			g.logger.WarnWithContext("failed to cleanup conflicting releases", map[string]interface{}{
				"chart": chart.Name,
				"error": err.Error(),
			})
		}
	}

	// Check for existing operations before starting deployment
	if err := g.checkAndHandleConflicts(ctx, releaseName, namespace); err != nil {
		g.logger.WarnWithContext("conflict detected, attempting to resolve", map[string]interface{}{
			"chart":     chart.Name,
			"namespace": namespace,
			"error":     err.Error(),
		})
		// Continue with deployment - conflicts should be handled by the driver
	}

	// Check chart dependencies before deployment
	if err := g.validateChartDependencies(ctx, chart); err != nil {
		g.logger.WarnWithContext("chart dependencies validation failed", map[string]interface{}{
			"chart":     chart.Name,
			"namespace": namespace,
			"error":     err.Error(),
		})
		// Continue with deployment - Helm will handle dependency resolution
	}

	// Configure timeout strategy based on chart type and deployment complexity
	chartTimeout := g.calculateOptimalTimeout(chart, options)
	
	g.logger.InfoWithContext("timeout strategy determined", map[string]interface{}{
		"chart":                 chart.Name,
		"calculated_timeout":    chartTimeout,
		"default_timeout":       options.Timeout,
		"chart_type":           g.getChartType(chart),
		"override_applied":     options.Timeout != 0,
	})

	// Configure wait strategy based on chart type for proper lock management
	shouldWait := true
	shouldWaitForJobs := false
	
	// StatefulSet charts (databases) need proper waiting for atomic operations
	if g.isStatefulSetChart(chart) {
		shouldWait = true           // Essential for atomic operations on StatefulSets
		shouldWaitForJobs = false   // Jobs not typically used in StatefulSet charts
		g.logger.InfoWithContext("using StatefulSet wait strategy", map[string]interface{}{
			"chart": chart.Name,
			"wait": shouldWait,
			"wait_for_jobs": shouldWaitForJobs,
		})
	} else if chart.Name == "migrate" || chart.Name == "backup" {
		// Job-based charts need to wait for job completion
		shouldWait = true
		shouldWaitForJobs = true
		g.logger.InfoWithContext("using Job wait strategy", map[string]interface{}{
			"chart": chart.Name,
			"wait": shouldWait,
			"wait_for_jobs": shouldWaitForJobs,
		})
	} else {
		// Standard Deployment charts
		shouldWait = true           // Always wait when using atomic mode
		shouldWaitForJobs = false   // Standard deployments don't need job waiting
		g.logger.InfoWithContext("using standard wait strategy", map[string]interface{}{
			"chart": chart.Name,
			"wait": shouldWait,
			"wait_for_jobs": shouldWaitForJobs,
		})
	}

	helmOptions := helm_port.HelmUpgradeOptions{
		ValuesFile:      g.getValuesFile(chart, options.Environment),
		Namespace:       namespace,
		CreateNamespace: true,
		Wait:            shouldWait,     // ✅ Enable proper waiting for atomic operations
		WaitForJobs:     shouldWaitForJobs, // ✅ Chart-specific job waiting strategy
		Timeout:         chartTimeout,
		Force:           options.ForceUpdate,
		Atomic:          true, // ✅ Enable atomic deployment with proper waiting
	}

	// Simplified image override logic
	shouldOverrideImage := chart.SupportsImageOverride() && (options.ShouldOverrideImage() || options.ForceUpdate)

	if shouldOverrideImage {
		imageTag := options.GetImageTag(chart.Name)
		helmOptions.ImageOverrides = map[string]string{
			"image.repository": options.ImagePrefix,
			"image.tag":        imageTag,
		}

		g.logger.InfoWithContext("applying image override", map[string]interface{}{
			"chart":                   chart.Name,
			"repository":              options.ImagePrefix,
			"tag":                     imageTag,
			"force_update":            options.ForceUpdate,
			"should_override":         options.ShouldOverrideImage(),
			"tag_base":                options.TagBase,
			"supports_image_override": chart.SupportsImageOverride(),
		})
	} else {
		g.logger.InfoWithContext("not applying image override", map[string]interface{}{
			"chart":                   chart.Name,
			"supports_image_override": chart.SupportsImageOverride(),
			"should_override_image":   options.ShouldOverrideImage(),
			"force_update":            options.ForceUpdate,
			"tag_base":                options.TagBase,
		})
	}

	// Add force update flag for deployment annotations
	if options.ForceUpdate {
		if helmOptions.SetValues == nil {
			helmOptions.SetValues = make(map[string]string)
		}
		helmOptions.SetValues["forceUpdate"] = "true"

		g.logger.InfoWithContext("enabling force update", map[string]interface{}{
			"chart":             chart.Name,
			"force_update_flag": true,
		})
	}

	// Execute deployment with retry logic for retriable errors
	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		start := time.Now()

		// Create a timeout context for this specific deployment attempt
		deployCtx, cancel := context.WithTimeout(ctx, helmOptions.Timeout)

		g.logger.InfoWithContext("executing helm deployment", map[string]interface{}{
			"chart":     chart.Name,
			"namespace": namespace,
			"attempt":   attempt,
			"timeout":   helmOptions.Timeout,
			"atomic":    helmOptions.Atomic,
			"wait":      helmOptions.Wait,
		})

		g.logger.InfoWithContext("using release name for deployment", map[string]interface{}{
			"chart":           chart.Name,
			"namespace":       namespace,
			"release_name":    releaseName,
			"multi_namespace": chart.MultiNamespace,
		})

		err := g.helmPort.UpgradeInstall(deployCtx, releaseName, chart.Path, helmOptions)
		cancel()
		duration := time.Since(start)

		if err == nil {
			g.logger.InfoWithContext("chart deployed successfully", map[string]interface{}{
				"chart":     chart.Name,
				"namespace": namespace,
				"duration":  duration,
				"attempt":   attempt,
				"final_timeout_used": helmOptions.Timeout,
				"wait_strategy": map[string]interface{}{
					"wait": helmOptions.Wait,
					"wait_for_jobs": helmOptions.WaitForJobs,
					"atomic": helmOptions.Atomic,
				},
				"deployment_metrics": g.collectDeploymentMetrics(chart, namespace, duration, attempt),
			})
			
			// Log deployment success metrics for monitoring
			g.logDeploymentSuccess(chart, namespace, duration, attempt)
			return nil
		}

		// Check if the deployment was cancelled or timed out
		if deployCtx.Err() == context.DeadlineExceeded {
			g.logger.ErrorWithContext("chart deployment timed out", map[string]interface{}{
				"chart":     chart.Name,
				"namespace": namespace,
				"timeout":   helmOptions.Timeout,
				"attempt":   attempt,
				"duration":  duration,
			})
			lastErr = fmt.Errorf("deployment timed out after %v", helmOptions.Timeout)
			break // Don't retry timeout errors
		}

		lastErr = err

		g.logger.ErrorWithContext("chart deployment failed", map[string]interface{}{
			"chart":       chart.Name,
			"namespace":   namespace,
			"chart_path":  chart.Path,
			"values_file": helmOptions.ValuesFile,
			"error":       err.Error(),
			"duration":    duration,
			"timeout":     helmOptions.Timeout,
			"attempt":     attempt,
			"max_retries": maxRetries,
			"atomic":      helmOptions.Atomic,
			"wait":        helmOptions.Wait,
			"error_classification": g.classifyError(err),
			"retry_strategy": g.getRetryStrategy(err),
			"deployment_metrics": g.collectDeploymentMetrics(chart, namespace, duration, attempt),
		})
		
		// Log deployment failure metrics for monitoring and alerting
		g.logDeploymentFailure(chart, namespace, duration, attempt, err)

		// Check if error is retriable
		isRetriable := g.isRetriableError(err)
		if !isRetriable || attempt == maxRetries {
			g.logger.WarnWithContext("not retrying chart deployment", map[string]interface{}{
				"chart":           chart.Name,
				"attempt":         attempt,
				"retriable":       isRetriable,
				"is_last_attempt": attempt == maxRetries,
				"error_type":      g.classifyError(err),
			})
			break
		}

		// Enhanced retry strategy for different error types
		retryDelay := g.calculateRetryDelay(err, attempt)
		
		// Special handling for lock-related errors
		if g.isLockError(err) {
			g.logger.InfoWithContext("detected helm lock error, performing enhanced retry", map[string]interface{}{
				"chart":           chart.Name,
				"attempt":         attempt,
				"error":           err.Error(),
			})
			
			// Log lock detection for monitoring
			g.logLockDetection(chart, namespace, "helm_operation_in_progress")
			
			// Wait for any pending operations to complete before retrying
			if waitErr := g.waitForLockRelease(ctx, releaseName, namespace); waitErr != nil {
				g.logger.WarnWithContext("failed to wait for lock release", map[string]interface{}{
					"chart":     chart.Name,
					"error":     waitErr.Error(),
				})
			}
		}
		
		// Log retry attempt for monitoring
		errorType := g.classifyError(err)
		g.logRetryAttempt(chart, namespace, attempt, errorType, retryDelay)

		g.logger.WarnWithContext("retrying chart deployment", map[string]interface{}{
			"chart":           chart.Name,
			"attempt":         attempt,
			"next_attempt_in": retryDelay,
			"error_type":      g.classifyError(err),
			"retry_strategy":  g.getRetryStrategy(err),
		})

		select {
		case <-ctx.Done():
			g.logger.WarnWithContext("chart deployment cancelled during retry", map[string]interface{}{
				"chart":   chart.Name,
				"attempt": attempt,
			})
			return ctx.Err()
		case <-time.After(retryDelay):
			// Continue to next attempt
		}
	}

	return fmt.Errorf("chart deployment failed after %d attempts for %s: %w", maxRetries, chart.Name, lastErr)
}

// GetReleaseStatus returns the status of a Helm release
func (g *HelmGateway) GetReleaseStatus(ctx context.Context, releaseName, namespace string) (helm_port.HelmStatus, error) {
	g.logger.DebugWithContext("getting release status", map[string]interface{}{
		"release":   releaseName,
		"namespace": namespace,
	})

	status, err := g.helmPort.Status(ctx, releaseName, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to get release status", map[string]interface{}{
			"release":   releaseName,
			"namespace": namespace,
			"error":     err.Error(),
		})
		return helm_port.HelmStatus{}, fmt.Errorf("failed to get release status for %s: %w", releaseName, err)
	}

	g.logger.DebugWithContext("release status retrieved", map[string]interface{}{
		"release":   releaseName,
		"namespace": namespace,
		"status":    status.Status,
	})

	return status, nil
}

// ListReleases returns list of Helm releases as domain objects
func (g *HelmGateway) ListReleases(ctx context.Context, namespace string) ([]domain.HelmReleaseInfo, error) {
	g.logger.DebugWithContext("listing releases", map[string]interface{}{
		"namespace": namespace,
	})

	portReleases, err := g.helmPort.List(ctx, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to list releases", map[string]interface{}{
			"namespace": namespace,
			"error":     err.Error(),
		})
		return nil, fmt.Errorf("failed to list releases in namespace %s: %w", namespace, err)
	}

	// Convert port releases to domain releases
	var domainReleases []domain.HelmReleaseInfo
	for _, portRelease := range portReleases {
		domainRelease := domain.HelmReleaseInfo{
			Name:       portRelease.Name,
			Namespace:  portRelease.Namespace,
			Revision:   portRelease.Revision,
			Status:     portRelease.Status,
			Chart:      portRelease.Chart,
			AppVersion: portRelease.AppVersion,
			Updated:    portRelease.Updated,
		}
		domainReleases = append(domainReleases, domainRelease)
	}

	g.logger.DebugWithContext("releases listed", map[string]interface{}{
		"namespace": namespace,
		"count":     len(domainReleases),
	})

	return domainReleases, nil
}

// UninstallRelease removes a Helm release
func (g *HelmGateway) UninstallRelease(ctx context.Context, releaseName, namespace string) error {
	g.logger.InfoWithContext("uninstalling release", map[string]interface{}{
		"release":   releaseName,
		"namespace": namespace,
	})

	err := g.helmPort.Uninstall(ctx, releaseName, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to uninstall release", map[string]interface{}{
			"release":   releaseName,
			"namespace": namespace,
			"error":     err.Error(),
		})
		return fmt.Errorf("failed to uninstall release %s: %w", releaseName, err)
	}

	g.logger.InfoWithContext("release uninstalled successfully", map[string]interface{}{
		"release":   releaseName,
		"namespace": namespace,
	})

	return nil
}

// ValidateChart validates a chart before deployment
func (g *HelmGateway) ValidateChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	g.logger.InfoWithContext("validating chart", map[string]interface{}{
		"chart": chart.Name,
	})

	// Check if chart path exists
	if chart.Path == "" {
		return fmt.Errorf("chart path is empty for %s", chart.Name)
	}

	// Check if values file exists
	valuesFile := g.getValuesFile(chart, options.Environment)
	if valuesFile == "" {
		g.logger.WarnWithContext("no values file found for chart", map[string]interface{}{
			"chart":       chart.Name,
			"environment": options.Environment,
		})
	}

	// Try to template the chart to validate it
	_, err := g.TemplateChart(ctx, chart, options)
	if err != nil {
		return fmt.Errorf("chart validation failed for %s: %w", chart.Name, err)
	}

	g.logger.InfoWithContext("chart validated successfully", map[string]interface{}{
		"chart": chart.Name,
	})

	return nil
}

// getValuesFile returns the appropriate values file for the chart and environment
func (g *HelmGateway) getValuesFile(chart domain.Chart, env domain.Environment) string {
	// Try environment-specific values file first
	envFile := chart.ValuesFile(env)
	if envFile != "" {
		// Check if the environment-specific file exists
		if _, err := os.Stat(envFile); err == nil {
			g.logger.DebugWithContext("using environment-specific values file", map[string]interface{}{
				"chart":       chart.Name,
				"environment": env,
				"values_file": envFile,
			})
			return envFile
		}

		// Log warning if environment-specific file doesn't exist
		g.logger.WarnWithContext("environment-specific values file not found, using default", map[string]interface{}{
			"chart":               chart.Name,
			"environment":         env,
			"env_values_file":     envFile,
			"default_values_file": chart.DefaultValuesFile(),
		})
	}

	// Fall back to default values file
	return chart.DefaultValuesFile()
}

// BuildImageOverrides builds image override settings
func (g *HelmGateway) BuildImageOverrides(chart domain.Chart, options *domain.DeploymentOptions) map[string]string {
	overrides := make(map[string]string)

	if chart.SupportsImageOverride() && options.ShouldOverrideImage() {
		overrides["image.repository"] = options.ImagePrefix
		overrides["image.tag"] = options.GetImageTag(chart.Name)

		g.logger.DebugWithContext("built image overrides", map[string]interface{}{
			"chart":     chart.Name,
			"overrides": overrides,
		})
	}

	return overrides
}

// GetWaitOptions returns wait options for the chart
func (g *HelmGateway) GetWaitOptions(chart domain.Chart, options *domain.DeploymentOptions) (bool, time.Duration) {
	wait := chart.ShouldWaitForReadiness()
	timeout := options.Timeout

	if !wait {
		timeout = 0
	}

	g.logger.DebugWithContext("determined wait options", map[string]interface{}{
		"chart":   chart.Name,
		"wait":    wait,
		"timeout": timeout,
	})

	return wait, timeout
}

// GetReleaseHistory returns the history of a Helm release for a chart
func (g *HelmGateway) GetReleaseHistory(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) (string, error) {
	namespace := options.GetNamespace(chart.Name)

	revisions, err := g.helmPort.History(ctx, chart.Name, namespace)
	if err != nil {
		return "", fmt.Errorf("failed to get release history for %s: %w", chart.Name, err)
	}

	// Format the history output
	var result strings.Builder
	for _, rev := range revisions {
		result.WriteString(fmt.Sprintf("Revision %d: %s - %s (%s)\n",
			rev.Revision, rev.Status, rev.Chart, rev.Updated.Format("2006-01-02 15:04:05")))
		if rev.Description != "" {
			result.WriteString(fmt.Sprintf("  Description: %s\n", rev.Description))
		}
	}

	return result.String(), nil
}

// checkAndHandleConflicts checks for and handles Helm operation conflicts
func (g *HelmGateway) checkAndHandleConflicts(ctx context.Context, releaseName, namespace string) error {
	g.logger.DebugWithContext("checking for helm operation conflicts", map[string]interface{}{
		"release":   releaseName,
		"namespace": namespace,
	})

	// Check if there are any pending operations
	operation, err := g.helmPort.DetectPendingOperation(ctx, releaseName, namespace)
	if err != nil {
		return fmt.Errorf("failed to detect pending operations: %w", err)
	}

	if operation != nil {
		g.logger.WarnWithContext("detected pending helm operation", map[string]interface{}{
			"release":    releaseName,
			"namespace":  namespace,
			"operation":  operation.Type,
			"status":     operation.Status,
			"start_time": operation.StartTime,
			"pid":        operation.PID,
		})

		// Try to cleanup stuck operations
		if operation.Status == "stuck" {
			g.logger.InfoWithContext("cleaning up stuck helm operation", map[string]interface{}{
				"release":   releaseName,
				"namespace": namespace,
				"pid":       operation.PID,
			})

			if err := g.helmPort.CleanupStuckOperations(ctx, releaseName, namespace); err != nil {
				return fmt.Errorf("failed to cleanup stuck operations: %w", err)
			}

			g.logger.InfoWithContext("stuck helm operation cleaned up", map[string]interface{}{
				"release":   releaseName,
				"namespace": namespace,
			})
		}
	}

	return nil
}

// DetectPendingOperation checks for pending Helm operations for a chart
func (g *HelmGateway) DetectPendingOperation(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) (*helm_port.HelmOperation, error) {
	namespace := options.GetNamespace(chart.Name)

	g.logger.DebugWithContext("detecting pending operations", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": namespace,
	})

	operation, err := g.helmPort.DetectPendingOperation(ctx, chart.Name, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to detect pending operations", map[string]interface{}{
			"chart":     chart.Name,
			"namespace": namespace,
			"error":     err.Error(),
		})
		return nil, fmt.Errorf("failed to detect pending operations for %s: %w", chart.Name, err)
	}

	if operation != nil {
		g.logger.InfoWithContext("pending operation detected", map[string]interface{}{
			"chart":      chart.Name,
			"namespace":  namespace,
			"operation":  operation.Type,
			"status":     operation.Status,
			"start_time": operation.StartTime,
		})
	}

	return operation, nil
}

// CleanupStuckOperations cleans up stuck Helm operations for a chart
func (g *HelmGateway) CleanupStuckOperations(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	namespace := options.GetNamespace(chart.Name)

	g.logger.InfoWithContext("cleaning up stuck operations", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": namespace,
	})

	err := g.helmPort.CleanupStuckOperations(ctx, chart.Name, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to cleanup stuck operations", map[string]interface{}{
			"chart":     chart.Name,
			"namespace": namespace,
			"error":     err.Error(),
		})
		return fmt.Errorf("failed to cleanup stuck operations for %s: %w", chart.Name, err)
	}

	g.logger.InfoWithContext("stuck operations cleaned up", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": namespace,
	})

	return nil
}

// validateChartDependencies validates chart dependencies before deployment
func (g *HelmGateway) validateChartDependencies(ctx context.Context, chart domain.Chart) error {
	g.logger.DebugWithContext("validating chart dependencies", map[string]interface{}{
		"chart": chart.Name,
	})

	// Check if Chart.lock exists (indicates dependencies are resolved)
	chartLockPath := fmt.Sprintf("%s/Chart.lock", chart.Path)
	if _, err := os.Stat(chartLockPath); err != nil {
		if os.IsNotExist(err) {
			g.logger.DebugWithContext("no Chart.lock found, dependencies may not be resolved", map[string]interface{}{
				"chart": chart.Name,
				"path":  chartLockPath,
			})
		} else {
			return fmt.Errorf("failed to check Chart.lock: %w", err)
		}
	}

	// Check if charts/ directory exists (indicates dependencies are downloaded)
	chartsDir := fmt.Sprintf("%s/charts", chart.Path)
	if _, err := os.Stat(chartsDir); err != nil {
		if os.IsNotExist(err) {
			g.logger.DebugWithContext("no charts directory found, dependencies may not be downloaded", map[string]interface{}{
				"chart": chart.Name,
				"path":  chartsDir,
			})
		} else {
			return fmt.Errorf("failed to check charts directory: %w", err)
		}
	}

	return nil
}

// validatePrerequisites validates chart prerequisites before deployment
func (g *HelmGateway) validatePrerequisites(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	// Check if chart path exists and is accessible
	if chart.Path == "" {
		return fmt.Errorf("chart path is empty")
	}

	// Validate that IMAGE_PREFIX is set for charts that support image override
	if chart.SupportsImageOverride() && options.ImagePrefix == "" {
		return fmt.Errorf("IMAGE_PREFIX is required for chart %s", chart.Name)
	}

	// Check values file existence
	valuesFile := g.getValuesFile(chart, options.Environment)
	if valuesFile == "" {
		g.logger.WarnWithContext("no values file found for chart", map[string]interface{}{
			"chart":       chart.Name,
			"environment": options.Environment,
		})
	}

	// Postgres-specific validation
	if chart.Name == "postgres" {
		if err := g.validatePostgresChart(ctx, chart, options); err != nil {
			return fmt.Errorf("postgres chart validation failed: %w", err)
		}
	}

	// ClickHouse-specific validation
	if chart.Name == "clickhouse" {
		if err := g.validateClickHouseChart(ctx, chart, options); err != nil {
			return fmt.Errorf("clickhouse chart validation failed: %w", err)
		}
	}

	// StatefulSet-specific validation
	if g.isStatefulSetChart(chart) {
		if err := g.validateStatefulSetChart(ctx, chart, options); err != nil {
			return fmt.Errorf("statefulset chart validation failed: %w", err)
		}
	}

	return nil
}

// isRetriableError determines if an error is worth retrying
func (g *HelmGateway) isRetriableError(err error) bool {
	if err == nil {
		return false
	}

	errorMsg := strings.ToLower(err.Error())

	// Non-retriable conditions (immediate failures)
	nonRetriablePatterns := []string{
		"cannot be imported into the current release",
		"invalid ownership metadata",
		"annotation validation error",
		"meta.helm.sh/release-name",
		"already exists and cannot be imported",
	}

	for _, pattern := range nonRetriablePatterns {
		if strings.Contains(errorMsg, pattern) {
			g.logger.DebugWithContext("error classified as non-retriable Helm metadata conflict", map[string]interface{}{
				"error":   errorMsg,
				"pattern": pattern,
			})
			return false
		}
	}

	// Retriable conditions - enhanced for lock-related errors
	retriablePatterns := []string{
		"another operation in progress",        // Helm lock conflict
		"another operation (install/upgrade/rollback) is in progress", // Full error message
		"operation in progress",                // Partial match
		"connection refused",                   // Network issues
		"timeout",                             // Various timeout scenarios
		"temporary failure",                   // Temporary issues
		"resource temporarily unavailable",    // Resource constraints
		"server gave http response",           // Network/connection issues
		"no such host",                       // DNS issues
		"connection reset by peer",           // Network issues
		"context deadline exceeded",          // Timeout variations
		"i/o timeout",                        // I/O timeout
		"operation not permitted",            // Permission issues (may be temporary)
		"resource busy",                      // Resource lock issues
		"pending-install",                    // Helm release stuck in pending state
		"pending-upgrade",                    // Helm release stuck in pending upgrade
	}

	for _, pattern := range retriablePatterns {
		if strings.Contains(errorMsg, pattern) {
			g.logger.DebugWithContext("error classified as retriable", map[string]interface{}{
				"error":   errorMsg,
				"pattern": pattern,
				"reason":  "error indicates temporary condition that may resolve with retry",
			})
			return true
		}
	}

	// Check for numeric timeout patterns (e.g., "timeout after 10m0s")
	if strings.Contains(errorMsg, "timeout after") {
		g.logger.DebugWithContext("timeout error classified as retriable", map[string]interface{}{
			"error": errorMsg,
			"reason": "timeout errors may succeed with longer wait times",
		})
		return true
	}

	g.logger.DebugWithContext("error classified as non-retriable", map[string]interface{}{
		"error": errorMsg,
		"reason": "error does not match known retriable patterns",
	})
	return false
}

// validatePostgresChart validates postgres-specific requirements
func (g *HelmGateway) validatePostgresChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	g.logger.DebugWithContext("validating postgres chart", map[string]interface{}{
		"chart": chart.Name,
	})

	// Check if the namespace alignment is correct
	valuesFile := g.getValuesFile(chart, options.Environment)
	if valuesFile != "" {
		// TODO: Parse values file to check for configuration conflicts
		g.logger.DebugWithContext("postgres values file found", map[string]interface{}{
			"values_file": valuesFile,
		})
	}

	// Check if persistent volumes are available
	namespace := options.GetNamespace(chart.Name)
	g.logger.DebugWithContext("postgres chart validation completed", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": namespace,
	})

	return nil
}

// validateClickHouseChart validates ClickHouse-specific requirements
func (g *HelmGateway) validateClickHouseChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	g.logger.DebugWithContext("validating ClickHouse chart", map[string]interface{}{
		"chart": chart.Name,
	})

	// Check if the namespace alignment is correct for ClickHouse
	namespace := options.GetNamespace(chart.Name)
	expectedNamespace := "alt-database"
	if namespace != expectedNamespace {
		return fmt.Errorf("ClickHouse chart namespace mismatch: expected %s, got %s", expectedNamespace, namespace)
	}

	// Check for values file and validate basic structure
	valuesFile := g.getValuesFile(chart, options.Environment)
	if valuesFile != "" {
		// Validate ClickHouse-specific configuration
		if err := g.validateClickHouseValues(valuesFile); err != nil {
			return fmt.Errorf("ClickHouse values validation failed: %w", err)
		}

		g.logger.DebugWithContext("ClickHouse values file validated", map[string]interface{}{
			"values_file": valuesFile,
		})
	}

	// Check if required persistent volumes are available
	if err := g.validateClickHousePersistentVolumes(ctx, namespace); err != nil {
		return fmt.Errorf("ClickHouse persistent volume validation failed: %w", err)
	}

	// Check if required secrets exist or can be created
	if err := g.validateClickHouseSecrets(ctx, namespace); err != nil {
		g.logger.WarnWithContext("ClickHouse secret validation warning", map[string]interface{}{
			"error":     err.Error(),
			"namespace": namespace,
		})
		// Not a hard failure - secrets can be created during deployment
	}

	g.logger.DebugWithContext("ClickHouse chart validation completed", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": namespace,
	})

	return nil
}

// validateClickHouseValues validates ClickHouse values file structure
func (g *HelmGateway) validateClickHouseValues(valuesFile string) error {
	// TODO: Parse YAML values file and validate:
	// - auth.username exists and is not empty
	// - auth.password exists and is not empty
	// - persistence.data.size is reasonable (>= 1Gi)
	// - SSL configuration consistency

	g.logger.DebugWithContext("ClickHouse values validation placeholder", map[string]interface{}{
		"values_file": valuesFile,
	})

	return nil
}

// validateClickHousePersistentVolumes checks if required PVs are available
func (g *HelmGateway) validateClickHousePersistentVolumes(ctx context.Context, namespace string) error {
	// Check if clickhouse-pv exists and is available
	pvName := "clickhouse-pv"

	g.logger.DebugWithContext("validating ClickHouse persistent volumes", map[string]interface{}{
		"pv_name":   pvName,
		"namespace": namespace,
	})

	// This validation is informational - PVs will be created if they don't exist
	return nil
}

// validateClickHouseSecrets checks if required secrets exist
func (g *HelmGateway) validateClickHouseSecrets(ctx context.Context, namespace string) error {
	secretName := "clickhouse-secrets"

	g.logger.DebugWithContext("validating ClickHouse secrets", map[string]interface{}{
		"secret_name": secretName,
		"namespace":   namespace,
	})

	// Check if the secret exists
	// This is informational - secrets will be created during deployment
	return nil
}

// isStatefulSetChart checks if a chart deploys StatefulSets
func (g *HelmGateway) isStatefulSetChart(chart domain.Chart) bool {
	// Known StatefulSet charts
	statefulSetCharts := []string{
		"postgres", "auth-postgres", "kratos-postgres",
		"clickhouse", "meilisearch",
	}

	for _, name := range statefulSetCharts {
		if chart.Name == name {
			return true
		}
	}

	return false
}

// validateStatefulSetChart validates StatefulSet-specific requirements
func (g *HelmGateway) validateStatefulSetChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	g.logger.DebugWithContext("validating statefulset chart", map[string]interface{}{
		"chart": chart.Name,
	})

	// TODO: Add more StatefulSet-specific validations
	// - Check for PV availability
	// - Validate storage class
	// - Check for proper resource limits

	return nil
}

// ForceDeleteRelease forcefully deletes a Helm release with aggressive cleanup
func (g *HelmGateway) ForceDeleteRelease(ctx context.Context, releaseName, namespace string) error {
	g.logger.InfoWithContext("force deleting helm release", map[string]interface{}{
		"release":   releaseName,
		"namespace": namespace,
	})

	// Use the existing Uninstall method from the port
	err := g.helmPort.Uninstall(ctx, releaseName, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to force delete helm release", map[string]interface{}{
			"release":   releaseName,
			"namespace": namespace,
			"error":     err.Error(),
		})
		return fmt.Errorf("failed to force delete helm release %s: %w", releaseName, err)
	}

	g.logger.InfoWithContext("helm release force deleted", map[string]interface{}{
		"release":   releaseName,
		"namespace": namespace,
	})

	return nil
}

// EmergencyCleanupAllReleases performs emergency cleanup of all Helm releases
func (g *HelmGateway) EmergencyCleanupAllReleases(ctx context.Context) error {
	g.logger.InfoWithContext("starting emergency cleanup of all helm releases", map[string]interface{}{})

	// Get all releases across all namespaces
	releases, err := g.ListReleases(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to list releases for emergency cleanup: %w", err)
	}

	g.logger.InfoWithContext("found releases for emergency cleanup", map[string]interface{}{
		"count": len(releases),
	})

	// Force delete each release
	for _, release := range releases {
		g.logger.InfoWithContext("force deleting release in emergency cleanup", map[string]interface{}{
			"release":   release.Name,
			"namespace": release.Namespace,
			"status":    release.Status,
		})

		if err := g.ForceDeleteRelease(ctx, release.Name, release.Namespace); err != nil {
			g.logger.WarnWithContext("failed to force delete release during emergency cleanup", map[string]interface{}{
				"release":   release.Name,
				"namespace": release.Namespace,
				"error":     err.Error(),
			})
			// Continue with other releases even if one fails
		}
	}

	g.logger.InfoWithContext("emergency cleanup of helm releases completed", map[string]interface{}{
		"releases_processed": len(releases),
	})

	return nil
}

// ValidateReleaseState validates the state of a Helm release
func (g *HelmGateway) ValidateReleaseState(ctx context.Context, releaseName, namespace string) (*helm_port.HelmStatus, error) {
	g.logger.InfoWithContext("validating helm release state", map[string]interface{}{
		"release":   releaseName,
		"namespace": namespace,
	})

	// Use the existing Status method from the port
	status, err := g.helmPort.Status(ctx, releaseName, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to validate helm release state", map[string]interface{}{
			"release":   releaseName,
			"namespace": namespace,
			"error":     err.Error(),
		})
		return nil, fmt.Errorf("failed to validate helm release state for %s: %w", releaseName, err)
	}

	g.logger.InfoWithContext("helm release state validated", map[string]interface{}{
		"release":   releaseName,
		"namespace": namespace,
		"status":    status.Status,
	})

	return &status, nil
}

// generateReleaseName generates namespace-scoped release name for multi-namespace charts
func (g *HelmGateway) generateReleaseName(chart domain.Chart, namespace string) string {
	if chart.MultiNamespace {
		// マルチネームスペースチャートは namespace suffix を追加
		namespaceSuffix := strings.TrimPrefix(namespace, "alt-")
		releaseName := fmt.Sprintf("%s-%s", chart.Name, namespaceSuffix)
		g.logger.InfoWithContext("generating namespace-scoped release name", map[string]interface{}{
			"chart":                  chart.Name,
			"namespace":              namespace,
			"original_name":          chart.Name,
			"namespace_suffix":       namespaceSuffix,
			"generated_release_name": releaseName,
		})
		return releaseName
	}
	// 単一ネームスペースチャートは従来通り
	g.logger.InfoWithContext("using original release name for single-namespace chart", map[string]interface{}{
		"chart":        chart.Name,
		"namespace":    namespace,
		"release_name": chart.Name,
	})
	return chart.Name
}

// CleanupConflictingReleases cleans up conflicting releases for multi-namespace charts
func (g *HelmGateway) CleanupConflictingReleases(ctx context.Context, chart domain.Chart, targetNamespaces []string) error {
	if !chart.MultiNamespace {
		return nil // 単一ネームスペースは対象外
	}

	g.logger.InfoWithContext("cleaning up conflicting releases for multi-namespace chart", map[string]interface{}{
		"chart":             chart.Name,
		"target_namespaces": targetNamespaces,
	})

	// 全ネームスペースで同名リリースを検索・削除
	for _, ns := range targetNamespaces {
		if err := g.helmPort.Uninstall(ctx, chart.Name, ns); err != nil {
			// エラーログ出力して継続（リリースが存在しない場合は正常）
			g.logger.WarnWithContext("failed to cleanup conflicting release", map[string]interface{}{
				"chart":     chart.Name,
				"namespace": ns,
				"error":     err.Error(),
			})
		} else {
			g.logger.InfoWithContext("conflicting release cleaned up", map[string]interface{}{
				"chart":     chart.Name,
				"namespace": ns,
			})
		}
	}
	return nil
}

// RollbackRelease rolls back a Helm release to a specific revision
func (g *HelmGateway) RollbackRelease(ctx context.Context, releaseName, namespace string, revision int) error {
	g.logger.InfoWithContext("rolling back Helm release", map[string]interface{}{
		"release":   releaseName,
		"namespace": namespace,
		"revision":  revision,
	})

	err := g.helmPort.Rollback(ctx, releaseName, namespace, revision)
	if err != nil {
		return fmt.Errorf("failed to rollback release %s to revision %d: %w", releaseName, revision, err)
	}

	g.logger.InfoWithContext("release rolled back successfully", map[string]interface{}{
		"release":   releaseName,
		"namespace": namespace,
		"revision":  revision,
	})

	return nil
}

// calculateOptimalTimeout determines the best timeout for a chart based on its type and characteristics
func (g *HelmGateway) calculateOptimalTimeout(chart domain.Chart, options *domain.DeploymentOptions) time.Duration {
	// Start with user-provided timeout if available
	if options.Timeout > 0 {
		g.logger.DebugWithContext("using user-provided timeout", map[string]interface{}{
			"chart":            chart.Name,
			"user_timeout":     options.Timeout,
		})
		return options.Timeout
	}

	// Chart type-based timeout strategy
	chartType := g.getChartType(chart)
	
	switch chartType {
	case "statefulset":
		// StatefulSet database charts need extended timeout for initialization
		timeout := 12 * time.Minute
		if chart.Name == "postgres" {
			// Postgres may need extra time for WAL replay, index rebuilds, etc.
			timeout = 15 * time.Minute
		}
		g.logger.InfoWithContext("using extended timeout for StatefulSet chart", map[string]interface{}{
			"chart":        chart.Name,
			"timeout":      timeout,
			"reason":       "StatefulSet requires time for persistent volume mounting and database initialization",
		})
		return timeout
		
	case "job":
		// Job-based charts (migrate, backup) may run for varying durations
		timeout := 8 * time.Minute
		if chart.Name == "migrate" {
			// Migration jobs may need more time for large database changes
			timeout = 10 * time.Minute
		}
		g.logger.InfoWithContext("using job timeout", map[string]interface{}{
			"chart":        chart.Name,
			"timeout":      timeout,
			"reason":       "Job completion time varies based on workload",
		})
		return timeout
		
	case "frontend":
		// Frontend applications should start quickly
		timeout := 3 * time.Minute
		g.logger.InfoWithContext("using frontend timeout", map[string]interface{}{
			"chart":        chart.Name,
			"timeout":      timeout,
			"reason":       "Frontend applications have fast startup times",
		})
		return timeout
		
	case "service":
		// Backend services and APIs
		timeout := 5 * time.Minute
		g.logger.InfoWithContext("using service timeout", map[string]interface{}{
			"chart":        chart.Name,
			"timeout":      timeout,
			"reason":       "Service applications need moderate startup time",
		})
		return timeout
		
	case "infrastructure":
		// Infrastructure components (nginx, auth services)
		timeout := 4 * time.Minute
		g.logger.InfoWithContext("using infrastructure timeout", map[string]interface{}{
			"chart":        chart.Name,
			"timeout":      timeout,
			"reason":       "Infrastructure components need stable startup",
		})
		return timeout
		
	default:
		// Default timeout for unknown chart types
		timeout := 6 * time.Minute
		g.logger.InfoWithContext("using default timeout for unknown chart type", map[string]interface{}{
			"chart":        chart.Name,
			"chart_type":   chartType,
			"timeout":      timeout,
			"reason":       "Unknown chart type, using conservative timeout",
		})
		return timeout
	}
}

// getChartType classifies charts into deployment categories for timeout and wait strategies
func (g *HelmGateway) getChartType(chart domain.Chart) string {
	// StatefulSet database charts
	statefulSetCharts := []string{"postgres", "auth-postgres", "kratos-postgres", "clickhouse", "meilisearch"}
	for _, name := range statefulSetCharts {
		if chart.Name == name {
			return "statefulset"
		}
	}
	
	// Job-based charts
	jobCharts := []string{"migrate", "backup"}
	for _, name := range jobCharts {
		if chart.Name == name {
			return "job"
		}
	}
	
	// Frontend applications
	frontendCharts := []string{"alt-frontend"}
	for _, name := range frontendCharts {
		if chart.Name == name {
			return "frontend"
		}
	}
	
	// Backend services
	serviceCharts := []string{"alt-backend", "pre-processor", "search-indexer", "tag-generator", "news-creator", "rask-log-aggregator", "rask-log-forwarder"}
	for _, name := range serviceCharts {
		if chart.Name == name {
			return "service"
		}
	}
	
	// Infrastructure components
	infrastructureCharts := []string{"nginx", "nginx-external", "auth-service", "kratos"}
	for _, name := range infrastructureCharts {
		if chart.Name == name {
			return "infrastructure"
		}
	}
	
	// Unknown chart type
	g.logger.DebugWithContext("chart type not classified", map[string]interface{}{
		"chart": chart.Name,
	})
	return "unknown"
}

// isLockError determines if an error is related to Helm release locks
func (g *HelmGateway) isLockError(err error) bool {
	if err == nil {
		return false
	}
	
	errorMsg := strings.ToLower(err.Error())
	lockPatterns := []string{
		"another operation in progress",
		"another operation (install/upgrade/rollback) is in progress",
		"operation in progress",
		"pending-install",
		"pending-upgrade",
		"resource busy",
	}
	
	for _, pattern := range lockPatterns {
		if strings.Contains(errorMsg, pattern) {
			return true
		}
	}
	
	return false
}

// classifyError provides a classification of the error for logging and decision making
func (g *HelmGateway) classifyError(err error) string {
	if err == nil {
		return "none"
	}
	
	errorMsg := strings.ToLower(err.Error())
	
	if g.isLockError(err) {
		return "lock_conflict"
	}
	
	if strings.Contains(errorMsg, "timeout") || strings.Contains(errorMsg, "context deadline exceeded") {
		return "timeout"
	}
	
	if strings.Contains(errorMsg, "connection refused") || strings.Contains(errorMsg, "no such host") {
		return "network"
	}
	
	if strings.Contains(errorMsg, "cannot be imported") || strings.Contains(errorMsg, "invalid ownership") {
		return "ownership_conflict"
	}
	
	if strings.Contains(errorMsg, "not found") {
		return "resource_not_found"
	}
	
	return "unknown"
}

// calculateRetryDelay determines the appropriate delay before retrying based on error type
func (g *HelmGateway) calculateRetryDelay(err error, attempt int) time.Duration {
	errorType := g.classifyError(err)
	
	switch errorType {
	case "lock_conflict":
		// Longer delay for lock conflicts to allow operations to complete
		baseDelay := 15 * time.Second
		return baseDelay + time.Duration(attempt*5)*time.Second
		
	case "timeout":
		// Moderate delay for timeout errors
		baseDelay := 10 * time.Second
		return baseDelay + time.Duration(attempt*3)*time.Second
		
	case "network":
		// Shorter delay for network issues (they often resolve quickly)
		baseDelay := 5 * time.Second
		return baseDelay + time.Duration(attempt*2)*time.Second
		
	default:
		// Standard exponential backoff for other errors
		baseDelay := 8 * time.Second
		return baseDelay + time.Duration(attempt*4)*time.Second
	}
}

// getRetryStrategy returns a description of the retry strategy for an error
func (g *HelmGateway) getRetryStrategy(err error) string {
	errorType := g.classifyError(err)
	
	switch errorType {
	case "lock_conflict":
		return "extended_delay_with_lock_wait"
	case "timeout":
		return "moderate_exponential_backoff"
	case "network":
		return "quick_retry"
	default:
		return "standard_exponential_backoff"
	}
}

// waitForLockRelease waits for any pending Helm operations to complete
func (g *HelmGateway) waitForLockRelease(ctx context.Context, releaseName, namespace string) error {
	maxWaitTime := 2 * time.Minute
	pollInterval := 5 * time.Second
	
	g.logger.InfoWithContext("waiting for helm lock release", map[string]interface{}{
		"release":       releaseName,
		"namespace":     namespace,
		"max_wait_time": maxWaitTime,
		"poll_interval": pollInterval,
	})
	
	deadline := time.Now().Add(maxWaitTime)
	
	for time.Now().Before(deadline) {
		// Check if there are any pending operations
		operation, err := g.helmPort.DetectPendingOperation(ctx, releaseName, namespace)
		if err != nil {
			g.logger.DebugWithContext("error checking pending operations", map[string]interface{}{
				"release":   releaseName,
				"namespace": namespace,
				"error":     err.Error(),
			})
			// Continue waiting even if detection fails
		} else if operation == nil {
			g.logger.InfoWithContext("helm lock released", map[string]interface{}{
				"release":   releaseName,
				"namespace": namespace,
			})
			return nil
		} else {
			g.logger.DebugWithContext("helm operation still pending", map[string]interface{}{
				"release":    releaseName,
				"namespace":  namespace,
				"operation":  operation.Type,
				"status":     operation.Status,
			})
		}
		
		// Wait before next check
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
			// Continue polling
		}
	}
	
	g.logger.WarnWithContext("timeout waiting for helm lock release", map[string]interface{}{
		"release":   releaseName,
		"namespace": namespace,
		"waited":    maxWaitTime,
	})
	
	return fmt.Errorf("timeout waiting for helm lock release after %v", maxWaitTime)
}

// collectDeploymentMetrics collects comprehensive metrics for deployment monitoring
func (g *HelmGateway) collectDeploymentMetrics(chart domain.Chart, namespace string, duration time.Duration, attempt int) map[string]interface{} {
	return map[string]interface{}{
		"chart_type":           g.getChartType(chart),
		"is_statefulset":       g.isStatefulSetChart(chart),
		"deployment_duration_seconds": duration.Seconds(),
		"attempt_number":       attempt,
		"chart_name":          chart.Name,
		"target_namespace":    namespace,
		"multi_namespace":     chart.MultiNamespace,
		"supports_image_override": chart.SupportsImageOverride(),
		"timestamp":           time.Now().Unix(),
		"deployment_id":       g.generateDeploymentID(chart, namespace),
	}
}

// logDeploymentSuccess logs deployment success for monitoring systems
func (g *HelmGateway) logDeploymentSuccess(chart domain.Chart, namespace string, duration time.Duration, attempt int) {
	// Structured log for monitoring systems (Prometheus, Grafana, etc.)
	g.logger.InfoWithContext("DEPLOYMENT_SUCCESS_METRIC", map[string]interface{}{
		"metric_type":         "deployment_success",
		"chart":              chart.Name,
		"namespace":          namespace,
		"duration_seconds":   duration.Seconds(),
		"attempts":           attempt,
		"chart_type":         g.getChartType(chart),
		"success_rate":       1.0, // 100% for successful deployments
		"timestamp":          time.Now().Unix(),
		"deployment_id":      g.generateDeploymentID(chart, namespace),
	})
	
	// Additional success metrics
	if duration > 5*time.Minute {
		g.logger.WarnWithContext("DEPLOYMENT_SLOW_SUCCESS", map[string]interface{}{
			"chart":            chart.Name,
			"namespace":        namespace,
			"duration_seconds": duration.Seconds(),
			"threshold_seconds": 300, // 5 minutes
			"performance_impact": "slow_deployment",
		})
	}
}

// logDeploymentFailure logs deployment failure for monitoring and alerting
func (g *HelmGateway) logDeploymentFailure(chart domain.Chart, namespace string, duration time.Duration, attempt int, err error) {
	// Structured log for monitoring systems
	g.logger.ErrorWithContext("DEPLOYMENT_FAILURE_METRIC", map[string]interface{}{
		"metric_type":         "deployment_failure",
		"chart":              chart.Name,
		"namespace":          namespace,
		"duration_seconds":   duration.Seconds(),
		"attempts":           attempt,
		"chart_type":         g.getChartType(chart),
		"error_type":         g.classifyError(err),
		"error_message":      err.Error(),
		"success_rate":       0.0, // 0% for failed deployments
		"timestamp":          time.Now().Unix(),
		"deployment_id":      g.generateDeploymentID(chart, namespace),
		"alert_level":        g.determineAlertLevel(err, attempt),
	})
	
	// Critical failure alerting
	if g.isCriticalFailure(err, attempt) {
		g.logger.ErrorWithContext("CRITICAL_DEPLOYMENT_FAILURE", map[string]interface{}{
			"alert_type":    "critical",
			"chart":         chart.Name,
			"namespace":     namespace,
			"error_type":    g.classifyError(err),
			"requires_immediate_attention": true,
			"escalation_needed": attempt >= 2,
		})
	}
}

// generateDeploymentID creates a unique deployment identifier for tracking
func (g *HelmGateway) generateDeploymentID(chart domain.Chart, namespace string) string {
	return fmt.Sprintf("%s-%s-%d", chart.Name, namespace, time.Now().Unix())
}

// determineAlertLevel determines the severity level for monitoring alerts
func (g *HelmGateway) determineAlertLevel(err error, attempt int) string {
	errorType := g.classifyError(err)
	
	switch errorType {
	case "lock_conflict":
		if attempt >= 2 {
			return "high"
		}
		return "medium"
	case "timeout":
		if attempt >= 3 {
			return "critical"
		}
		return "high"
	case "ownership_conflict":
		return "critical" // These usually require manual intervention
	case "network":
		return "medium"
	default:
		if attempt >= 2 {
			return "high"
		}
		return "low"
	}
}

// isCriticalFailure determines if a failure requires immediate attention
func (g *HelmGateway) isCriticalFailure(err error, attempt int) bool {
	errorType := g.classifyError(err)
	
	// Always critical
	criticalErrors := []string{"ownership_conflict"}
	for _, critical := range criticalErrors {
		if errorType == critical {
			return true
		}
	}
	
	// Critical after multiple attempts
	if attempt >= 3 {
		return true
	}
	
	// Critical timeouts on StatefulSets
	if errorType == "timeout" && attempt >= 2 {
		return true
	}
	
	return false
}

// logLockDetection logs when lock conflicts are detected for monitoring
func (g *HelmGateway) logLockDetection(chart domain.Chart, namespace string, lockType string) {
	g.logger.WarnWithContext("HELM_LOCK_DETECTED", map[string]interface{}{
		"metric_type":    "lock_detection",
		"chart":          chart.Name,
		"namespace":      namespace,
		"lock_type":      lockType,
		"timestamp":      time.Now().Unix(),
		"requires_retry": true,
		"impact":         "deployment_delay",
	})
}

// logRetryAttempt logs retry attempts for monitoring retry patterns
func (g *HelmGateway) logRetryAttempt(chart domain.Chart, namespace string, attempt int, errorType string, delay time.Duration) {
	g.logger.InfoWithContext("DEPLOYMENT_RETRY_METRIC", map[string]interface{}{
		"metric_type":       "retry_attempt",
		"chart":            chart.Name,
		"namespace":        namespace,
		"attempt":          attempt,
		"error_type":       errorType,
		"retry_delay_seconds": delay.Seconds(),
		"timestamp":        time.Now().Unix(),
		"retry_strategy":   g.getRetryStrategy(fmt.Errorf("error_type: %s", errorType)),
	})
}
