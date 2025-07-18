package deployment_usecase

import (
	"context"
	"fmt"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// HealthCheckUsecase handles all service and resource health validation
type HealthCheckUsecase struct {
	logger        logger_port.LoggerPort
	healthChecker *HealthChecker
}

// NewHealthCheckUsecase creates a new health check usecase
func NewHealthCheckUsecase(
	logger logger_port.LoggerPort,
	healthChecker *HealthChecker,
) *HealthCheckUsecase {
	return &HealthCheckUsecase{
		logger:        logger,
		healthChecker: healthChecker,
	}
}

// performLayerHealthCheck performs health checks for a specific deployment layer
func (u *HealthCheckUsecase) performLayerHealthCheck(ctx context.Context, layer domain.LayerConfiguration, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("performing layer health check", map[string]interface{}{
		"layer_name":   layer.Name,
		"charts_count": len(layer.Charts),
	})

	switch layer.Name {
	case "storage":
		return u.performStorageLayerHealthCheck(ctx, layer.Charts, options)
	case "core-services":
		return u.performCoreServicesHealthCheck(ctx, layer.Charts, options)
	case "processing-services":
		return u.performProcessingServicesHealthCheck(ctx, layer.Charts, options)
	default:
		return u.performDefaultLayerHealthCheck(ctx, layer.Charts, options)
	}
}

// performChartHealthCheck performs health checks for a specific chart
func (u *HealthCheckUsecase) performChartHealthCheck(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("performing chart health check", map[string]interface{}{
		"chart_name":     chart.Name,
		"multi_namespace": chart.MultiNamespace,
	})

	if chart.MultiNamespace {
		// Handle multi-namespace charts
		for _, namespace := range chart.TargetNamespaces {
			if err := u.performSingleNamespaceHealthCheck(ctx, chart, namespace, options); err != nil {
				return fmt.Errorf("health check failed for chart %s in namespace %s: %w", chart.Name, namespace, err)
			}
		}
		return nil
	}

	// Handle single namespace charts
	namespace := u.getNamespaceForChart(chart)
	return u.performSingleNamespaceHealthCheck(ctx, chart, namespace, options)
}

// performSingleNamespaceHealthCheck performs health check for a chart in a single namespace
func (u *HealthCheckUsecase) performSingleNamespaceHealthCheck(ctx context.Context, chart domain.Chart, namespace string, options *domain.DeploymentOptions) error {
	u.logger.DebugWithContext("performing single namespace health check", map[string]interface{}{
		"chart_name": chart.Name,
		"namespace":  namespace,
	})

	// Check if this is a StatefulSet chart
	if u.isStatefulSetChart(chart.Name) {
		return u.performStatefulSetHealthCheck(ctx, chart.Name, namespace)
	}

	// Default to deployment health check
	return u.performDeploymentHealthCheck(ctx, chart.Name, namespace)
}

// performStorageLayerHealthCheck performs health checks for storage layer services
func (u *HealthCheckUsecase) performStorageLayerHealthCheck(ctx context.Context, charts []domain.Chart, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("performing storage layer health check", map[string]interface{}{
		"charts_count": len(charts),
	})

	for _, chart := range charts {
		if err := u.performChartHealthCheck(ctx, chart, options); err != nil {
			return fmt.Errorf("storage layer health check failed for chart %s: %w", chart.Name, err)
		}

		// Additional storage-specific checks
		if chart.Name == "postgres" || chart.Name == "auth-postgres" || chart.Name == "kratos-postgres" {
			if err := u.verifyDatabaseConnectivity(ctx, chart.Name, u.getNamespaceForChart(chart)); err != nil {
				u.logger.WarnWithContext("database connectivity check failed", map[string]interface{}{
					"chart_name": chart.Name,
					"error":      err.Error(),
				})
				// Don't fail deployment for connectivity issues, just warn
			}
		}
	}

	return nil
}

// performCoreServicesHealthCheck performs health checks for core services
func (u *HealthCheckUsecase) performCoreServicesHealthCheck(ctx context.Context, charts []domain.Chart, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("performing core services health check", map[string]interface{}{
		"charts_count": len(charts),
	})

	for _, chart := range charts {
		if err := u.performChartHealthCheck(ctx, chart, options); err != nil {
			return fmt.Errorf("core services health check failed for chart %s: %w", chart.Name, err)
		}

		// Additional core service-specific checks
		if chart.Name == "auth-service" || chart.Name == "alt-backend" {
			if err := u.verifyServiceHealthEndpoint(ctx, chart.Name, u.getNamespaceForChart(chart)); err != nil {
				u.logger.WarnWithContext("service health endpoint check failed", map[string]interface{}{
					"chart_name": chart.Name,
					"error":      err.Error(),
				})
				// Don't fail deployment for endpoint issues, just warn
			}
		}
	}

	return nil
}

// performProcessingServicesHealthCheck performs health checks for processing services
func (u *HealthCheckUsecase) performProcessingServicesHealthCheck(ctx context.Context, charts []domain.Chart, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("performing processing services health check", map[string]interface{}{
		"charts_count": len(charts),
	})

	for _, chart := range charts {
		if err := u.performChartHealthCheck(ctx, chart, options); err != nil {
			return fmt.Errorf("processing services health check failed for chart %s: %w", chart.Name, err)
		}
	}

	return nil
}

// performDefaultLayerHealthCheck performs default health checks for unspecified layers
func (u *HealthCheckUsecase) performDefaultLayerHealthCheck(ctx context.Context, charts []domain.Chart, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("performing default layer health check", map[string]interface{}{
		"charts_count": len(charts),
	})

	for _, chart := range charts {
		if err := u.performChartHealthCheck(ctx, chart, options); err != nil {
			return fmt.Errorf("default layer health check failed for chart %s: %w", chart.Name, err)
		}
	}

	return nil
}

// performStatefulSetHealthCheck performs health checks for StatefulSet-based services
func (u *HealthCheckUsecase) performStatefulSetHealthCheck(ctx context.Context, chartName, namespace string) error {
	u.logger.InfoWithContext("performing StatefulSet health check", map[string]interface{}{
		"chart_name": chartName,
		"namespace":  namespace,
	})

	// Use the existing health checker for StatefulSet validation
	if err := u.healthChecker.WaitForStatefulSetReady(ctx, namespace, chartName); err != nil {
		return fmt.Errorf("StatefulSet health check failed for %s in %s: %w", chartName, namespace, err)
	}

	// Additional StatefulSet-specific checks
	if chartName == "postgres" || chartName == "auth-postgres" || chartName == "kratos-postgres" {
		if err := u.verifyDatabaseConnectivity(ctx, chartName, namespace); err != nil {
			u.logger.WarnWithContext("database connectivity check failed", map[string]interface{}{
				"chart_name": chartName,
				"namespace":  namespace,
				"error":      err.Error(),
			})
			// Don't fail for connectivity issues, just warn
		}
	}

	return nil
}

// performDeploymentHealthCheck performs health checks for Deployment-based services
func (u *HealthCheckUsecase) performDeploymentHealthCheck(ctx context.Context, chartName, namespace string) error {
	u.logger.InfoWithContext("performing Deployment health check", map[string]interface{}{
		"chart_name": chartName,
		"namespace":  namespace,
	})

	// Use the existing health checker for Deployment validation
	if err := u.healthChecker.WaitForServiceReady(ctx, chartName, "deployment", namespace); err != nil {
		return fmt.Errorf("Deployment health check failed for %s in %s: %w", chartName, namespace, err)
	}

	// Additional Deployment-specific checks
	if chartName == "auth-service" || chartName == "alt-backend" || chartName == "alt-frontend" {
		if err := u.verifyServiceHealthEndpoint(ctx, chartName, namespace); err != nil {
			u.logger.WarnWithContext("service health endpoint check failed", map[string]interface{}{
				"chart_name": chartName,
				"namespace":  namespace,
				"error":      err.Error(),
			})
			// Don't fail for endpoint issues, just warn
		}
	}

	return nil
}

// verifyChartDeployment verifies that a chart has been deployed successfully
func (u *HealthCheckUsecase) verifyChartDeployment(ctx context.Context, chart domain.Chart, namespace string) error {
	u.logger.InfoWithContext("verifying chart deployment", map[string]interface{}{
		"chart_name": chart.Name,
		"namespace":  namespace,
	})

	// Check if this is a secret-only chart
	if u.isSecretOnlyChart(chart.Name) {
		return u.verifySecretChart(ctx, chart.Name, namespace)
	}

	// For regular charts, verify the deployment
	if u.isStatefulSetChart(chart.Name) {
		return u.performStatefulSetHealthCheck(ctx, chart.Name, namespace)
	}

	return u.performDeploymentHealthCheck(ctx, chart.Name, namespace)
}

// verifySecretChart verifies that a secret chart has been deployed successfully
func (u *HealthCheckUsecase) verifySecretChart(ctx context.Context, chartName, namespace string) error {
	u.logger.InfoWithContext("verifying secret chart", map[string]interface{}{
		"chart_name": chartName,
		"namespace":  namespace,
	})

	// Use the existing health checker for secret validation
	if err := u.healthChecker.WaitForServiceReady(ctx, chartName, "secret", namespace); err != nil {
		return fmt.Errorf("secret chart verification failed for %s in %s: %w", chartName, namespace, err)
	}

	return nil
}

// verifyDatabaseConnectivity verifies database connectivity for database services
func (u *HealthCheckUsecase) verifyDatabaseConnectivity(ctx context.Context, chartName, namespace string) error {
	u.logger.InfoWithContext("verifying database connectivity", map[string]interface{}{
		"chart_name": chartName,
		"namespace":  namespace,
	})

	// Use the existing health checker for database connectivity
	if chartName == "postgres" || chartName == "auth-postgres" || chartName == "kratos-postgres" {
		if err := u.healthChecker.WaitForPostgreSQLReady(ctx, namespace, chartName); err != nil {
			return fmt.Errorf("database connectivity verification failed for %s in %s: %w", chartName, namespace, err)
		}
	} else if chartName == "clickhouse" {
		if err := u.healthChecker.WaitForClickHouseReady(ctx, namespace, chartName); err != nil {
			return fmt.Errorf("database connectivity verification failed for %s in %s: %w", chartName, namespace, err)
		}
	} else if chartName == "meilisearch" {
		if err := u.healthChecker.WaitForMeilisearchReady(ctx, namespace, chartName); err != nil {
			return fmt.Errorf("database connectivity verification failed for %s in %s: %w", chartName, namespace, err)
		}
	}

	return nil
}

// verifyServiceHealthEndpoint verifies service health endpoints
func (u *HealthCheckUsecase) verifyServiceHealthEndpoint(ctx context.Context, chartName, namespace string) error {
	u.logger.InfoWithContext("verifying service health endpoint", map[string]interface{}{
		"chart_name": chartName,
		"namespace":  namespace,
	})

	// Use the existing health checker for service health endpoint validation
	if err := u.healthChecker.WaitForServiceReady(ctx, chartName, "service", namespace); err != nil {
		return fmt.Errorf("service health endpoint verification failed for %s in %s: %w", chartName, namespace, err)
	}

	return nil
}

// validatePodUpdates validates that pods have been updated successfully
func (u *HealthCheckUsecase) validatePodUpdates(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("validating pod updates", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// Get all namespaces to validate
	namespaces := u.getNamespacesToValidate(options)

	for _, namespace := range namespaces {
		u.logger.DebugWithContext("validating pods in namespace", map[string]interface{}{
			"namespace": namespace,
		})

		// Validate deployment pods
		if err := u.validateDeploymentPods(ctx, namespace); err != nil {
			u.logger.WarnWithContext("deployment pod validation failed", map[string]interface{}{
				"namespace": namespace,
				"error":     err.Error(),
			})
			// Don't fail deployment for pod validation issues, just warn
		}

		// Validate StatefulSet pods
		if err := u.validateStatefulSetPods(ctx, namespace); err != nil {
			u.logger.WarnWithContext("StatefulSet pod validation failed", map[string]interface{}{
				"namespace": namespace,
				"error":     err.Error(),
			})
			// Don't fail deployment for pod validation issues, just warn
		}
	}

	return nil
}

// validateDeploymentPods validates deployment pods in a namespace
func (u *HealthCheckUsecase) validateDeploymentPods(ctx context.Context, namespace string) error {
	u.logger.DebugWithContext("validating deployment pods", map[string]interface{}{
		"namespace": namespace,
	})

	// Use the existing health checker for deployment pod validation
	if err := u.healthChecker.WaitForPodsReady(ctx, namespace, "deployment"); err != nil {
		return fmt.Errorf("deployment pod validation failed in namespace %s: %w", namespace, err)
	}

	return nil
}

// validateStatefulSetPods validates StatefulSet pods in a namespace
func (u *HealthCheckUsecase) validateStatefulSetPods(ctx context.Context, namespace string) error {
	u.logger.DebugWithContext("validating StatefulSet pods", map[string]interface{}{
		"namespace": namespace,
	})

	// Use the existing health checker for StatefulSet pod validation
	if err := u.healthChecker.WaitForPodsReady(ctx, namespace, "statefulset"); err != nil {
		return fmt.Errorf("StatefulSet pod validation failed in namespace %s: %w", namespace, err)
	}

	return nil
}

// Helper methods

// isStatefulSetChart checks if a chart is a StatefulSet-based chart
func (u *HealthCheckUsecase) isStatefulSetChart(chartName string) bool {
	statefulSetCharts := []string{
		"postgres", "auth-postgres", "kratos-postgres", "clickhouse", "meilisearch",
	}

	for _, chart := range statefulSetCharts {
		if chart == chartName {
			return true
		}
	}
	return false
}

// isSecretOnlyChart checks if a chart is a secret-only chart
func (u *HealthCheckUsecase) isSecretOnlyChart(chartName string) bool {
	secretCharts := []string{
		"common-secrets", "common-config", "common-ssl",
	}

	for _, chart := range secretCharts {
		if chart == chartName {
			return true
		}
	}
	return false
}

// getNamespaceForChart returns the appropriate namespace for a chart
func (u *HealthCheckUsecase) getNamespaceForChart(chart domain.Chart) string {
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

// getNamespacesToValidate returns the list of namespaces to validate based on deployment options
func (u *HealthCheckUsecase) getNamespacesToValidate(options *domain.DeploymentOptions) []string {
	// This is a simplified implementation - in practice, this would be determined
	// by the actual charts being deployed
	return []string{"alt-apps", "alt-database", "alt-auth", "alt-ingress", "alt-search"}
}