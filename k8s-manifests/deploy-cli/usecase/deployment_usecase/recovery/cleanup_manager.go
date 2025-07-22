package recovery

import (
	"context"
	"fmt"
	"strings"

	"deploy-cli/domain"
	"deploy-cli/gateway/helm_gateway"
	"deploy-cli/gateway/kubectl_gateway"
	"deploy-cli/port/logger_port"
)

// CleanupManager handles cleanup operations for failed deployments
type CleanupManager struct {
	helmGateway    *helm_gateway.HelmGateway
	kubectlGateway *kubectl_gateway.KubectlGateway
	logger         logger_port.LoggerPort
}

// CleanupManagerPort defines the interface for cleanup management
type CleanupManagerPort interface {
	CleanupFailedDeployment(ctx context.Context, deploymentID string) error
	CleanupChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error
	CleanupNamespace(ctx context.Context, namespace string) error
	CleanupOrphanedResources(ctx context.Context, namespaces []string) error
	ForceCleanup(ctx context.Context, deploymentID string) error
}

// NewCleanupManager creates a new cleanup manager
func NewCleanupManager(
	helmGateway *helm_gateway.HelmGateway,
	kubectlGateway *kubectl_gateway.KubectlGateway,
	logger logger_port.LoggerPort,
) *CleanupManager {
	return &CleanupManager{
		helmGateway:    helmGateway,
		kubectlGateway: kubectlGateway,
		logger:         logger,
	}
}

// CleanupFailedDeployment cleans up resources from a failed deployment
func (c *CleanupManager) CleanupFailedDeployment(ctx context.Context, deploymentID string) error {
	c.logger.InfoWithContext("starting cleanup of failed deployment", map[string]interface{}{
		"deployment_id": deploymentID,
	})

	// For now, perform general cleanup operations
	// In a full implementation, we would track deployment resources and clean them up specifically

	// Clean up failed pods and resources in common namespaces
	namespaces := []string{"alt-apps", "alt-database", "alt-search", "alt-auth", "alt-ingress"}
	
	for _, namespace := range namespaces {
		if err := c.cleanupFailedResourcesInNamespace(ctx, namespace); err != nil {
			c.logger.ErrorWithContext("failed to cleanup namespace resources", map[string]interface{}{
				"deployment_id": deploymentID,
				"namespace":     namespace,
				"error":         err.Error(),
			})
			// Continue with other namespaces even if one fails
		}
	}

	c.logger.InfoWithContext("cleanup of failed deployment completed", map[string]interface{}{
		"deployment_id": deploymentID,
	})

	return nil
}

// CleanupChart cleans up a specific chart deployment
func (c *CleanupManager) CleanupChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	c.logger.InfoWithContext("cleaning up chart", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
	})

	namespace := options.GetNamespace(chart.Name)

	// First try to uninstall via Helm if it's a Helm-managed chart
	err := c.helmGateway.UndeployChart(ctx, chart, options)
	if err != nil {
		c.logger.WarnWithContext("helm cleanup failed, attempting manual cleanup", map[string]interface{}{
			"chart":     chart.Name,
			"namespace": namespace,
			"error":     err.Error(),
		})

		// Manual cleanup of resources
		if err := c.cleanupChartResources(ctx, chart, namespace); err != nil {
			return fmt.Errorf("manual cleanup failed: %w", err)
		}
	}

	c.logger.InfoWithContext("chart cleanup completed", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": namespace,
	})

	return nil
}

// CleanupNamespace cleans up all resources in a namespace
func (c *CleanupManager) CleanupNamespace(ctx context.Context, namespace string) error {
	c.logger.InfoWithContext("cleaning up namespace", map[string]interface{}{
		"namespace": namespace,
	})

	return c.kubectlGateway.CleanupNamespace(ctx, namespace)
}

// CleanupOrphanedResources cleans up orphaned resources across namespaces
func (c *CleanupManager) CleanupOrphanedResources(ctx context.Context, namespaces []string) error {
	c.logger.InfoWithContext("cleaning up orphaned resources", map[string]interface{}{
		"namespaces": namespaces,
	})

	for _, namespace := range namespaces {
		if err := c.cleanupOrphanedResourcesInNamespace(ctx, namespace); err != nil {
			c.logger.ErrorWithContext("failed to cleanup orphaned resources in namespace", map[string]interface{}{
				"namespace": namespace,
				"error":     err.Error(),
			})
			// Continue with other namespaces
		}
	}

	c.logger.InfoWithContext("orphaned resources cleanup completed", map[string]interface{}{
		"namespaces": namespaces,
	})

	return nil
}

// ForceCleanup performs aggressive cleanup of all deployment-related resources
func (c *CleanupManager) ForceCleanup(ctx context.Context, deploymentID string) error {
	c.logger.WarnWithContext("performing force cleanup", map[string]interface{}{
		"deployment_id": deploymentID,
	})

	// Perform aggressive cleanup
	if err := c.helmGateway.EmergencyCleanupAllReleases(ctx); err != nil {
		c.logger.ErrorWithContext("emergency Helm cleanup failed", map[string]interface{}{
			"deployment_id": deploymentID,
			"error":         err.Error(),
		})
	}

	// Clean up failed resources in all namespaces
	namespaces := []string{"alt-apps", "alt-database", "alt-search", "alt-auth", "alt-ingress", "alt-observability"}
	if err := c.kubectlGateway.CleanupFailedResources(ctx, namespaces); err != nil {
		c.logger.ErrorWithContext("kubectl cleanup failed", map[string]interface{}{
			"deployment_id": deploymentID,
			"error":         err.Error(),
		})
	}

	c.logger.WarnWithContext("force cleanup completed", map[string]interface{}{
		"deployment_id": deploymentID,
	})

	return nil
}

// Helper methods

func (c *CleanupManager) cleanupFailedResourcesInNamespace(ctx context.Context, namespace string) error {
	c.logger.DebugWithContext("cleaning up failed resources in namespace", map[string]interface{}{
		"namespace": namespace,
	})

	// Get problematic pods
	pods, err := c.kubectlGateway.GetProblematicPods(ctx)
	if err != nil {
		return fmt.Errorf("failed to get problematic pods: %w", err)
	}

	// Delete problematic pods in this namespace
	for _, pod := range pods {
		if pod.Namespace == namespace && c.isPodProblematic(pod.Status) {
			c.logger.DebugWithContext("deleting problematic pod", map[string]interface{}{
				"pod":       pod.Name,
				"namespace": pod.Namespace,
				"status":    pod.Status,
			})

			if err := c.kubectlGateway.DeleteResource(ctx, "pod", pod.Name, pod.Namespace); err != nil {
				c.logger.WarnWithContext("failed to delete problematic pod", map[string]interface{}{
					"pod":       pod.Name,
					"namespace": pod.Namespace,
					"error":     err.Error(),
				})
			}
		}
	}

	return nil
}

func (c *CleanupManager) cleanupChartResources(ctx context.Context, chart domain.Chart, namespace string) error {
	c.logger.DebugWithContext("manually cleaning up chart resources", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": namespace,
	})

	// Delete deployments related to the chart
	deployments, err := c.kubectlGateway.GetDeploymentsForChart(ctx, chart.Name, namespace)
	if err != nil {
		c.logger.WarnWithContext("failed to get deployments for chart", map[string]interface{}{
			"chart":     chart.Name,
			"namespace": namespace,
			"error":     err.Error(),
		})
	} else {
		for _, deployment := range deployments {
			if err := c.kubectlGateway.DeleteResource(ctx, "deployment", deployment.Name, namespace); err != nil {
				c.logger.WarnWithContext("failed to delete deployment", map[string]interface{}{
					"deployment": deployment.Name,
					"namespace":  namespace,
					"error":      err.Error(),
				})
			}
		}
	}

	// Delete stateful sets related to the chart
	statefulSets, err := c.kubectlGateway.GetStatefulSetsForChart(ctx, chart.Name, namespace)
	if err != nil {
		c.logger.WarnWithContext("failed to get stateful sets for chart", map[string]interface{}{
			"chart":     chart.Name,
			"namespace": namespace,
			"error":     err.Error(),
		})
	} else {
		for _, statefulSet := range statefulSets {
			if err := c.kubectlGateway.DeleteResource(ctx, "statefulset", statefulSet.Name, namespace); err != nil {
				c.logger.WarnWithContext("failed to delete stateful set", map[string]interface{}{
					"statefulset": statefulSet.Name,
					"namespace":   namespace,
					"error":       err.Error(),
				})
			}
		}
	}

	return nil
}

func (c *CleanupManager) cleanupOrphanedResourcesInNamespace(ctx context.Context, namespace string) error {
	c.logger.DebugWithContext("cleaning up orphaned resources in namespace", map[string]interface{}{
		"namespace": namespace,
	})

	// Get all secrets and identify orphaned ones (secrets without associated deployments)
	secrets, err := c.kubectlGateway.GetSecrets(ctx, namespace)
	if err != nil {
		return fmt.Errorf("failed to get secrets: %w", err)
	}

	// Get all deployments to compare against
	deployments, err := c.kubectlGateway.GetDeployments(ctx, namespace)
	if err != nil {
		return fmt.Errorf("failed to get deployments: %w", err)
	}

	// Create a map of deployment names for quick lookup
	deploymentNames := make(map[string]bool)
	for _, deployment := range deployments {
		deploymentNames[deployment.Name] = true
	}

	// Check for orphaned secrets (simple heuristic: secrets that don't match any deployment name)
	for _, secret := range secrets {
		isOrphaned := true
		for deploymentName := range deploymentNames {
			if strings.Contains(secret.Name, deploymentName) {
				isOrphaned = false
				break
			}
		}

		if isOrphaned && c.isSecretSafeToDelete(secret.Name) {
			c.logger.DebugWithContext("deleting orphaned secret", map[string]interface{}{
				"secret":    secret.Name,
				"namespace": namespace,
			})

			if err := c.kubectlGateway.DeleteSecret(ctx, secret.Name, namespace); err != nil {
				c.logger.WarnWithContext("failed to delete orphaned secret", map[string]interface{}{
					"secret":    secret.Name,
					"namespace": namespace,
					"error":     err.Error(),
				})
			}
		}
	}

	return nil
}

// Helper methods for validation

func (c *CleanupManager) isPodProblematic(status string) bool {
	problematicStatuses := []string{
		"Failed", "Error", "CrashLoopBackOff", "ImagePullBackOff", 
		"ErrImagePull", "InvalidImageName", "Terminating",
	}

	for _, problematicStatus := range problematicStatuses {
		if strings.Contains(status, problematicStatus) {
			return true
		}
	}
	return false
}

func (c *CleanupManager) isSecretSafeToDelete(secretName string) bool {
	// Don't delete system secrets or important secrets
	protectedSecrets := []string{
		"default-token", "kube-system", "kubernetes.io", 
		"common-secrets", "tls-secret", "registry-secret",
	}

	secretLower := strings.ToLower(secretName)
	for _, protected := range protectedSecrets {
		if strings.Contains(secretLower, protected) {
			return false
		}
	}

	return true
}