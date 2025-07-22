// PHASE R2: Helm release management functionality
package management

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/helm_port"
	"deploy-cli/port/logger_port"
)

// HelmReleaseManager handles Helm release lifecycle management
type HelmReleaseManager struct {
	helmPort helm_port.HelmPort
	logger   logger_port.LoggerPort
}

// HelmReleaseManagerPort defines the interface for Helm release management operations
type HelmReleaseManagerPort interface {
	ListReleases(ctx context.Context, options *domain.ReleaseListOptions) ([]*domain.ReleaseInfo, error)
	GetReleaseHistory(ctx context.Context, releaseName, namespace string) ([]*domain.ReleaseRevision, error)
	GetReleaseStatus(ctx context.Context, releaseName, namespace string) (*domain.HelmReleaseStatus, error)
	GetReleaseValues(ctx context.Context, releaseName, namespace string, allValues bool) (map[string]interface{}, error)
	GetReleaseManifest(ctx context.Context, releaseName, namespace string, revision int) (string, error)
	TestRelease(ctx context.Context, releaseName, namespace string, options *domain.TestOptions) (*domain.TestResult, error)
	PurgeRelease(ctx context.Context, releaseName, namespace string) error
}

// NewHelmReleaseManager creates a new Helm release manager
func NewHelmReleaseManager(
	helmPort helm_port.HelmPort,
	logger logger_port.LoggerPort,
) *HelmReleaseManager {
	return &HelmReleaseManager{
		helmPort: helmPort,
		logger:   logger,
	}
}

// ListReleases lists Helm releases with filtering options
func (h *HelmReleaseManager) ListReleases(ctx context.Context, options *domain.ReleaseListOptions) ([]*domain.ReleaseInfo, error) {
	h.logger.InfoWithContext("listing Helm releases", map[string]interface{}{
		"namespace":    options.Namespace,
		"all_namespaces": options.AllNamespaces,
		"status_filter": options.StatusFilter,
		"max_releases": options.MaxReleases,
	})

	releases, err := h.helmPort.ListReleases(ctx, options)
	if err != nil {
		h.logger.ErrorWithContext("failed to list Helm releases", map[string]interface{}{
			"namespace": options.Namespace,
			"error":     err.Error(),
		})
		return nil, fmt.Errorf("failed to list Helm releases: %w", err)
	}

	// Apply additional filtering
	filteredReleases := h.applyAdvancedFiltering(releases, options)

	// Sort releases if requested
	if options.SortBy != "" {
		h.sortReleases(filteredReleases, options.SortBy, options.SortOrder)
	}

	h.logger.InfoWithContext("Helm releases listed successfully", map[string]interface{}{
		"total_releases":    len(releases),
		"filtered_releases": len(filteredReleases),
		"namespace":         options.Namespace,
	})

	return filteredReleases, nil
}

// GetReleaseHistory gets the revision history of a Helm release
func (h *HelmReleaseManager) GetReleaseHistory(ctx context.Context, releaseName, namespace string) ([]*domain.ReleaseRevision, error) {
	h.logger.InfoWithContext("getting Helm release history", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
	})

	request := &domain.HelmHistoryRequest{
		ReleaseName:  releaseName,
		Namespace:    namespace,
		MaxRevisions: 0, // Get all history
	}

	history, err := h.helmPort.GetReleaseHistory(ctx, request)
	if err != nil {
		h.logger.ErrorWithContext("failed to get Helm release history", map[string]interface{}{
			"release_name": releaseName,
			"namespace":    namespace,
			"error":        err.Error(),
		})
		return nil, fmt.Errorf("failed to get release history for %s: %w", releaseName, err)
	}

	// Sort history by revision number (newest first)
	sort.Slice(history, func(i, j int) bool {
		return history[i].Revision > history[j].Revision
	})

	h.logger.InfoWithContext("Helm release history retrieved successfully", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
		"revisions":    len(history),
	})

	return history, nil
}

// GetReleaseStatus gets the current status of a Helm release
func (h *HelmReleaseManager) GetReleaseStatus(ctx context.Context, releaseName, namespace string) (*domain.HelmReleaseStatus, error) {
	h.logger.DebugWithContext("getting Helm release status", map[string]interface{}{
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
		return nil, fmt.Errorf("failed to get release status for %s: %w", releaseName, err)
	}

	// Convert ReleaseInfo to HelmReleaseStatus format
	releaseStatus := &domain.HelmReleaseStatus{
		Name:        status.Name,
		Namespace:   status.Namespace,
		Version:     status.Version,
		Status:      status.Status,
		Description: "Release status from ReleaseInfo",
		LastUpdated: status.Updated.Format(time.RFC3339),
		Exists:      true,
	}

	h.logger.DebugWithContext("Helm release status retrieved successfully", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
		"status":       releaseStatus.Status,
		"version":      releaseStatus.Version,
	})

	return releaseStatus, nil
}

// GetReleaseValues gets the values of a Helm release
func (h *HelmReleaseManager) GetReleaseValues(ctx context.Context, releaseName, namespace string, allValues bool) (map[string]interface{}, error) {
	h.logger.DebugWithContext("getting Helm release values", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
		"all_values":   allValues,
	})

	request := &domain.HelmValuesRequest{
		ChartName: releaseName, // Use releaseName as chartName
		ChartPath: "",         // Empty path for release values
	}

	values, err := h.helmPort.GetChartValues(ctx, request)
	if err != nil {
		h.logger.ErrorWithContext("failed to get Helm release values", map[string]interface{}{
			"release_name": releaseName,
			"namespace":    namespace,
			"error":        err.Error(),
		})
		return nil, fmt.Errorf("failed to get release values for %s: %w", releaseName, err)
	}

	h.logger.DebugWithContext("Helm release values retrieved successfully", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
		"values_keys":  len(values),
	})

	return values, nil
}

// GetReleaseManifest gets the rendered manifest of a Helm release
func (h *HelmReleaseManager) GetReleaseManifest(ctx context.Context, releaseName, namespace string, revision int) (string, error) {
	h.logger.DebugWithContext("getting Helm release manifest", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
		"revision":     revision,
	})

	h.logger.WarnWithContext("GetReleaseManifest not implemented in HelmPort", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
		"revision":     revision,
	})

	return "", fmt.Errorf("GetReleaseManifest is not implemented for release %s", releaseName)
}

// TestRelease runs Helm tests for a release
func (h *HelmReleaseManager) TestRelease(ctx context.Context, releaseName, namespace string, options *domain.TestOptions) (*domain.TestResult, error) {
	h.logger.InfoWithContext("running Helm release tests", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
		"cleanup":      options.Cleanup,
		"parallel":     options.Parallel,
		"timeout":      options.Timeout.String(),
	})

	h.logger.WarnWithContext("TestRelease not implemented in HelmPort", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
	})

	return nil, fmt.Errorf("TestRelease is not implemented for release %s", releaseName)
}

// PurgeRelease completely removes a Helm release and its history
func (h *HelmReleaseManager) PurgeRelease(ctx context.Context, releaseName, namespace string) error {
	h.logger.InfoWithContext("purging Helm release", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
	})

	// First, uninstall the release without keeping history
	uninstallRequest := &domain.HelmUndeploymentRequest{
		ReleaseName:  releaseName,
		Namespace:    namespace,
		KeepHistory:  false,
		Wait:         true,
		Timeout:      5 * time.Minute,
		DisableHooks: false,
		DryRun:       false,
	}

	err := h.helmPort.UninstallChart(ctx, uninstallRequest)
	if err != nil {
		h.logger.ErrorWithContext("failed to purge Helm release", map[string]interface{}{
			"release_name": releaseName,
			"namespace":    namespace,
			"error":        err.Error(),
		})
		return fmt.Errorf("failed to purge release %s: %w", releaseName, err)
	}

	h.logger.InfoWithContext("Helm release purged successfully", map[string]interface{}{
		"release_name": releaseName,
		"namespace":    namespace,
	})

	return nil
}

// Helper methods

// applyAdvancedFiltering applies advanced filtering options to releases
func (h *HelmReleaseManager) applyAdvancedFiltering(releases []*domain.ReleaseInfo, options *domain.ReleaseListOptions) []*domain.ReleaseInfo {
	filtered := make([]*domain.ReleaseInfo, 0, len(releases))

	for _, release := range releases {
		// Apply name filter if specified (using Filter field)
		if options.Filter != "" && !h.matchesNameFilter(release.Name, options.Filter) {
			continue
		}

		// Apply status filter if specified
		if len(options.StatusFilter) > 0 {
			statusMatch := false
			for _, status := range options.StatusFilter {
				if release.Status == status {
					statusMatch = true
					break
				}
			}
			if !statusMatch {
				continue
			}
		}

		filtered = append(filtered, release)
	}

	return filtered
}

// matchesNameFilter checks if a release name matches the filter pattern
func (h *HelmReleaseManager) matchesNameFilter(releaseName, filter string) bool {
	// Support simple wildcard matching
	if strings.Contains(filter, "*") {
		// Convert simple wildcard to regex-like matching
		pattern := strings.ReplaceAll(filter, "*", ".*")
		// For simplicity, just check prefix/suffix matching
		if strings.HasPrefix(pattern, ".*") {
			return strings.HasSuffix(releaseName, strings.TrimPrefix(pattern, ".*"))
		}
		if strings.HasSuffix(pattern, ".*") {
			return strings.HasPrefix(releaseName, strings.TrimSuffix(pattern, ".*"))
		}
	}
	
	// Exact match or substring match
	return strings.Contains(releaseName, filter)
}

// sortReleases sorts releases based on the specified criteria
func (h *HelmReleaseManager) sortReleases(releases []*domain.ReleaseInfo, sortBy, sortOrder string) {
	ascending := sortOrder != "desc"

	sort.Slice(releases, func(i, j int) bool {
		var less bool
		
		switch sortBy {
		case "name":
			less = releases[i].Name < releases[j].Name
		case "namespace":
			less = releases[i].Namespace < releases[j].Namespace
		case "revision":
			less = releases[i].Revision < releases[j].Revision
		case "updated":
			less = releases[i].Updated.Before(releases[j].Updated)
		case "status":
			less = releases[i].Status < releases[j].Status
		case "chart":
			less = releases[i].Chart < releases[j].Chart
		case "app_version":
			less = releases[i].AppVersion < releases[j].AppVersion
		default:
			// Default sort by name
			less = releases[i].Name < releases[j].Name
		}

		if !ascending {
			less = !less
		}

		return less
	})
}

// GetReleasesByStatus gets releases filtered by status
func (h *HelmReleaseManager) GetReleasesByStatus(ctx context.Context, namespace, status string) ([]*domain.ReleaseInfo, error) {
	options := &domain.ReleaseListOptions{
		Namespace:    namespace,
		StatusFilter: []string{status},
		MaxReleases:  100,
	}

	return h.ListReleases(ctx, options)
}

// GetPendingReleases gets releases that are in pending/processing states
func (h *HelmReleaseManager) GetPendingReleases(ctx context.Context, namespace string) ([]*domain.ReleaseInfo, error) {
	options := &domain.ReleaseListOptions{
		Namespace:    namespace,
		StatusFilter: []string{"pending-install", "pending-upgrade", "pending-rollback"},
		MaxReleases:  100,
	}

	return h.ListReleases(ctx, options)
}

// GetFailedReleases gets releases that are in failed states
func (h *HelmReleaseManager) GetFailedReleases(ctx context.Context, namespace string) ([]*domain.ReleaseInfo, error) {
	options := &domain.ReleaseListOptions{
		Namespace:    namespace,
		StatusFilter: []string{"failed"},
		MaxReleases:  100,
	}

	return h.ListReleases(ctx, options)
}

// GetReleaseMetrics calculates metrics for releases
func (h *HelmReleaseManager) GetReleaseMetrics(ctx context.Context, namespace string) (*domain.ReleaseMetrics, error) {
	h.logger.DebugWithContext("calculating release metrics", map[string]interface{}{
		"namespace": namespace,
	})

	options := &domain.ReleaseListOptions{
		Namespace:     namespace,
		AllNamespaces: namespace == "",
		MaxReleases:   1000,
	}

	releases, err := h.ListReleases(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("failed to get releases for metrics: %w", err)
	}

	metrics := &domain.ReleaseMetrics{
		Name:              namespace,
		Namespace:         namespace,
		CollectionTime:    time.Now(),
		DeploymentCount:   len(releases),
		SuccessfulDeploys: 0,
		FailedDeploys:     0,
	}

	for _, release := range releases {
		// Count successful vs failed releases based on status
		if release.Status == "deployed" {
			metrics.SuccessfulDeploys++
		} else if release.Status == "failed" {
			metrics.FailedDeploys++
		}
	}

	h.logger.DebugWithContext("release metrics calculated", map[string]interface{}{
		"namespace":         namespace,
		"deployment_count":  metrics.DeploymentCount,
		"successful_deploys": metrics.SuccessfulDeploys,
		"failed_deploys":    metrics.FailedDeploys,
	})

	return metrics, nil
}