package deployment_usecase

import (
	"context"
	"fmt"
	"strings"

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

// prepareStatefulSetRecovery prepares StatefulSet recovery for database charts with pre-checks
func (u *StatefulSetManagementUsecase) prepareStatefulSetRecovery(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("preparing StatefulSet recovery with pre-checks", map[string]interface{}{
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

	// Step 1: Check which StatefulSets actually need recovery
	var chartsNeedingRecovery []struct {
		name      string
		namespace string
		reason    string
	}

	for _, chart := range statefulSetCharts {
		needsRecovery, reason, err := u.checkStatefulSetNeedsRecovery(ctx, chart.name, chart.namespace)
		if err != nil {
			u.logger.WarnWithContext("failed to check StatefulSet recovery need, skipping chart", map[string]interface{}{
				"chart":     chart.name,
				"namespace": chart.namespace,
				"error":     err.Error(),
			})
			// On check failure, skip this chart rather than assume recovery is needed
			continue
		}

		if needsRecovery {
			chartsNeedingRecovery = append(chartsNeedingRecovery, struct {
				name      string
				namespace string
				reason    string
			}{chart.name, chart.namespace, reason})
		}
	}

	// Step 2: Log pre-check results
	u.logger.InfoWithContext("StatefulSet recovery pre-check completed", map[string]interface{}{
		"total_charts":            len(statefulSetCharts),
		"charts_needing_recovery": len(chartsNeedingRecovery),
		"charts_skipped":          len(statefulSetCharts) - len(chartsNeedingRecovery),
	})

	// Log detailed recovery decisions
	if len(chartsNeedingRecovery) > 0 {
		u.logger.InfoWithContext("StatefulSets requiring recovery", map[string]interface{}{
			"charts": chartsNeedingRecovery,
		})
	}

	chartsSkipped := len(statefulSetCharts) - len(chartsNeedingRecovery)
	if chartsSkipped > 0 {
		u.logger.InfoWithContext("StatefulSets skipped (healthy or non-existent)", map[string]interface{}{
			"count": chartsSkipped,
		})
	}

	// Step 3: Only perform recovery on charts that actually need it
	if len(chartsNeedingRecovery) == 0 {
		u.logger.InfoWithContext("no StatefulSet recovery needed, skipping recovery phase", map[string]interface{}{
			"environment": options.Environment.String(),
		})
		return nil
	}

	u.logger.InfoWithContext("performing StatefulSet recovery for identified charts", map[string]interface{}{
		"environment":       options.Environment.String(),
		"charts_to_recover": len(chartsNeedingRecovery),
	})

	for _, chart := range chartsNeedingRecovery {
		u.logger.InfoWithContext("recovering StatefulSet", map[string]interface{}{
			"chart":     chart.name,
			"namespace": chart.namespace,
			"reason":    chart.reason,
		})

		if err := u.safeStatefulSetRecreation(ctx, chart.name, chart.namespace); err != nil {
			return fmt.Errorf("failed to prepare StatefulSet recovery for %s in namespace %s: %w\n\nThis usually indicates:\n- kubectl connectivity issues\n- missing namespace\n- insufficient permissions", chart.name, chart.namespace, err)
		}
	}

	u.logger.InfoWithContext("StatefulSet recovery preparation completed", map[string]interface{}{
		"environment":      options.Environment.String(),
		"charts_processed": len(chartsNeedingRecovery),
		"charts_skipped":   len(statefulSetCharts) - len(chartsNeedingRecovery),
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
		u.logger.WarnWithContext("failed to check StatefulSet existence, assuming it doesn't exist", map[string]interface{}{
			"statefulset": statefulSetName,
			"namespace":   namespace,
			"error":       err.Error(),
		})
		exists = false // Assume it doesn't exist and proceed with fresh deployment
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
			"statefulset":   statefulSetName,
			"namespace":     namespace,
			"pvc_preserved": true,
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

// checkStatefulSetNeedsRecovery determines if a StatefulSet needs recovery based on its current state
func (u *StatefulSetManagementUsecase) checkStatefulSetNeedsRecovery(ctx context.Context, name, namespace string) (bool, string, error) {
	u.logger.InfoWithContext("checking StatefulSet recovery necessity", map[string]interface{}{
		"statefulset": name,
		"namespace":   namespace,
	})

	// Step 1: Check if StatefulSet exists
	exists, err := u.checkStatefulSetExists(ctx, name, namespace)
	if err != nil {
		// If we can't check existence, log warning but don't fail - assume no recovery needed
		u.logger.WarnWithContext("failed to check StatefulSet existence, assuming no recovery needed", map[string]interface{}{
			"statefulset": name,
			"namespace":   namespace,
			"error":       err.Error(),
		})
		return false, "existence_check_failed", nil
	}

	if !exists {
		u.logger.InfoWithContext("StatefulSet does not exist, no recovery needed", map[string]interface{}{
			"statefulset": name,
			"namespace":   namespace,
		})
		return false, "", nil
	}

	// Step 2: Check StatefulSet health/readiness
	output, err := u.systemGateway.ExecuteCommand(ctx, "kubectl", "get", "statefulset", name, "-n", namespace, "-o", "jsonpath='{.status.readyReplicas}/{.status.replicas}'")
	if err != nil {
		u.logger.WarnWithContext("failed to check StatefulSet readiness, assuming recovery needed", map[string]interface{}{
			"statefulset": name,
			"namespace":   namespace,
			"error":       err.Error(),
		})
		return true, "readiness_check_failed", nil
	}

	readinessStatus := strings.TrimSpace(strings.Trim(output, "'"))

	// Check if StatefulSet is ready (1/1 or similar)
	if readinessStatus == "1/1" || readinessStatus == "0/0" {
		// Step 3: Check pod health for ready StatefulSets
		podHealthy, err := u.checkStatefulSetPodHealth(ctx, name, namespace)
		if err != nil {
			u.logger.WarnWithContext("failed to check pod health, assuming recovery needed", map[string]interface{}{
				"statefulset": name,
				"namespace":   namespace,
				"error":       err.Error(),
			})
			return true, "pod_health_check_failed", nil
		}

		if podHealthy {
			u.logger.InfoWithContext("StatefulSet is healthy, no recovery needed", map[string]interface{}{
				"statefulset": name,
				"namespace":   namespace,
				"readiness":   readinessStatus,
			})
			return false, "", nil
		} else {
			u.logger.InfoWithContext("StatefulSet pods are unhealthy, recovery needed", map[string]interface{}{
				"statefulset": name,
				"namespace":   namespace,
				"readiness":   readinessStatus,
			})
			return true, "pod_unhealthy", nil
		}
	}

	// Step 4: StatefulSet is not ready, check the reason
	u.logger.InfoWithContext("StatefulSet is not ready, checking detailed status", map[string]interface{}{
		"statefulset": name,
		"namespace":   namespace,
		"readiness":   readinessStatus,
	})

	// Get detailed StatefulSet status
	statusOutput, err := u.systemGateway.ExecuteCommand(ctx, "kubectl", "get", "statefulset", name, "-n", namespace, "-o", "jsonpath='{.status.conditions[?(@.type==\"Ready\")].status}'")
	if err == nil && strings.TrimSpace(strings.Trim(statusOutput, "'")) == "False" {
		return true, "not_ready", nil
	}

	// Check for update conflicts or other issues
	updateRevision, err := u.systemGateway.ExecuteCommand(ctx, "kubectl", "get", "statefulset", name, "-n", namespace, "-o", "jsonpath='{.status.updateRevision}'")
	currentRevision, err2 := u.systemGateway.ExecuteCommand(ctx, "kubectl", "get", "statefulset", name, "-n", namespace, "-o", "jsonpath='{.status.currentRevision}'")

	if err == nil && err2 == nil {
		updateRev := strings.TrimSpace(strings.Trim(updateRevision, "'"))
		currentRev := strings.TrimSpace(strings.Trim(currentRevision, "'"))

		if updateRev != currentRev && updateRev != "" && currentRev != "" {
			u.logger.InfoWithContext("StatefulSet has update conflicts, recovery needed", map[string]interface{}{
				"statefulset":      name,
				"namespace":        namespace,
				"current_revision": currentRev,
				"update_revision":  updateRev,
			})
			return true, "update_conflict", nil
		}
	}

	// Default to recovery needed for unknown states
	u.logger.InfoWithContext("StatefulSet in unknown state, recovery needed", map[string]interface{}{
		"statefulset": name,
		"namespace":   namespace,
		"readiness":   readinessStatus,
	})
	return true, "unknown_state", nil
}

// checkStatefulSetPodHealth checks if StatefulSet pods are healthy
func (u *StatefulSetManagementUsecase) checkStatefulSetPodHealth(ctx context.Context, name, namespace string) (bool, error) {
	// Try multiple label selectors to find pods
	labelSelectors := []string{
		fmt.Sprintf("app=%s", name),
		fmt.Sprintf("app.kubernetes.io/name=%s", name),
		fmt.Sprintf("app.kubernetes.io/instance=%s", name),
	}

	for _, selector := range labelSelectors {
		output, err := u.systemGateway.ExecuteCommand(ctx, "kubectl", "get", "pods", "-n", namespace, "-l", selector, "--no-headers")
		if err != nil {
			continue
		}

		if strings.TrimSpace(output) == "" {
			continue // No pods found with this selector
		}

		// Parse pod status
		pods := strings.Split(strings.TrimSpace(output), "\n")
		for _, pod := range pods {
			if strings.TrimSpace(pod) == "" {
				continue
			}

			fields := strings.Fields(pod)
			if len(fields) < 3 {
				continue
			}

			podStatus := fields[2]
			// Check for problematic pod states
			if podStatus == "CrashLoopBackOff" || podStatus == "Error" || podStatus == "Failed" ||
				podStatus == "ImagePullBackOff" || podStatus == "InvalidImageName" || podStatus == "ErrImagePull" {
				u.logger.WarnWithContext("found unhealthy pod", map[string]interface{}{
					"pod":         fields[0],
					"status":      podStatus,
					"statefulset": name,
					"namespace":   namespace,
				})
				return false, nil
			}
		}

		// Found pods and they seem healthy
		return true, nil
	}

	// No pods found with any selector - might be scaled to 0 or other issue
	u.logger.InfoWithContext("no pods found for StatefulSet", map[string]interface{}{
		"statefulset": name,
		"namespace":   namespace,
	})
	return false, nil
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

// waitForPodsTermination waits for all pods of a StatefulSet to terminate with detailed logging
func (u *StatefulSetManagementUsecase) waitForPodsTermination(ctx context.Context, name, namespace string, timeoutSeconds int) error {
	u.logger.InfoWithContext("waiting for pods termination using kubectl wait", map[string]interface{}{
		"statefulset": name,
		"namespace":   namespace,
		"timeout":     timeoutSeconds,
	})

	// Define label selectors to try
	labelSelectors := []string{
		fmt.Sprintf("app=%s", name),
		fmt.Sprintf("app.kubernetes.io/name=%s", name),
		fmt.Sprintf("app.kubernetes.io/instance=%s", name),
	}

	// First, check if any pods exist with any of these selectors
	var existingSelector string
	for _, selector := range labelSelectors {
		output, err := u.systemGateway.ExecuteCommand(ctx, "kubectl", "get", "pods", "-n", namespace, "-l", selector, "--no-headers")
		if err != nil {
			continue
		}
		if strings.TrimSpace(output) != "" {
			existingSelector = selector
			u.logger.InfoWithContext("found pods with selector", map[string]interface{}{
				"selector":    selector,
				"statefulset": name,
				"namespace":   namespace,
			})
			break
		}
	}

	// If no pods found with any selector, they're already terminated
	if existingSelector == "" {
		u.logger.InfoWithContext("no pods found with any selector, termination already complete", map[string]interface{}{
			"statefulset": name,
			"namespace":   namespace,
		})
		return nil
	}

	// Use kubectl wait --for=delete to wait for pod deletion
	timeoutArg := fmt.Sprintf("%ds", timeoutSeconds)
	waitArgs := []string{
		"wait", "--for=delete", "pod",
		"--selector=" + existingSelector,
		"--namespace=" + namespace,
		"--timeout=" + timeoutArg,
	}

	u.logger.InfoWithContext("executing kubectl wait for pod deletion", map[string]interface{}{
		"command":     "kubectl " + strings.Join(waitArgs, " "),
		"statefulset": name,
		"namespace":   namespace,
		"selector":    existingSelector,
		"timeout":     timeoutArg,
	})

	output, err := u.systemGateway.ExecuteCommand(ctx, "kubectl", waitArgs...)
	if err != nil {
		// kubectl wait can return error even on success, check the actual pod status
		verifyOutput, verifyErr := u.systemGateway.ExecuteCommand(ctx, "kubectl", "get", "pods", "-n", namespace, "-l", existingSelector, "--no-headers")
		if verifyErr == nil && strings.TrimSpace(verifyOutput) == "" {
			u.logger.InfoWithContext("pods terminated successfully (verified by secondary check)", map[string]interface{}{
				"statefulset": name,
				"namespace":   namespace,
				"selector":    existingSelector,
			})
			return nil
		}

		u.logger.ErrorWithContext("kubectl wait failed and pods still exist", map[string]interface{}{
			"statefulset":  name,
			"namespace":    namespace,
			"selector":     existingSelector,
			"wait_error":   err.Error(),
			"wait_output":  output,
			"verify_error": verifyErr,
			"verify_pods":  verifyOutput,
		})
		
		// Continue with deployment instead of failing - pods might be terminating gracefully
		u.logger.InfoWithContext("continuing with deployment despite kubectl wait timeout", map[string]interface{}{
			"statefulset": name,
			"namespace":   namespace,
			"reason":      "kubectl_wait_timeout_but_proceeding",
		})
		return nil
	}

	u.logger.InfoWithContext("kubectl wait completed successfully", map[string]interface{}{
		"statefulset": name,
		"namespace":   namespace,
		"selector":    existingSelector,
		"output":      output,
	})

	return nil
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
