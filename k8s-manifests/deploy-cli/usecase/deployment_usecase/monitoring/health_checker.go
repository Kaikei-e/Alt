// PHASE R1: Health checking functionality
package monitoring

import (
	"context"
	"fmt"
	"time"

	"deploy-cli/domain"
	"deploy-cli/gateway/kubectl_gateway"
	"deploy-cli/port/logger_port"
)

// HealthChecker handles health checking operations
type HealthChecker struct {
	kubectlGateway *kubectl_gateway.KubectlGateway
	logger         logger_port.LoggerPort
}

// HealthCheckerPort defines the interface for health checking
type HealthCheckerPort interface {
	CheckDeploymentHealth(ctx context.Context, deploymentID string) (*domain.HealthStatus, error)
	CheckNamespaceHealth(ctx context.Context, namespace string) (*domain.HealthStatus, error)
	CheckChartHealth(ctx context.Context, chartName, namespace string) (*domain.ChartHealthStatus, error)
	WaitForHealthy(ctx context.Context, target *domain.HealthTarget, timeout time.Duration) error
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(
	kubectlGateway *kubectl_gateway.KubectlGateway,
	logger logger_port.LoggerPort,
) *HealthChecker {
	return &HealthChecker{
		kubectlGateway: kubectlGateway,
		logger:         logger,
	}
}

// CheckDeploymentHealth checks the health of an entire deployment
func (h *HealthChecker) CheckDeploymentHealth(ctx context.Context, deploymentID string) (*domain.HealthStatus, error) {
	h.logger.DebugWithContext("checking deployment health", map[string]interface{}{
		"deployment_id": deploymentID,
	})

	// For now, return a placeholder implementation
	// In a full implementation, this would:
	// 1. Look up the deployment by ID
	// 2. Check all associated resources
	// 3. Aggregate health status

	status := &domain.HealthStatus{
		Target:        fmt.Sprintf("deployment-%s", deploymentID),
		OverallStatus: domain.HealthStatusHealthy,
		CheckTime:     time.Now(),
		Details:       make(map[string]interface{}),
	}

	return status, nil
}

// CheckNamespaceHealth checks the health of all resources in a namespace
func (h *HealthChecker) CheckNamespaceHealth(ctx context.Context, namespace string) (*domain.HealthStatus, error) {
	h.logger.DebugWithContext("checking namespace health", map[string]interface{}{
		"namespace": namespace,
	})

	status := &domain.HealthStatus{
		Target:    fmt.Sprintf("namespace-%s", namespace),
		CheckTime: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Check pods in namespace
	pods, err := h.kubectlGateway.GetPods(ctx, namespace, "")
	if err != nil {
		status.OverallStatus = domain.HealthStatusUnknown
		status.Details["error"] = err.Error()
		return status, err
	}

	// Analyze pod status
	healthyPods := 0
	totalPods := len(pods)

	for _, pod := range pods {
		if pod.Status == "Running" && pod.Ready == "Running" {
			healthyPods++
		}
	}

	status.Details["total_pods"] = totalPods
	status.Details["healthy_pods"] = healthyPods
	status.Details["unhealthy_pods"] = totalPods - healthyPods

	// Determine overall health
	if totalPods == 0 {
		status.OverallStatus = domain.HealthStatusUnknown
	} else if healthyPods == totalPods {
		status.OverallStatus = domain.HealthStatusHealthy
	} else if healthyPods == 0 {
		status.OverallStatus = domain.HealthStatusUnhealthy
	} else {
		status.OverallStatus = domain.HealthStatusDegraded
	}

	h.logger.DebugWithContext("namespace health check completed", map[string]interface{}{
		"namespace":    namespace,
		"status":       status.OverallStatus,
		"healthy_pods": healthyPods,
		"total_pods":   totalPods,
	})

	return status, nil
}

// CheckChartHealth checks the health of a specific chart deployment
func (h *HealthChecker) CheckChartHealth(ctx context.Context, chartName, namespace string) (*domain.ChartHealthStatus, error) {
	h.logger.DebugWithContext("checking chart health", map[string]interface{}{
		"chart":     chartName,
		"namespace": namespace,
	})

	chartHealth := &domain.ChartHealthStatus{
		ChartName:   chartName,
		Namespace:   namespace,
		CheckTime:   time.Now(),
		Resources:   make([]domain.ResourceHealthStatus, 0),
	}

	// Check deployments for this chart
	deployments, err := h.kubectlGateway.GetDeploymentsForChart(ctx, chartName, namespace)
	if err != nil {
		chartHealth.OverallStatus = domain.HealthStatusUnknown
		return chartHealth, err
	}

	allHealthy := true
	hasResources := false

	for _, deployment := range deployments {
		hasResources = true
		resourceHealth := domain.ResourceHealthStatus{
			Name:         deployment.Name,
			Kind:         "Deployment",
			Namespace:    deployment.Namespace,
			Status:       deployment.Status,
			Health:       domain.HealthStatusHealthy,
			Ready:        deployment.ReadyReplicas == deployment.Replicas,
			Replicas:     deployment.Replicas,
			ReadyReplicas: deployment.ReadyReplicas,
		}

		// Check deployment readiness
		if deployment.ReadyReplicas != deployment.Replicas {
			resourceHealth.Health = domain.HealthStatusUnhealthy
			allHealthy = false
		}

		chartHealth.Resources = append(chartHealth.Resources, resourceHealth)
	}

	// Check StatefulSets for this chart
	statefulSets, err := h.kubectlGateway.GetStatefulSetsForChart(ctx, chartName, namespace)
	if err != nil {
		chartHealth.OverallStatus = domain.HealthStatusUnknown
		return chartHealth, err
	}

	for _, sts := range statefulSets {
		hasResources = true
		resourceHealth := domain.ResourceHealthStatus{
			Name:         sts.Name,
			Kind:         "StatefulSet",
			Namespace:    sts.Namespace,
			Status:       sts.Status,
			Health:       domain.HealthStatusHealthy,
			Ready:        sts.ReadyReplicas == sts.Replicas,
			Replicas:     sts.Replicas,
			ReadyReplicas: sts.ReadyReplicas,
		}

		// Check StatefulSet readiness
		if sts.ReadyReplicas != sts.Replicas {
			resourceHealth.Health = domain.HealthStatusUnhealthy
			allHealthy = false
		}

		chartHealth.Resources = append(chartHealth.Resources, resourceHealth)
	}

	// Determine overall chart health
	if !hasResources {
		chartHealth.OverallStatus = domain.HealthStatusUnknown
	} else if allHealthy {
		chartHealth.OverallStatus = domain.HealthStatusHealthy
	} else {
		chartHealth.OverallStatus = domain.HealthStatusUnhealthy
	}

	h.logger.DebugWithContext("chart health check completed", map[string]interface{}{
		"chart":           chartName,
		"namespace":       namespace,
		"status":          chartHealth.OverallStatus,
		"resource_count":  len(chartHealth.Resources),
	})

	return chartHealth, nil
}

// WaitForHealthy waits for a target to become healthy
func (h *HealthChecker) WaitForHealthy(ctx context.Context, target *domain.HealthTarget, timeout time.Duration) error {
	h.logger.InfoWithContext("waiting for target to become healthy", map[string]interface{}{
		"target":  target.Name,
		"type":    target.Type,
		"timeout": timeout.String(),
	})

	ticker := time.NewTicker(10 * time.Second) // Check every 10 seconds
	defer ticker.Stop()

	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-timeoutTimer.C:
			return fmt.Errorf("timeout waiting for %s %s to become healthy after %v", target.Type, target.Name, timeout)

		case <-ticker.C:
			var healthy bool
			var err error

			switch target.Type {
			case "namespace":
				status, checkErr := h.CheckNamespaceHealth(ctx, target.Name)
				err = checkErr
				healthy = status != nil && status.OverallStatus == domain.HealthStatusHealthy

			case "chart":
				status, checkErr := h.CheckChartHealth(ctx, target.Name, target.Namespace)
				err = checkErr
				healthy = status != nil && status.OverallStatus == domain.HealthStatusHealthy

			default:
				return fmt.Errorf("unsupported target type: %s", target.Type)
			}

			if err != nil {
				h.logger.WarnWithContext("health check failed during wait", map[string]interface{}{
					"target": target.Name,
					"type":   target.Type,
					"error":  err.Error(),
				})
				continue
			}

			if healthy {
				h.logger.InfoWithContext("target became healthy", map[string]interface{}{
					"target": target.Name,
					"type":   target.Type,
				})
				return nil
			}

			h.logger.DebugWithContext("target not yet healthy, continuing to wait", map[string]interface{}{
				"target": target.Name,
				"type":   target.Type,
			})
		}
	}
}