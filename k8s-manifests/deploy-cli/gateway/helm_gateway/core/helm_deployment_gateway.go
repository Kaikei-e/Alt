// PHASE R2: Core Helm deployment functionality extracted from helm_gateway.go
package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/helm_port"
	"deploy-cli/port/logger_port"
)

// HelmDeploymentGateway handles core Helm deployment operations
type HelmDeploymentGateway struct {
	helmPort helm_port.HelmPort
	logger   logger_port.LoggerPort
}

// HelmDeploymentGatewayPort defines the interface for core Helm deployment operations
type HelmDeploymentGatewayPort interface {
	DeployChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error
	UndeployChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error
	UpgradeChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error
	RollbackChart(ctx context.Context, chart domain.Chart, targetVersion string, options *domain.DeploymentOptions) error
	GetDeploymentStatus(ctx context.Context, releaseName, namespace string) (*domain.ChartStatus, error)
}

// NewHelmDeploymentGateway creates a new Helm deployment gateway
func NewHelmDeploymentGateway(
	helmPort helm_port.HelmPort,
	logger logger_port.LoggerPort,
) *HelmDeploymentGateway {
	return &HelmDeploymentGateway{
		helmPort: helmPort,
		logger:   logger,
	}
}

// DeployChart deploys a chart using Helm
func (h *HelmDeploymentGateway) DeployChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	h.logger.InfoWithContext("deploying chart with Helm", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
		"path":      chart.Path,
		"type":      chart.Type,
	})

	// Prepare deployment request
	request := &domain.HelmDeploymentRequest{
		ChartName:         chart.Name,
		ChartPath:         chart.Path,
		ReleaseName:       h.generateReleaseName(chart, options.GetNamespace(chart.Name)),
		Namespace:         options.GetNamespace(chart.Name),
		Values:            h.prepareValues(chart, options),
		Wait:              chart.WaitReady,
		Timeout:           options.DeployTimeout,
		CreateNamespace:   options.CreateNamespace,
		DryRun:           options.DryRun,
		Force:            options.Force,
		DisableHooks:     options.DisableHooks,
		SkipCRDs:         options.SkipCRDs,
	}

	// Execute deployment
	err := h.helmPort.InstallChart(ctx, request)
	if err != nil {
		h.logger.ErrorWithContext("Helm chart deployment failed", map[string]interface{}{
			"chart":        chart.Name,
			"namespace":    request.Namespace,
			"release_name": request.ReleaseName,
			"error":        err.Error(),
		})
		return fmt.Errorf("Helm deployment failed for chart %s: %w", chart.Name, err)
	}

	h.logger.InfoWithContext("Helm chart deployment completed", map[string]interface{}{
		"chart":        chart.Name,
		"namespace":    request.Namespace,
		"release_name": request.ReleaseName,
	})

	return nil
}

// UndeployChart undeploys a chart using Helm
func (h *HelmDeploymentGateway) UndeployChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	h.logger.InfoWithContext("undeploying chart with Helm", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
	})

	releaseName := h.generateReleaseName(chart, options.GetNamespace(chart.Name))
	namespace := options.GetNamespace(chart.Name)

	request := &domain.HelmUndeploymentRequest{
		ReleaseName:    releaseName,
		Namespace:      namespace,
		KeepHistory:    options.KeepHistory,
		Wait:           true,
		Timeout:        options.UndeployTimeout,
		DisableHooks:   options.DisableHooks,
		DryRun:         options.DryRun,
	}

	err := h.helmPort.UninstallChart(ctx, request)
	if err != nil {
		h.logger.ErrorWithContext("Helm chart undeployment failed", map[string]interface{}{
			"chart":        chart.Name,
			"namespace":    namespace,
			"release_name": releaseName,
			"error":        err.Error(),
		})
		return fmt.Errorf("Helm undeployment failed for chart %s: %w", chart.Name, err)
	}

	h.logger.InfoWithContext("Helm chart undeployment completed", map[string]interface{}{
		"chart":        chart.Name,
		"namespace":    namespace,
		"release_name": releaseName,
	})

	return nil
}

// UpgradeChart upgrades a chart using Helm
func (h *HelmDeploymentGateway) UpgradeChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	h.logger.InfoWithContext("upgrading chart with Helm", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
		"path":      chart.Path,
	})

	releaseName := h.generateReleaseName(chart, options.GetNamespace(chart.Name))
	namespace := options.GetNamespace(chart.Name)

	request := &domain.HelmUpgradeRequest{
		ChartName:         chart.Name,
		ChartPath:         chart.Path,
		ReleaseName:       releaseName,
		Namespace:         namespace,
		Values:            h.prepareValues(chart, options),
		Wait:              chart.WaitReady,
		Timeout:           options.DeployTimeout,
		Force:            options.Force,
		ResetValues:      options.ResetValues,
		ReuseValues:      options.ReuseValues,
		DisableHooks:     options.DisableHooks,
		DryRun:           options.DryRun,
		Install:          true, // Enable install if not already installed
	}

	err := h.helmPort.UpgradeChart(ctx, request)
	if err != nil {
		h.logger.ErrorWithContext("Helm chart upgrade failed", map[string]interface{}{
			"chart":        chart.Name,
			"namespace":    namespace,
			"release_name": releaseName,
			"error":        err.Error(),
		})
		return fmt.Errorf("Helm upgrade failed for chart %s: %w", chart.Name, err)
	}

	h.logger.InfoWithContext("Helm chart upgrade completed", map[string]interface{}{
		"chart":        chart.Name,
		"namespace":    namespace,
		"release_name": releaseName,
		"operation":    "upgrade",
	})

	return nil
}

// RollbackChart rolls back a chart to a specific version
func (h *HelmDeploymentGateway) RollbackChart(ctx context.Context, chart domain.Chart, targetVersion string, options *domain.DeploymentOptions) error {
	h.logger.InfoWithContext("rolling back chart with Helm", map[string]interface{}{
		"chart":          chart.Name,
		"namespace":      options.GetNamespace(chart.Name),
		"target_version": targetVersion,
	})

	releaseName := h.generateReleaseName(chart, options.GetNamespace(chart.Name))
	namespace := options.GetNamespace(chart.Name)

	request := &domain.HelmRollbackRequest{
		ReleaseName:    releaseName,
		Namespace:      namespace,
		Revision:       1, // Default to revision 1
		Wait:           true,
		Timeout:        options.RollbackTimeout,
		DisableHooks:   options.DisableHooks,
		DryRun:         options.DryRun,
		Force:          options.Force,
		RecreateResources: options.RecreateResources,
	}

	err := h.helmPort.RollbackChart(ctx, request)
	if err != nil {
		h.logger.ErrorWithContext("Helm chart rollback failed", map[string]interface{}{
			"chart":          chart.Name,
			"namespace":      namespace,
			"release_name":   releaseName,
			"target_version": targetVersion,
			"error":          err.Error(),
		})
		return fmt.Errorf("Helm rollback failed for chart %s: %w", chart.Name, err)
	}

	h.logger.InfoWithContext("Helm chart rollback completed", map[string]interface{}{
		"chart":          chart.Name,
		"namespace":      namespace,
		"release_name":   releaseName,
		"target_version": targetVersion,
		"operation":      "rollback",
	})

	return nil
}

// GetDeploymentStatus gets the status of a deployed chart
func (h *HelmDeploymentGateway) GetDeploymentStatus(ctx context.Context, releaseName, namespace string) (*domain.ChartStatus, error) {
	h.logger.DebugWithContext("getting deployment status from Helm", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
	})

	status, err := h.helmPort.GetReleaseStatus(ctx, releaseName, namespace)
	if err != nil {
		h.logger.ErrorWithContext("failed to get Helm release status", map[string]interface{}{
			"release_name": releaseName,
			"namespace":    namespace,
			"error":        err.Error(),
		})
		return nil, fmt.Errorf("failed to get Helm release status for %s: %w", releaseName, err)
	}

	// Convert ReleaseInfo to ChartStatus
	chartStatus := &domain.ChartStatus{
		Name:         status.Name,
		Namespace:    status.Namespace,
		Status:       status.Status,
		Version:      "", // Not available in ReleaseInfo
		AppVersion:   "", // Not available in ReleaseInfo
		LastDeployed: time.Now(), // Use current time as placeholder
	}
	
	return chartStatus, nil
}

// Helper methods

// generateReleaseName generates a consistent release name for a chart
func (h *HelmDeploymentGateway) generateReleaseName(chart domain.Chart, namespace string) string {
	// Use chart name as release name for consistency
	// In multi-namespace charts, include namespace in release name
	if chart.MultiNamespace {
		return fmt.Sprintf("%s-%s", chart.Name, namespace)
	}
	return chart.Name
}

// prepareValues prepares values for Helm deployment
func (h *HelmDeploymentGateway) prepareValues(chart domain.Chart, options *domain.DeploymentOptions) map[string]interface{} {
	values := make(map[string]interface{})

	// Add chart-specific values
	if chart.Values != nil {
		for key, value := range chart.Values {
			values[key] = value
		}
	}

	// Add deployment option values
	if options.Values != nil {
		for key, value := range options.Values {
			values[key] = value
		}
	}

	// Add image override if supported
	if chart.SupportsImageOverride() && options.ImageTag != "" {
		h.addImageOverride(values, options.ImageTag)
	}

	// Add namespace context
	values["namespace"] = map[string]interface{}{
		"name":        options.GetNamespace(chart.Name),
		"create":      options.CreateNamespace,
	}

	// Add deployment metadata
	values["deployment"] = map[string]interface{}{
		"name":      fmt.Sprintf("deployment-%s", chart.Name),
		"timestamp": time.Now().Unix(),
		"strategy":  options.Strategy,
	}

	return values
}

// addImageOverride adds image override configuration to values
func (h *HelmDeploymentGateway) addImageOverride(values map[string]interface{}, imageTag string) {
	imageConfig := map[string]interface{}{
		"tag": imageTag,
		"pullPolicy": "IfNotPresent",
	}

	// Check if image configuration already exists
	if existingImage, exists := values["image"]; exists {
		if imageMap, ok := existingImage.(map[string]interface{}); ok {
			imageMap["tag"] = imageTag
		}
	} else {
		values["image"] = imageConfig
	}
}

// validateDeploymentPreconditions validates preconditions before deployment
func (h *HelmDeploymentGateway) validateDeploymentPreconditions(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	namespace := options.GetNamespace(chart.Name)

	h.logger.DebugWithContext("validating deployment preconditions", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": namespace,
	})

	// Check if chart path exists
	if chart.Path == "" {
		return fmt.Errorf("chart path is required for %s", chart.Name)
	}

	// Validate namespace name
	if err := h.validateNamespaceName(namespace); err != nil {
		return fmt.Errorf("invalid namespace name %s: %w", namespace, err)
	}

	// Check for conflicting releases
	if !options.Force {
		if err := h.checkForConflictingReleases(ctx, chart, namespace); err != nil {
			return fmt.Errorf("conflicting release check failed: %w", err)
		}
	}

	return nil
}

// validateNamespaceName validates Kubernetes namespace naming conventions
func (h *HelmDeploymentGateway) validateNamespaceName(namespace string) error {
	if namespace == "" {
		return fmt.Errorf("namespace name cannot be empty")
	}

	if len(namespace) > 63 {
		return fmt.Errorf("namespace name too long (max 63 characters)")
	}

	if strings.Contains(namespace, "_") {
		return fmt.Errorf("namespace name cannot contain underscores")
	}

	return nil
}

// checkForConflictingReleases checks for conflicting Helm releases
func (h *HelmDeploymentGateway) checkForConflictingReleases(ctx context.Context, chart domain.Chart, namespace string) error {
	releaseName := h.generateReleaseName(chart, namespace)

	// Check if release already exists
	_, err := h.helmPort.GetReleaseStatus(ctx, releaseName, namespace)
	if err != nil {
		// Release doesn't exist - no conflict
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		// Other error
		return fmt.Errorf("failed to check existing release: %w", err)
	}

	// Release exists - this could be an upgrade scenario
	h.logger.InfoWithContext("existing release found, this will be treated as upgrade", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
		"chart":        chart.Name,
	})

	return nil
}