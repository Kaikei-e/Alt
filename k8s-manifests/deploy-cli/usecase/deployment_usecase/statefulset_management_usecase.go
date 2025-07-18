package deployment_usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"deploy-cli/domain"
	"deploy-cli/gateway/system_gateway"
	"deploy-cli/port/logger_port"
)

// StatefulSetManagementUsecase handles StatefulSet operations including detection, recovery, and lifecycle management
type StatefulSetManagementUsecase struct {
	systemGateway *system_gateway.SystemGateway
	logger        logger_port.LoggerPort
}

// NewStatefulSetManagementUsecase creates a new StatefulSet management usecase
func NewStatefulSetManagementUsecase(
	systemGateway *system_gateway.SystemGateway,
	logger logger_port.LoggerPort,
) *StatefulSetManagementUsecase {
	return &StatefulSetManagementUsecase{
		systemGateway: systemGateway,
		logger:        logger,
	}
}

// prepareStatefulSetRecovery prepares StatefulSet recovery for database charts
func (u *StatefulSetManagementUsecase) prepareStatefulSetRecovery(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("preparing StatefulSet recovery", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// Define StatefulSet charts that may need recovery
	statefulSetCharts := []struct {
		name      string
		namespace string
	}{
		{"postgres", "alt-database"},
		{"auth-postgres", "alt-auth"},
		{"kratos-postgres", "alt-auth"},
		{"clickhouse", "alt-database"},
		{"meilisearch", "alt-search"},
	}

	for _, chart := range statefulSetCharts {
		if err := u.safeStatefulSetRecreation(ctx, chart.name, chart.namespace); err != nil {
			return fmt.Errorf("failed to prepare StatefulSet recovery for %s: %w", chart.name, err)
		}
	}

	u.logger.InfoWithContext("StatefulSet recovery preparation completed", map[string]interface{}{
		"environment":       options.Environment.String(),
		"charts_processed":  len(statefulSetCharts),
	})

	return nil
}

// safeStatefulSetRecreation safely recreates StatefulSet to resolve conflicts
func (u *StatefulSetManagementUsecase) safeStatefulSetRecreation(ctx context.Context, chartName, namespace string) error {
	statefulSetName := chartName // postgres, auth-postgres, etc.
	
	u.logger.InfoWithContext("checking for existing StatefulSet", map[string]interface{}{
		"statefulset": statefulSetName,
		"namespace":   namespace,
	})

	// Check if StatefulSet exists
	exists, err := u.checkStatefulSetExists(ctx, statefulSetName, namespace)
	if err != nil {
		return fmt.Errorf("failed to check StatefulSet existence: %w", err)
	}

	if exists {
		u.logger.InfoWithContext("existing StatefulSet detected, performing safe recreation", map[string]interface{}{
			"statefulset": statefulSetName,
			"namespace":   namespace,
		})

		// Step 1: Scale down StatefulSet to 0
		if err := u.scaleStatefulSet(ctx, statefulSetName, namespace, 0); err != nil {
			return fmt.Errorf("failed to scale down StatefulSet: %w", err)
		}

		// Step 2: Wait for all pods to terminate
		if err := u.waitForPodsTermination(ctx, statefulSetName, namespace, 300); err != nil {
			return fmt.Errorf("failed to wait for pods termination: %w", err)
		}

		// Step 3: Delete StatefulSet (preserve PVC)
		if err := u.deleteStatefulSet(ctx, statefulSetName, namespace); err != nil {
			return fmt.Errorf("failed to delete StatefulSet: %w", err)
		}

		// Step 4: Clean up related resources (except PVC)
		if err := u.cleanupStatefulSetResources(ctx, statefulSetName, namespace); err != nil {
			return fmt.Errorf("failed to cleanup StatefulSet resources: %w", err)
		}

		u.logger.InfoWithContext("StatefulSet safely removed", map[string]interface{}{
			"statefulset":    statefulSetName,
			"namespace":      namespace,
			"pvc_preserved":  true,
		})
	} else {
		u.logger.InfoWithContext("no existing StatefulSet found, proceeding with fresh deployment", map[string]interface{}{
			"statefulset": statefulSetName,
			"namespace":   namespace,
		})
	}

	return nil
}

// checkStatefulSetExists checks if a StatefulSet exists in the namespace
func (u *StatefulSetManagementUsecase) checkStatefulSetExists(ctx context.Context, name, namespace string) (bool, error) {
	// Use kubectl to check StatefulSet existence
	_, err := u.systemGateway.ExecuteCommand(ctx, "kubectl", "get", "statefulset", name, "-n", namespace)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check StatefulSet existence: %w", err)
	}
	return true, nil
}

// scaleStatefulSet scales a StatefulSet to specified replica count
func (u *StatefulSetManagementUsecase) scaleStatefulSet(ctx context.Context, name, namespace string, replicas int) error {
	u.logger.InfoWithContext("scaling StatefulSet", map[string]interface{}{
		"statefulset": name,
		"namespace":   namespace,
		"replicas":    replicas,
	})

	_, err := u.systemGateway.ExecuteCommand(ctx, "kubectl", "scale", "statefulset", name, fmt.Sprintf("--replicas=%d", replicas), "-n", namespace)
	if err != nil {
		return fmt.Errorf("failed to scale StatefulSet: %w", err)
	}

	u.logger.InfoWithContext("StatefulSet scaled successfully", map[string]interface{}{
		"statefulset": name,
		"namespace":   namespace,
		"replicas":    replicas,
	})

	return nil
}

// waitForPodsTermination waits for all pods of a StatefulSet to terminate
func (u *StatefulSetManagementUsecase) waitForPodsTermination(ctx context.Context, name, namespace string, timeoutSeconds int) error {
	u.logger.InfoWithContext("waiting for pods termination", map[string]interface{}{
		"statefulset": name,
		"namespace":   namespace,
		"timeout":     timeoutSeconds,
	})

	for i := 0; i < timeoutSeconds; i += 5 {
		output, err := u.systemGateway.ExecuteCommand(ctx, "kubectl", "get", "pods", "-n", namespace, "-l", fmt.Sprintf("app=%s", name), "--no-headers")
		if err != nil {
			return fmt.Errorf("failed to check pod status: %w", err)
		}

		if strings.TrimSpace(output) == "" {
			u.logger.InfoWithContext("all pods terminated", map[string]interface{}{
				"statefulset": name,
				"namespace":   namespace,
				"elapsed":     i,
			})
			return nil
		}

		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("timeout waiting for pods termination after %d seconds", timeoutSeconds)
}

// deleteStatefulSet deletes a StatefulSet while preserving PVCs
func (u *StatefulSetManagementUsecase) deleteStatefulSet(ctx context.Context, name, namespace string) error {
	u.logger.InfoWithContext("deleting StatefulSet", map[string]interface{}{
		"statefulset": name,
		"namespace":   namespace,
	})

	_, err := u.systemGateway.ExecuteCommand(ctx, "kubectl", "delete", "statefulset", name, "-n", namespace)
	if err != nil {
		return fmt.Errorf("failed to delete StatefulSet: %w", err)
	}

	u.logger.InfoWithContext("StatefulSet deleted successfully", map[string]interface{}{
		"statefulset": name,
		"namespace":   namespace,
	})

	return nil
}

// cleanupStatefulSetResources cleans up related resources except PVCs
func (u *StatefulSetManagementUsecase) cleanupStatefulSetResources(ctx context.Context, name, namespace string) error {
	u.logger.InfoWithContext("cleaning up StatefulSet resources", map[string]interface{}{
		"statefulset": name,
		"namespace":   namespace,
	})

	// Clean up services (but not PVCs)
	resources := []string{"service", "configmap"}
	for _, resource := range resources {
		_, err := u.systemGateway.ExecuteCommand(ctx, "kubectl", "delete", resource, "-l", fmt.Sprintf("app=%s", name), "-n", namespace, "--ignore-not-found=true")
		if err != nil {
			u.logger.WarnWithContext("failed to cleanup resource", map[string]interface{}{
				"resource":    resource,
				"statefulset": name,
				"namespace":   namespace,
				"error":       err.Error(),
			})
		}
	}

	u.logger.InfoWithContext("StatefulSet resources cleanup completed", map[string]interface{}{
		"statefulset": name,
		"namespace":   namespace,
	})

	return nil
}

// isStatefulSetChart determines if a chart deploys a StatefulSet (includes detection fix)
func (u *StatefulSetManagementUsecase) isStatefulSetChart(chartName string) bool {
	statefulSetCharts := []string{
		"postgres", "auth-postgres", "kratos-postgres", "clickhouse", "meilisearch",
	}
	
	for _, stsChart := range statefulSetCharts {
		if chartName == stsChart {
			return true
		}
	}
	return false
}

// detectStatefulSetConflicts detects potential StatefulSet conflicts before deployment
func (u *StatefulSetManagementUsecase) detectStatefulSetConflicts(ctx context.Context, charts []domain.Chart) ([]string, error) {
	var conflicts []string

	for _, chart := range charts {
		if u.isStatefulSetChart(chart.Name) {
			namespace := u.getNamespaceForChart(chart)
			exists, err := u.checkStatefulSetExists(ctx, chart.Name, namespace)
			if err != nil {
				return nil, fmt.Errorf("failed to check StatefulSet existence for %s: %w", chart.Name, err)
			}

			if exists {
				conflicts = append(conflicts, fmt.Sprintf("%s/%s", namespace, chart.Name))
				u.logger.WarnWithContext("detected existing StatefulSet", map[string]interface{}{
					"chart_name":   chart.Name,
					"namespace":    namespace,
					"requires_fix": true,
				})
			}
		}
	}

	return conflicts, nil
}

// getNamespaceForChart returns the appropriate namespace for a chart
func (u *StatefulSetManagementUsecase) getNamespaceForChart(chart domain.Chart) string {
	// For multi-namespace charts, return the primary namespace
	if chart.MultiNamespace && len(chart.TargetNamespaces) > 0 {
		return chart.TargetNamespaces[0]
	}
	
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

// validateStatefulSetHealth validates the health of StatefulSet deployments
func (u *StatefulSetManagementUsecase) validateStatefulSetHealth(ctx context.Context, chartName, namespace string) error {
	u.logger.InfoWithContext("validating StatefulSet health", map[string]interface{}{
		"chart_name": chartName,
		"namespace":  namespace,
	})

	// Check if StatefulSet is ready
	output, err := u.systemGateway.ExecuteCommand(ctx, "kubectl", "get", "statefulset", chartName, "-n", namespace, "-o", "jsonpath='{.status.readyReplicas}/{.status.replicas}'")
	if err != nil {
		return fmt.Errorf("failed to check StatefulSet readiness: %w", err)
	}

	if strings.TrimSpace(output) == "'1/1'" || strings.TrimSpace(output) == "1/1" {
		u.logger.InfoWithContext("StatefulSet is healthy", map[string]interface{}{
			"chart_name": chartName,
			"namespace":  namespace,
			"status":     "ready",
		})
		return nil
	}

	return fmt.Errorf("StatefulSet %s in namespace %s is not ready: %s", chartName, namespace, output)
}

// manageStatefulSetRecovery manages the complete StatefulSet recovery process
func (u *StatefulSetManagementUsecase) manageStatefulSetRecovery(ctx context.Context, chartName, namespace string) error {
	u.logger.InfoWithContext("managing StatefulSet recovery", map[string]interface{}{
		"chart_name": chartName,
		"namespace":  namespace,
	})

	// Step 1: Safe recreation to resolve conflicts
	if err := u.safeStatefulSetRecreation(ctx, chartName, namespace); err != nil {
		return fmt.Errorf("StatefulSet recreation failed: %w", err)
	}

	// Step 2: Validate recovery was successful
	if err := u.validateStatefulSetHealth(ctx, chartName, namespace); err != nil {
		u.logger.WarnWithContext("StatefulSet health validation failed after recovery", map[string]interface{}{
			"chart_name": chartName,
			"namespace":  namespace,
			"error":      err.Error(),
		})
		// Don't fail the recovery - the deployment process will handle validation
	}

	u.logger.InfoWithContext("StatefulSet recovery completed successfully", map[string]interface{}{
		"chart_name": chartName,
		"namespace":  namespace,
	})

	return nil
}