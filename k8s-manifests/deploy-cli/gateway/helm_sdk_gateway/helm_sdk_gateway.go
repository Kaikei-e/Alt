package helm_sdk_gateway

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/storage/driver"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// HelmSDKGateway provides Helm operations using the official Go SDK
type HelmSDKGateway struct {
	logger       logger_port.LoggerPort
	settings     *cli.EnvSettings
	actionConfig *action.Configuration
}

// NewHelmSDKGateway creates a new Helm SDK gateway
func NewHelmSDKGateway(logger logger_port.LoggerPort, namespace string) (*HelmSDKGateway, error) {
	settings := cli.New()

	actionConfig := new(action.Configuration)

	// Initialize with the specified namespace
	if err := actionConfig.Init(settings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER"), func(format string, v ...interface{}) {
		log.Printf(format, v...)
	}); err != nil {
		return nil, fmt.Errorf("failed to initialize Helm action configuration: %w", err)
	}

	return &HelmSDKGateway{
		logger:       logger,
		settings:     settings,
		actionConfig: actionConfig,
	}, nil
}

// AtomicUpgrade performs an atomic Helm upgrade with automatic rollback on failure
func (g *HelmSDKGateway) AtomicUpgrade(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	g.logger.InfoWithContext("starting atomic Helm upgrade", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
		"atomic":    true,
	})

	// Load the chart
	chartPath := fmt.Sprintf("/home/koko/Documents/dev/Alt/charts/%s", chart.Name)
	helmChart, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load chart %s: %w", chart.Name, err)
	}

	// Prepare values
	vals := map[string]interface{}{}
	if options.ForceUpdate {
		vals["forceUpdate"] = true
	}

	// Create upgrade action
	upgrade := action.NewUpgrade(g.actionConfig)
	upgrade.Namespace = options.GetNamespace(chart.Name)
	upgrade.Atomic = true              // Enable atomic operations
	upgrade.Wait = true                // Wait for resources to be ready
	upgrade.Timeout = 10 * time.Minute // Extended timeout for database charts

	// Set image overrides if applicable
	if chart.SupportsImageOverride() && (options.ShouldOverrideImage() || options.ForceUpdate) {
		imageTag := options.GetImageTag(chart.Name)
		vals["image"] = map[string]interface{}{
			"repository": options.ImagePrefix,
			"tag":        imageTag,
		}
	}

	// Execute the upgrade
	g.logger.InfoWithContext("executing atomic upgrade", map[string]interface{}{
		"chart":   chart.Name,
		"timeout": upgrade.Timeout,
		"atomic":  upgrade.Atomic,
	})

	release, err := upgrade.Run(chart.Name, helmChart, vals)
	if err != nil {
		g.logger.ErrorWithContext("atomic upgrade failed", map[string]interface{}{
			"chart": chart.Name,
			"error": err.Error(),
		})
		return fmt.Errorf("atomic Helm upgrade failed for %s: %w", chart.Name, err)
	}

	g.logger.InfoWithContext("atomic upgrade completed successfully", map[string]interface{}{
		"chart":       chart.Name,
		"release":     release.Name,
		"version":     release.Version,
		"status":      release.Info.Status,
		"description": release.Info.Description,
	})

	return nil
}

// AtomicInstall performs an atomic Helm install with automatic cleanup on failure
func (g *HelmSDKGateway) AtomicInstall(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	g.logger.InfoWithContext("starting atomic Helm install", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
		"atomic":    true,
	})

	// Load the chart
	chartPath := fmt.Sprintf("/home/koko/Documents/dev/Alt/charts/%s", chart.Name)
	helmChart, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load chart %s: %w", chart.Name, err)
	}

	// Prepare values
	vals := map[string]interface{}{}
	if options.ForceUpdate {
		vals["forceUpdate"] = true
	}

	// Create install action
	install := action.NewInstall(g.actionConfig)
	install.ReleaseName = chart.Name
	install.Namespace = options.GetNamespace(chart.Name)
	install.Atomic = true              // Enable atomic operations
	install.Wait = true                // Wait for resources to be ready
	install.Timeout = 10 * time.Minute // Extended timeout
	install.CreateNamespace = true     // Create namespace if not exists

	// Set image overrides if applicable
	if chart.SupportsImageOverride() && (options.ShouldOverrideImage() || options.ForceUpdate) {
		imageTag := options.GetImageTag(chart.Name)
		vals["image"] = map[string]interface{}{
			"repository": options.ImagePrefix,
			"tag":        imageTag,
		}
	}

	// Execute the install
	g.logger.InfoWithContext("executing atomic install", map[string]interface{}{
		"chart":   chart.Name,
		"timeout": install.Timeout,
		"atomic":  install.Atomic,
	})

	release, err := install.Run(helmChart, vals)
	if err != nil {
		g.logger.ErrorWithContext("atomic install failed", map[string]interface{}{
			"chart": chart.Name,
			"error": err.Error(),
		})
		return fmt.Errorf("atomic Helm install failed for %s: %w", chart.Name, err)
	}

	g.logger.InfoWithContext("atomic install completed successfully", map[string]interface{}{
		"chart":       chart.Name,
		"release":     release.Name,
		"version":     release.Version,
		"status":      release.Info.Status,
		"description": release.Info.Description,
	})

	return nil
}

// RollbackRelease performs a rollback to the previous revision
func (g *HelmSDKGateway) RollbackRelease(ctx context.Context, releaseName string, revision int) error {
	g.logger.InfoWithContext("starting Helm rollback", map[string]interface{}{
		"release":  releaseName,
		"revision": revision,
	})

	rollback := action.NewRollback(g.actionConfig)
	rollback.Wait = true
	rollback.Timeout = 5 * time.Minute

	if err := rollback.Run(releaseName); err != nil {
		return fmt.Errorf("failed to rollback release %s: %w", releaseName, err)
	}

	g.logger.InfoWithContext("rollback completed successfully", map[string]interface{}{
		"release": releaseName,
	})

	return nil
}

// GetReleaseStatus returns the status of a Helm release
func (g *HelmSDKGateway) GetReleaseStatus(ctx context.Context, releaseName string) (*domain.HelmReleaseStatus, error) {
	status := action.NewStatus(g.actionConfig)

	release, err := status.Run(releaseName)
	if err != nil {
		// Check if it's a "not found" error
		if err == driver.ErrReleaseNotFound {
			return &domain.HelmReleaseStatus{
				Name:   releaseName,
				Status: "not-found",
				Exists: false,
			}, nil
		}
		return nil, fmt.Errorf("failed to get status for release %s: %w", releaseName, err)
	}

	return &domain.HelmReleaseStatus{
		Name:        release.Name,
		Namespace:   release.Namespace,
		Version:     release.Version,
		Status:      release.Info.Status.String(),
		Description: release.Info.Description,
		LastUpdated: release.Info.LastDeployed.Format(time.RFC3339),
		Exists:      true,
	}, nil
}

// DeployChart performs an atomic deployment (install or upgrade)
func (g *HelmSDKGateway) DeployChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	// Check if release exists
	status, err := g.GetReleaseStatus(ctx, chart.Name)
	if err != nil {
		return fmt.Errorf("failed to check release status: %w", err)
	}

	if status.Exists {
		return g.AtomicUpgrade(ctx, chart, options)
	} else {
		return g.AtomicInstall(ctx, chart, options)
	}
}
