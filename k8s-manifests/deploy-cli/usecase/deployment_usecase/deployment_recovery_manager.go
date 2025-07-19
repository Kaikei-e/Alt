package deployment_usecase

import (
	"context"
	"fmt"
	"time"

	"deploy-cli/domain"
	"deploy-cli/gateway/helm_gateway"
	"deploy-cli/port/logger_port"
)

// DeploymentRecoveryManager handles deployment rollback and recovery operations
type DeploymentRecoveryManager struct {
	helmGateway *helm_gateway.HelmGateway
	logger      logger_port.LoggerPort
}

// NewDeploymentRecoveryManager creates a new deployment recovery manager
func NewDeploymentRecoveryManager(
	helmGateway *helm_gateway.HelmGateway,
	logger logger_port.LoggerPort,
) *DeploymentRecoveryManager {
	return &DeploymentRecoveryManager{
		helmGateway: helmGateway,
		logger:      logger,
	}
}

// CreateDeploymentCheckpoint creates a checkpoint of the current deployment state
func (r *DeploymentRecoveryManager) CreateDeploymentCheckpoint(ctx context.Context, options *domain.DeploymentOptions) (*domain.DeploymentCheckpoint, error) {
	checkpointID := fmt.Sprintf("checkpoint-%d", time.Now().Unix())

	r.logger.InfoWithContext("creating deployment checkpoint", map[string]interface{}{
		"checkpoint_id": checkpointID,
		"environment":   options.Environment.String(),
	})

	// Get current Helm releases
	namespaces := domain.GetNamespacesForEnvironment(options.Environment)
	var releases []domain.HelmReleaseInfo

	for _, namespace := range namespaces {
		nsReleases, err := r.helmGateway.ListReleases(ctx, namespace)
		if err != nil {
			r.logger.WarnWithContext("failed to list releases for checkpoint", map[string]interface{}{
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

	r.logger.InfoWithContext("deployment checkpoint created", map[string]interface{}{
		"checkpoint_id":    checkpointID,
		"releases_count":   len(releases),
		"namespaces_count": len(namespaces),
	})

	return checkpoint, nil
}

// RollbackToCheckpoint rolls back deployment to a previous checkpoint
func (r *DeploymentRecoveryManager) RollbackToCheckpoint(ctx context.Context, checkpoint *domain.DeploymentCheckpoint, options *domain.DeploymentOptions) error {
	r.logger.InfoWithContext("starting rollback to checkpoint", map[string]interface{}{
		"checkpoint_id":        checkpoint.ID,
		"checkpoint_timestamp": checkpoint.Timestamp,
		"environment":          options.Environment.String(),
	})

	// Get current releases
	var currentReleases []domain.HelmReleaseInfo
	for _, namespace := range checkpoint.Namespaces {
		nsReleases, err := r.helmGateway.ListReleases(ctx, namespace)
		if err != nil {
			r.logger.WarnWithContext("failed to list current releases for rollback", map[string]interface{}{
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
				r.logger.InfoWithContext("rolling back release", map[string]interface{}{
					"release":          currentRelease.Name,
					"namespace":        currentRelease.Namespace,
					"current_revision": currentRelease.Revision,
					"target_revision":  checkpointRelease.Revision,
				})

				err := r.helmGateway.RollbackRelease(ctx, currentRelease.Name, currentRelease.Namespace, checkpointRelease.Revision)
				if err != nil {
					return fmt.Errorf("failed to rollback release %s in namespace %s: %w", currentRelease.Name, currentRelease.Namespace, err)
				}
			}
		} else {
			// Release didn't exist in checkpoint, uninstall it
			r.logger.InfoWithContext("uninstalling new release", map[string]interface{}{
				"release":   currentRelease.Name,
				"namespace": currentRelease.Namespace,
			})

			err := r.helmGateway.UninstallRelease(ctx, currentRelease.Name, currentRelease.Namespace)
			if err != nil {
				return fmt.Errorf("failed to uninstall release %s in namespace %s: %w", currentRelease.Name, currentRelease.Namespace, err)
			}
		}
	}

	r.logger.InfoWithContext("rollback to checkpoint completed", map[string]interface{}{
		"checkpoint_id": checkpoint.ID,
	})

	return nil
}

// DeployWithRetry deploys a chart with retry logic and cleanup between attempts
func (r *DeploymentRecoveryManager) DeployWithRetry(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions, maxRetries int, deployer func(context.Context, domain.Chart, *domain.DeploymentOptions) error) error {
	r.logger.InfoWithContext("starting chart deployment with retry", map[string]interface{}{
		"chart":       chart.Name,
		"max_retries": maxRetries,
	})

	for attempt := 1; attempt <= maxRetries; attempt++ {
		r.logger.InfoWithContext("attempting chart deployment", map[string]interface{}{
			"chart":       chart.Name,
			"attempt":     attempt,
			"max_retries": maxRetries,
		})

		err := deployer(ctx, chart, options)
		if err == nil {
			r.logger.InfoWithContext("chart deployment successful", map[string]interface{}{
				"chart":   chart.Name,
				"attempt": attempt,
			})
			return nil
		}

		r.logger.WarnWithContext("chart deployment failed", map[string]interface{}{
			"chart":   chart.Name,
			"attempt": attempt,
			"error":   err.Error(),
		})

		// Cleanup failed deployment before next attempt
		if attempt < maxRetries {
			cleanupErr := r.cleanupFailedDeployment(ctx, chart, options)
			if cleanupErr != nil {
				r.logger.WarnWithContext("cleanup failed", map[string]interface{}{
					"chart":         chart.Name,
					"attempt":       attempt,
					"cleanup_error": cleanupErr.Error(),
				})
			} else {
				r.logger.InfoWithContext("cleanup completed", map[string]interface{}{
					"chart":   chart.Name,
					"attempt": attempt,
				})
			}

			// Exponential backoff
			backoffDuration := time.Duration(attempt) * 10 * time.Second
			r.logger.InfoWithContext("waiting before retry", map[string]interface{}{
				"chart":            chart.Name,
				"attempt":          attempt,
				"backoff_duration": backoffDuration.String(),
			})
			time.Sleep(backoffDuration)
		}
	}

	return fmt.Errorf("chart deployment failed after %d attempts: %s", maxRetries, chart.Name)
}

// cleanupFailedDeployment cleans up a failed chart deployment
func (r *DeploymentRecoveryManager) cleanupFailedDeployment(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	r.logger.InfoWithContext("starting cleanup of failed deployment", map[string]interface{}{
		"chart": chart.Name,
	})

	namespace := r.getNamespaceForChart(chart)

	// Check if release exists
	releases, err := r.helmGateway.ListReleases(ctx, namespace)
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
		r.logger.InfoWithContext("no release found to cleanup", map[string]interface{}{
			"chart":     chart.Name,
			"namespace": namespace,
		})
		return nil
	}

	// Check release status
	if releaseToCleanup.Status == "failed" || releaseToCleanup.Status == "pending-install" || releaseToCleanup.Status == "pending-upgrade" {
		r.logger.InfoWithContext("uninstalling failed release", map[string]interface{}{
			"release":   releaseToCleanup.Name,
			"namespace": namespace,
			"status":    releaseToCleanup.Status,
		})

		err := r.helmGateway.UninstallRelease(ctx, releaseToCleanup.Name, namespace)
		if err != nil {
			return fmt.Errorf("failed to uninstall failed release %s: %w", releaseToCleanup.Name, err)
		}

		r.logger.InfoWithContext("failed release uninstalled", map[string]interface{}{
			"release":   releaseToCleanup.Name,
			"namespace": namespace,
		})
	}

	return nil
}

// CleanupFailedDeployments cleans up resources from failed deployments
func (r *DeploymentRecoveryManager) CleanupFailedDeployments(ctx context.Context, olderThan time.Duration) error {
	r.logger.InfoWithContext("cleaning up failed deployments", map[string]interface{}{
		"older_than": olderThan.String(),
	})

	// Get all namespaces to scan
	namespaces := []string{"alt-apps", "alt-auth", "alt-database", "alt-ingress", "alt-search"}

	var cleanedReleases []string
	cutoffTime := time.Now().Add(-olderThan)

	for _, namespace := range namespaces {
		releases, err := r.helmGateway.ListReleases(ctx, namespace)
		if err != nil {
			r.logger.WarnWithContext("failed to list releases for cleanup", map[string]interface{}{
				"namespace": namespace,
				"error":     err.Error(),
			})
			continue
		}

		for _, release := range releases {
			// Check if release is failed and older than cutoff time
			if (release.Status == "failed" || release.Status == "pending-install" || release.Status == "pending-upgrade") &&
				release.Updated.Before(cutoffTime) {

				r.logger.InfoWithContext("cleaning up old failed release", map[string]interface{}{
					"release":      release.Name,
					"namespace":    namespace,
					"status":       release.Status,
					"last_updated": release.Updated,
				})

				err := r.helmGateway.UninstallRelease(ctx, release.Name, namespace)
				if err != nil {
					r.logger.ErrorWithContext("failed to cleanup release", map[string]interface{}{
						"release":   release.Name,
						"namespace": namespace,
						"error":     err.Error(),
					})
				} else {
					cleanedReleases = append(cleanedReleases, fmt.Sprintf("%s/%s", namespace, release.Name))
				}
			}
		}
	}

	r.logger.InfoWithContext("failed deployment cleanup completed", map[string]interface{}{
		"cleaned_releases": cleanedReleases,
		"cleanup_count":    len(cleanedReleases),
	})

	return nil
}

// RecoverFromError attempts to recover from a deployment error
func (r *DeploymentRecoveryManager) RecoverFromError(ctx context.Context, err error, chart domain.Chart, options *domain.DeploymentOptions) error {
	r.logger.InfoWithContext("attempting error recovery", map[string]interface{}{
		"chart": chart.Name,
		"error": err.Error(),
	})

	// Check if error is recoverable
	if !r.isRecoverableError(err) {
		return fmt.Errorf("error is not recoverable: %w", err)
	}

	// Attempt specific recovery strategies based on error type
	if r.isSecretOwnershipError(err) {
		return r.recoverFromSecretOwnershipError(ctx, chart, options)
	}

	if r.isTimeoutError(err) {
		return r.recoverFromTimeoutError(ctx, chart, options)
	}

	if r.isResourceConflictError(err) {
		return r.recoverFromResourceConflictError(ctx, chart, options)
	}

	return fmt.Errorf("no recovery strategy available for error: %w", err)
}

// isRecoverableError determines if an error can be recovered from
func (r *DeploymentRecoveryManager) isRecoverableError(err error) bool {
	errorMsg := err.Error()

	recoverablePatterns := []string{
		"secret ownership",
		"timeout",
		"resource conflict",
		"temporary failure",
		"connection refused",
	}

	for _, pattern := range recoverablePatterns {
		if containsInsensitive(errorMsg, pattern) {
			return true
		}
	}

	return false
}

// isSecretOwnershipError checks if the error is related to secret ownership
func (r *DeploymentRecoveryManager) isSecretOwnershipError(err error) bool {
	return containsInsensitive(err.Error(), "secret ownership") || containsInsensitive(err.Error(), "secret already exists")
}

// isTimeoutError checks if the error is a timeout error
func (r *DeploymentRecoveryManager) isTimeoutError(err error) bool {
	return containsInsensitive(err.Error(), "timeout") || containsInsensitive(err.Error(), "deadline exceeded")
}

// isResourceConflictError checks if the error is a resource conflict error
func (r *DeploymentRecoveryManager) isResourceConflictError(err error) bool {
	return containsInsensitive(err.Error(), "conflict") || containsInsensitive(err.Error(), "already exists")
}

// recoverFromSecretOwnershipError attempts to recover from secret ownership errors
func (r *DeploymentRecoveryManager) recoverFromSecretOwnershipError(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	r.logger.InfoWithContext("attempting recovery from secret ownership error", map[string]interface{}{
		"chart": chart.Name,
	})

	// This would delegate to the secret management usecase
	// For now, return a placeholder implementation
	return fmt.Errorf("secret ownership error recovery not yet implemented")
}

// recoverFromTimeoutError attempts to recover from timeout errors
func (r *DeploymentRecoveryManager) recoverFromTimeoutError(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	r.logger.InfoWithContext("attempting recovery from timeout error", map[string]interface{}{
		"chart": chart.Name,
	})

	// Wait for a bit and then check if the deployment actually succeeded
	time.Sleep(30 * time.Second)

	// Check release status
	namespace := r.getNamespaceForChart(chart)
	releases, err := r.helmGateway.ListReleases(ctx, namespace)
	if err != nil {
		return fmt.Errorf("failed to check release status after timeout: %w", err)
	}

	for _, release := range releases {
		if release.Name == chart.Name && release.Status == "deployed" {
			r.logger.InfoWithContext("deployment actually succeeded despite timeout", map[string]interface{}{
				"chart":          chart.Name,
				"release_status": release.Status,
			})
			return nil
		}
	}

	return fmt.Errorf("timeout error recovery failed - deployment still not successful")
}

// recoverFromResourceConflictError attempts to recover from resource conflict errors
func (r *DeploymentRecoveryManager) recoverFromResourceConflictError(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	r.logger.InfoWithContext("attempting recovery from resource conflict error", map[string]interface{}{
		"chart": chart.Name,
	})

	// Try to cleanup conflicting resources and retry
	if err := r.cleanupFailedDeployment(ctx, chart, options); err != nil {
		return fmt.Errorf("failed to cleanup conflicting resources: %w", err)
	}

	return nil
}

// getNamespaceForChart returns the appropriate namespace for a chart
func (r *DeploymentRecoveryManager) getNamespaceForChart(chart domain.Chart) string {
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

// Note: Utility functions moved to shared_utils.go to avoid duplication
