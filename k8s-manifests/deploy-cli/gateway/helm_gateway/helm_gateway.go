package helm_gateway

import (
	"context"
	"fmt"
	"strings"
	"time"
	
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
	if chart.SupportsImageOverride() && options.ShouldOverrideImage() {
		helmOptions.ImageOverrides = map[string]string{
			"image.repository": options.ImagePrefix,
			"image.tag":        options.GetImageTag(chart.Name),
		}
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
	g.logger.InfoWithContext("deploying chart", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
	})
	
	helmOptions := helm_port.HelmUpgradeOptions{
		ValuesFile:      g.getValuesFile(chart, options.Environment),
		Namespace:       options.GetNamespace(chart.Name),
		CreateNamespace: true,
		Wait:            chart.ShouldWaitForReadiness(),
		Timeout:         options.Timeout,
		Force:           options.ForceUpdate,
	}
	
	// Add image overrides if applicable
	if chart.SupportsImageOverride() && options.ShouldOverrideImage() {
		helmOptions.ImageOverrides = map[string]string{
			"image.repository": options.ImagePrefix,
			"image.tag":        options.GetImageTag(chart.Name),
		}
	}
	
	// Add force update flag
	if options.ForceUpdate {
		helmOptions.SetValues = map[string]string{
			"forceUpdate": "true",
		}
	}
	
	start := time.Now()
	err := g.helmPort.UpgradeInstall(ctx, chart.Name, chart.Path, helmOptions)
	duration := time.Since(start)
	
	if err != nil {
		g.logger.ErrorWithContext("chart deployment failed", map[string]interface{}{
			"chart":       chart.Name,
			"namespace":   options.GetNamespace(chart.Name),
			"chart_path":  chart.Path,
			"values_file": helmOptions.ValuesFile,
			"error":       err.Error(),
			"duration":    duration,
			"timeout":     helmOptions.Timeout,
			"resolution":  "Check chart templates, values file, and cluster resources",
		})
		return fmt.Errorf("chart deployment failed for %s: %w", chart.Name, err)
	}
	
	g.logger.InfoWithContext("chart deployed successfully", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
		"duration":  duration,
	})
	
	return nil
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