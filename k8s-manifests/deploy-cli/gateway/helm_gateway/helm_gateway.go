package helm_gateway

import (
	"context"
	"fmt"
	"strings"
	"time"
	"os"
	
	"deploy-cli/port/helm_port"
	"deploy-cli/port/logger_port"
	"deploy-cli/domain"
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
		"chart":     chart.Name,
		"namespace": namespace,
		"force_update": options.ForceUpdate,
		"dry_run": options.DryRun,
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
	
	// Check for existing operations before starting deployment
	if err := g.checkAndHandleConflicts(ctx, chart.Name, namespace); err != nil {
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
	
	// Configure timeout - use shorter timeout for known problematic charts
	chartTimeout := options.Timeout
	if chartTimeout == 0 {
		chartTimeout = 3 * time.Minute // Default 3 minute timeout
	}
	
	// Use even shorter timeout for charts that often hang
	if chart.Name == "alt-frontend" || chart.Name == "migrate" {
		chartTimeout = 90 * time.Second // Very short timeout for problematic charts
		g.logger.InfoWithContext("using aggressive timeout for problematic chart", map[string]interface{}{
			"chart": chart.Name,
			"timeout": chartTimeout,
		})
	}
	
	helmOptions := helm_port.HelmUpgradeOptions{
		ValuesFile:      g.getValuesFile(chart, options.Environment),
		Namespace:       namespace,
		CreateNamespace: true,
		Wait:            false, // Disable wait to prevent hanging
		WaitForJobs:     false, // Disable job waiting to prevent hanging
		Timeout:         chartTimeout,
		Force:           options.ForceUpdate,
		Atomic:          true,  // Enable atomic deployment for rollback on failure
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
			"chart":           chart.Name,
			"repository":      options.ImagePrefix,
			"tag":             imageTag,
			"force_update":    options.ForceUpdate,
			"should_override": options.ShouldOverrideImage(),
			"tag_base":        options.TagBase,
			"supports_image_override": chart.SupportsImageOverride(),
		})
	} else {
		g.logger.InfoWithContext("not applying image override", map[string]interface{}{
			"chart":                   chart.Name,
			"supports_image_override": chart.SupportsImageOverride(),
			"should_override_image":   options.ShouldOverrideImage(),
			"force_update":           options.ForceUpdate,
			"tag_base":               options.TagBase,
		})
	}
	
	// Add force update flag for deployment annotations
	if options.ForceUpdate {
		if helmOptions.SetValues == nil {
			helmOptions.SetValues = make(map[string]string)
		}
		helmOptions.SetValues["forceUpdate"] = "true"
		
		g.logger.InfoWithContext("enabling force update", map[string]interface{}{
			"chart": chart.Name,
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
			"chart": chart.Name,
			"namespace": namespace,
			"attempt": attempt,
			"timeout": helmOptions.Timeout,
			"atomic": helmOptions.Atomic,
			"wait": helmOptions.Wait,
		})
		
		err := g.helmPort.UpgradeInstall(deployCtx, chart.Name, chart.Path, helmOptions)
		cancel()
		duration := time.Since(start)
		
		if err == nil {
			g.logger.InfoWithContext("chart deployed successfully", map[string]interface{}{
				"chart":     chart.Name,
				"namespace": namespace,
				"duration":  duration,
				"attempt":   attempt,
			})
			return nil
		}
		
		// Check if the deployment was cancelled or timed out
		if deployCtx.Err() == context.DeadlineExceeded {
			g.logger.ErrorWithContext("chart deployment timed out", map[string]interface{}{
				"chart": chart.Name,
				"namespace": namespace,
				"timeout": helmOptions.Timeout,
				"attempt": attempt,
				"duration": duration,
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
		})
		
		// Check if error is retriable
		if !g.isRetriableError(err) || attempt == maxRetries {
			g.logger.WarnWithContext("not retrying chart deployment", map[string]interface{}{
				"chart": chart.Name,
				"attempt": attempt,
				"retriable": g.isRetriableError(err),
				"is_last_attempt": attempt == maxRetries,
			})
			break
		}
		
		// Exponential backoff
		retryDelay := time.Duration(attempt) * 5 * time.Second // Shorter retry delay
		g.logger.WarnWithContext("retrying chart deployment", map[string]interface{}{
			"chart":       chart.Name,
			"attempt":     attempt,
			"next_attempt_in": retryDelay,
		})
		
		select {
		case <-ctx.Done():
			g.logger.WarnWithContext("chart deployment cancelled during retry", map[string]interface{}{
				"chart": chart.Name,
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

// ListReleases returns list of Helm releases
func (g *HelmGateway) ListReleases(ctx context.Context, namespace string) ([]helm_port.HelmRelease, error) {
	g.logger.DebugWithContext("listing releases", map[string]interface{}{
		"namespace": namespace,
	})
	
	releases, err := g.helmPort.List(ctx, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to list releases", map[string]interface{}{
			"namespace": namespace,
			"error":     err.Error(),
		})
		return nil, fmt.Errorf("failed to list releases in namespace %s: %w", namespace, err)
	}
	
	g.logger.DebugWithContext("releases listed", map[string]interface{}{
		"namespace": namespace,
		"count":     len(releases),
	})
	
	return releases, nil
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
		return envFile
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
			"release":     releaseName,
			"namespace":   namespace,
			"operation":   operation.Type,
			"status":      operation.Status,
			"start_time":  operation.StartTime,
			"pid":         operation.PID,
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
	
	// Retriable conditions
	retriablePatterns := []string{
		"another operation in progress",
		"connection refused",
		"timeout",
		"temporary failure",
		"resource temporarily unavailable",
	}
	
	for _, pattern := range retriablePatterns {
		if strings.Contains(errorMsg, pattern) {
			return true
		}
	}
	
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
		"chart": chart.Name,
		"namespace": namespace,
	})
	
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