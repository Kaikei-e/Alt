package deployment_usecase

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"deploy-cli/port/logger_port"
)

// HealthChecker provides functionality to validate service readiness
type HealthChecker struct {
	logger logger_port.LoggerPort
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(logger logger_port.LoggerPort) *HealthChecker {
	return &HealthChecker{
		logger: logger,
	}
}

// WaitForPostgreSQLReady waits for PostgreSQL service to be ready for connections
func (h *HealthChecker) WaitForPostgreSQLReady(ctx context.Context, namespace, serviceName string) error {
	h.logger.InfoWithContext("🐘 PostgreSQL health check STARTING", map[string]interface{}{
		"namespace":    namespace,
		"service":      serviceName,
		"max_duration": "5m",
		"context_deadline": func() string {
			if deadline, ok := ctx.Deadline(); ok {
				return deadline.Format(time.RFC3339)
			}
			return "no deadline"
		}(),
	})

	// Add emergency timeout detection
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	attempt := 0
	maxRetries := 60 // 5 minutes with 5 second intervals (more frequent checks)
	
	for attempt < maxRetries {
		select {
		case <-ctx.Done():
			h.logger.ErrorWithContext("⏰ PostgreSQL health check TIMEOUT", map[string]interface{}{
				"namespace": namespace,
				"service":   serviceName,
				"attempt":   attempt + 1,
				"error":     ctx.Err().Error(),
				"total_duration": fmt.Sprintf("%ds", attempt*5),
			})
			return ctx.Err()
		case <-ticker.C:
			attempt++
			
			// Log current attempt with detailed info
			h.logger.InfoWithContext("🔍 Checking PostgreSQL connection", map[string]interface{}{
				"namespace":   namespace,
				"service":     serviceName,
				"attempt":     attempt,
				"max_retries": maxRetries,
				"elapsed_time": fmt.Sprintf("%ds", attempt*5),
				"remaining_retries": maxRetries - attempt,
			})

			// Check if PostgreSQL is ready to accept connections
			if err := h.checkPostgreSQLConnection(namespace, serviceName); err == nil {
				h.logger.InfoWithContext("🐘 PostgreSQL service is READY", map[string]interface{}{
					"namespace": namespace,
					"service":   serviceName,
					"attempts":  attempt,
					"total_duration": fmt.Sprintf("%ds", attempt*5),
				})
				return nil
			} else {
				h.logger.WarnWithContext("🐘 PostgreSQL not ready, retrying", map[string]interface{}{
					"namespace":   namespace,
					"service":     serviceName,
					"attempt":     attempt,
					"max_retries": maxRetries,
					"error":       err.Error(),
					"retry_delay": "5s",
					"time_remaining": fmt.Sprintf("%ds", (maxRetries-attempt)*5),
				})
			}
		}
	}

	h.logger.ErrorWithContext("🐘 PostgreSQL service FAILED after maximum attempts", map[string]interface{}{
		"namespace":      namespace,
		"service":        serviceName,
		"max_attempts":   maxRetries,
		"total_duration": fmt.Sprintf("%ds", maxRetries*5),
	})
	return fmt.Errorf("PostgreSQL service %s in namespace %s not ready after %d attempts (%ds total)", serviceName, namespace, maxRetries, maxRetries*5)
}

// checkPostgreSQLConnection checks if PostgreSQL is ready to accept connections
func (h *HealthChecker) checkPostgreSQLConnection(namespace, serviceName string) error {
	// Create a timeout context to prevent kubectl exec from hanging
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try to connect to PostgreSQL using pg_isready
	podName := serviceName + "-0" // StatefulSet naming convention
	h.logger.DebugWithContext("executing PostgreSQL connection check", map[string]interface{}{
		"namespace": namespace,
		"service":   serviceName,
		"pod":       podName,
		"command":   "pg_isready",
		"timeout":   "30s",
	})

	cmd := exec.CommandContext(ctx, "kubectl", "exec", "-n", namespace, podName, "--", "pg_isready", "-U", "alt_db_user")

	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			h.logger.WarnWithContext("PostgreSQL connection check timed out", map[string]interface{}{
				"namespace": namespace,
				"service":   serviceName,
				"pod":       podName,
				"timeout":   "30s",
				"output":    string(output),
			})
			return fmt.Errorf("PostgreSQL connection check timed out after 30s")
		}
		h.logger.DebugWithContext("PostgreSQL connection check failed", map[string]interface{}{
			"namespace": namespace,
			"service":   serviceName,
			"pod":       podName,
			"error":     err.Error(),
			"output":    string(output),
		})
		return fmt.Errorf("PostgreSQL connection check failed: %w", err)
	}

	// Check if output contains "accepting connections"
	if !strings.Contains(string(output), "accepting connections") {
		h.logger.DebugWithContext("PostgreSQL not accepting connections", map[string]interface{}{
			"namespace": namespace,
			"service":   serviceName,
			"pod":       podName,
			"output":    string(output),
		})
		return fmt.Errorf("PostgreSQL not accepting connections: %s", string(output))
	}

	h.logger.DebugWithContext("PostgreSQL connection check successful", map[string]interface{}{
		"namespace": namespace,
		"service":   serviceName,
		"pod":       podName,
		"output":    string(output),
	})
	return nil
}

// WaitForMeilisearchReady waits for Meilisearch service to be ready
func (h *HealthChecker) WaitForMeilisearchReady(ctx context.Context, namespace, serviceName string) error {
	h.logger.Info("waiting for Meilisearch service to be ready",
		"namespace", namespace,
		"service", serviceName,
	)

	maxRetries := 30 // 5 minutes with 10 second intervals
	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if Meilisearch is ready
		if err := h.checkMeilisearchHealth(namespace, serviceName); err == nil {
			h.logger.Info("Meilisearch service is ready",
				"namespace", namespace,
				"service", serviceName,
				"attempts", i+1,
			)
			return nil
		}

		h.logger.Debug("Meilisearch not ready, retrying",
			"namespace", namespace,
			"service", serviceName,
			"attempt", i+1,
			"max_retries", maxRetries,
		)

		time.Sleep(10 * time.Second)
	}

	return fmt.Errorf("Meilisearch service %s in namespace %s not ready after %d attempts", serviceName, namespace, maxRetries)
}

// checkMeilisearchHealth checks if Meilisearch is healthy
func (h *HealthChecker) checkMeilisearchHealth(namespace, serviceName string) error {
	// Try to access Meilisearch health endpoint
	podName := serviceName + "-0" // StatefulSet naming convention
	cmd := exec.Command("kubectl", "exec", "-n", namespace, podName, "--", "curl", "-f", "http://localhost:7700/health")

	output, err := cmd.CombinedOutput()
	if err != nil {
		h.logger.Debug("Meilisearch health check failed",
			"namespace", namespace,
			"service", serviceName,
			"error", err,
			"output", string(output),
		)
		return fmt.Errorf("Meilisearch health check failed: %w", err)
	}

	// Check if output contains "available"
	if !strings.Contains(string(output), "available") {
		return fmt.Errorf("Meilisearch not available: %s", string(output))
	}

	return nil
}

// WaitForServiceReady waits for any service to be ready based on its type
func (h *HealthChecker) WaitForServiceReady(ctx context.Context, serviceName, serviceType, namespace string) error {
	h.logger.InfoWithContext("waiting for service readiness", map[string]interface{}{
		"service":   serviceName,
		"type":      serviceType,
		"namespace": namespace,
	})

	// Check if this is a StatefulSet service
	if h.isStatefulSetService(serviceName) {
		if err := h.WaitForStatefulSetReady(ctx, namespace, serviceName); err != nil {
			return fmt.Errorf("statefulset readiness check failed: %w", err)
		}
	}

	switch serviceType {
	case "postgres", "postgresql":
		return h.WaitForPostgreSQLReady(ctx, namespace, serviceName)
	case "meilisearch":
		return h.WaitForMeilisearchReady(ctx, namespace, serviceName)
	case "clickhouse":
		return h.WaitForClickHouseReady(ctx, namespace, serviceName)
	default:
		// For other services, just check if pods are ready
		h.logger.InfoWithContext("using generic pod readiness check", map[string]interface{}{
			"service":   serviceName,
			"type":      serviceType,
			"namespace": namespace,
		})
		return h.WaitForPodsReady(ctx, namespace, serviceName)
	}
}

// WaitForPodsReady waits for pods to be in ready state
func (h *HealthChecker) WaitForPodsReady(ctx context.Context, namespace, serviceName string) error {
	h.logger.Info("waiting for pods to be ready",
		"namespace", namespace,
		"service", serviceName,
	)

	maxRetries := 30 // 5 minutes with 10 second intervals
	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if pods are ready
		if err := h.checkPodsReady(namespace, serviceName); err == nil {
			h.logger.Info("pods are ready",
				"namespace", namespace,
				"service", serviceName,
				"attempts", i+1,
			)
			return nil
		}

		h.logger.Debug("pods not ready, retrying",
			"namespace", namespace,
			"service", serviceName,
			"attempt", i+1,
			"max_retries", maxRetries,
		)

		time.Sleep(10 * time.Second)
	}

	return fmt.Errorf("pods for service %s in namespace %s not ready after %d attempts", serviceName, namespace, maxRetries)
}

// checkPodsReady checks if pods are in ready state
func (h *HealthChecker) checkPodsReady(namespace, serviceName string) error {
	cmd := exec.Command("kubectl", "get", "pods", "-n", namespace, "-l", fmt.Sprintf("app.kubernetes.io/name=%s", serviceName), "-o", "jsonpath={.items[*].status.conditions[?(@.type=='Ready')].status}")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to check pod readiness: %w", err)
	}

	// Check if all pods are ready
	statuses := strings.Fields(string(output))
	for _, status := range statuses {
		if status != "True" {
			return fmt.Errorf("not all pods are ready")
		}
	}

	if len(statuses) == 0 {
		return fmt.Errorf("no pods found")
	}

	return nil
}

// WaitForStatefulSetReady waits for StatefulSet to be fully ready
func (h *HealthChecker) WaitForStatefulSetReady(ctx context.Context, namespace, serviceName string) error {
	h.logger.InfoWithContext("waiting for StatefulSet to be ready", map[string]interface{}{
		"namespace":    namespace,
		"service":      serviceName,
		"max_duration": "15m",
	})

	maxRetries := 90 // 15 minutes with 10 second intervals
	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			h.logger.ErrorWithContext("StatefulSet wait cancelled", map[string]interface{}{
				"namespace": namespace,
				"service":   serviceName,
				"attempt":   i + 1,
				"error":     ctx.Err().Error(),
			})
			return ctx.Err()
		default:
		}

		// Log current attempt
		h.logger.InfoWithContext("checking StatefulSet readiness", map[string]interface{}{
			"namespace":   namespace,
			"service":     serviceName,
			"attempt":     i + 1,
			"max_retries": maxRetries,
		})

		// First check if StatefulSet exists
		exists, err := h.checkStatefulSetExists(namespace, serviceName)
		if err != nil {
			h.logger.WarnWithContext("Failed to check StatefulSet existence, retrying", map[string]interface{}{
				"namespace":   namespace,
				"service":     serviceName,
				"attempt":     i + 1,
				"max_retries": maxRetries,
				"error":       err.Error(),
				"retry_delay": "10s",
			})
		} else if !exists {
			h.logger.InfoWithContext("StatefulSet does not exist yet, waiting for creation", map[string]interface{}{
				"namespace":   namespace,
				"service":     serviceName,
				"attempt":     i + 1,
				"max_retries": maxRetries,
				"retry_delay": "10s",
			})
		} else {
			// StatefulSet exists, check if it's ready
			if err := h.checkStatefulSetReady(namespace, serviceName); err == nil {
				h.logger.InfoWithContext("StatefulSet is ready", map[string]interface{}{
					"namespace": namespace,
					"service":   serviceName,
					"attempts":  i + 1,
				})
				return nil
			} else {
				h.logger.WarnWithContext("StatefulSet not ready, retrying", map[string]interface{}{
					"namespace":   namespace,
					"service":     serviceName,
					"attempt":     i + 1,
					"max_retries": maxRetries,
					"error":       err.Error(),
					"retry_delay": "10s",
				})
			}
		}

		time.Sleep(10 * time.Second)
	}

	h.logger.ErrorWithContext("StatefulSet not ready after maximum attempts", map[string]interface{}{
		"namespace":      namespace,
		"service":        serviceName,
		"max_attempts":   maxRetries,
		"total_duration": "15m",
	})
	return fmt.Errorf("StatefulSet %s in namespace %s not ready after %d attempts", serviceName, namespace, maxRetries)
}

// checkStatefulSetExists checks if a StatefulSet exists in the namespace
func (h *HealthChecker) checkStatefulSetExists(namespace, serviceName string) (bool, error) {
	// Use kubectl to check StatefulSet existence
	cmd := exec.Command("kubectl", "get", "statefulset", serviceName, "-n", namespace)
	_, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check StatefulSet existence: %w", err)
	}
	return true, nil
}

// checkStatefulSetReady checks if StatefulSet is fully ready (assumes StatefulSet exists)
func (h *HealthChecker) checkStatefulSetReady(namespace, serviceName string) error {
	h.logger.DebugWithContext("checking StatefulSet status", map[string]interface{}{
		"namespace": namespace,
		"service":   serviceName,
	})

	// Check StatefulSet status (StatefulSet existence should be checked by caller)
	cmd := exec.Command("kubectl", "get", "statefulset", serviceName, "-n", namespace, "-o", "jsonpath={.status.replicas},{.status.readyReplicas},{.status.currentReplicas}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		h.logger.DebugWithContext("StatefulSet status check failed", map[string]interface{}{
			"namespace": namespace,
			"service":   serviceName,
			"error":     err.Error(),
			"output":    string(output),
		})
		return fmt.Errorf("StatefulSet status check failed: %w", err)
	}

	statusParts := strings.Split(strings.TrimSpace(string(output)), ",")
	if len(statusParts) != 3 {
		return fmt.Errorf("invalid StatefulSet status format: %s", string(output))
	}

	replicas := statusParts[0]
	readyReplicas := statusParts[1]
	currentReplicas := statusParts[2]

	// Check if all replicas are ready
	if replicas != readyReplicas || replicas != currentReplicas {
		return fmt.Errorf("StatefulSet not ready: %s/%s ready, %s current", readyReplicas, replicas, currentReplicas)
	}

	if replicas == "" || replicas == "0" {
		return fmt.Errorf("StatefulSet has no replicas")
	}

	// Additionally check if all pods are running
	if err := h.checkStatefulSetPodsRunning(namespace, serviceName); err != nil {
		return fmt.Errorf("StatefulSet pods not running: %w", err)
	}

	h.logger.DebugWithContext("StatefulSet is ready", map[string]interface{}{
		"namespace":        namespace,
		"service":          serviceName,
		"replicas":         replicas,
		"ready_replicas":   readyReplicas,
		"current_replicas": currentReplicas,
	})

	return nil
}

// checkStatefulSetPodsRunning checks if all StatefulSet pods are running
func (h *HealthChecker) checkStatefulSetPodsRunning(namespace, serviceName string) error {
	cmd := exec.Command("kubectl", "get", "pods", "-n", namespace, "-l", fmt.Sprintf("app.kubernetes.io/name=%s", serviceName), "-o", "jsonpath={.items[*].status.phase}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to get pod status: %w", err)
	}

	phases := strings.Fields(string(output))
	if len(phases) == 0 {
		return fmt.Errorf("no pods found for StatefulSet")
	}

	for _, phase := range phases {
		if phase != "Running" {
			return fmt.Errorf("pod not running: phase=%s", phase)
		}
	}

	return nil
}

// WaitForClickHouseReady waits for ClickHouse service to be ready
func (h *HealthChecker) WaitForClickHouseReady(ctx context.Context, namespace, serviceName string) error {
	h.logger.InfoWithContext("waiting for ClickHouse service to be ready", map[string]interface{}{
		"namespace":    namespace,
		"service":      serviceName,
		"max_duration": "10m",
	})

	maxRetries := 60 // 10 minutes with 10 second intervals
	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			h.logger.ErrorWithContext("ClickHouse wait cancelled", map[string]interface{}{
				"namespace": namespace,
				"service":   serviceName,
				"attempt":   i + 1,
				"error":     ctx.Err().Error(),
			})
			return ctx.Err()
		default:
		}

		// Log current attempt
		h.logger.InfoWithContext("checking ClickHouse connection", map[string]interface{}{
			"namespace":   namespace,
			"service":     serviceName,
			"attempt":     i + 1,
			"max_retries": maxRetries,
		})

		// Check if ClickHouse is ready
		if err := h.checkClickHouseHealth(namespace, serviceName); err == nil {
			h.logger.InfoWithContext("ClickHouse service is ready", map[string]interface{}{
				"namespace": namespace,
				"service":   serviceName,
				"attempts":  i + 1,
			})
			return nil
		} else {
			h.logger.WarnWithContext("ClickHouse not ready, retrying", map[string]interface{}{
				"namespace":   namespace,
				"service":     serviceName,
				"attempt":     i + 1,
				"max_retries": maxRetries,
				"error":       err.Error(),
				"retry_delay": "10s",
			})
		}

		time.Sleep(10 * time.Second)
	}

	h.logger.ErrorWithContext("ClickHouse service not ready after maximum attempts", map[string]interface{}{
		"namespace":      namespace,
		"service":        serviceName,
		"max_attempts":   maxRetries,
		"total_duration": "10m",
	})
	return fmt.Errorf("ClickHouse service %s in namespace %s not ready after %d attempts", serviceName, namespace, maxRetries)
}

// checkClickHouseHealth checks if ClickHouse is healthy
func (h *HealthChecker) checkClickHouseHealth(namespace, serviceName string) error {
	// Create a timeout context to prevent kubectl exec from hanging
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	podName := serviceName + "-0" // StatefulSet naming convention

	// Check if SSL is enabled for ClickHouse
	sslEnabled, err := h.isClickHouseSSLEnabled(namespace, podName)
	if err != nil {
		h.logger.WarnWithContext("failed to detect ClickHouse SSL configuration, trying HTTP", map[string]interface{}{
			"namespace": namespace,
			"service":   serviceName,
			"pod":       podName,
			"error":     err.Error(),
		})
		sslEnabled = false
	}

	var endpoint string
	var scheme string
	if sslEnabled {
		endpoint = "https://localhost:8443/ping"
		scheme = "HTTPS"
	} else {
		endpoint = "http://localhost:8123/ping"
		scheme = "HTTP"
	}

	h.logger.DebugWithContext("executing ClickHouse health check", map[string]interface{}{
		"namespace":   namespace,
		"service":     serviceName,
		"pod":         podName,
		"endpoint":    endpoint,
		"scheme":      scheme,
		"timeout":     "30s",
		"ssl_enabled": sslEnabled,
	})

	var cmd *exec.Cmd
	if sslEnabled {
		// Use -k flag to ignore SSL certificate verification for health checks
		cmd = exec.CommandContext(ctx, "kubectl", "exec", "-n", namespace, podName, "--", "curl", "-f", "-k", endpoint)
	} else {
		cmd = exec.CommandContext(ctx, "kubectl", "exec", "-n", namespace, podName, "--", "curl", "-f", endpoint)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			h.logger.WarnWithContext("ClickHouse health check timed out", map[string]interface{}{
				"namespace": namespace,
				"service":   serviceName,
				"pod":       podName,
				"timeout":   "30s",
				"output":    string(output),
			})
			return fmt.Errorf("ClickHouse health check timed out after 30s")
		}
		h.logger.DebugWithContext("ClickHouse health check failed", map[string]interface{}{
			"namespace": namespace,
			"service":   serviceName,
			"pod":       podName,
			"error":     err.Error(),
			"output":    string(output),
		})
		return fmt.Errorf("ClickHouse health check failed: %w", err)
	}

	// Check if output contains "Ok"
	if !strings.Contains(string(output), "Ok") {
		h.logger.DebugWithContext("ClickHouse not responding correctly", map[string]interface{}{
			"namespace": namespace,
			"service":   serviceName,
			"pod":       podName,
			"output":    string(output),
		})
		return fmt.Errorf("ClickHouse not responding correctly: %s", string(output))
	}

	h.logger.DebugWithContext("ClickHouse health check successful", map[string]interface{}{
		"namespace": namespace,
		"service":   serviceName,
		"pod":       podName,
		"output":    string(output),
	})
	return nil
}

// isStatefulSetService determines if a service is deployed as a StatefulSet
func (h *HealthChecker) isStatefulSetService(serviceName string) bool {
	statefulSetServices := []string{
		"postgres", "auth-postgres", "kratos-postgres", "clickhouse", "meilisearch",
	}

	for _, stsService := range statefulSetServices {
		if serviceName == stsService {
			return true
		}
	}
	return false
}

// WaitForSecretsReady waits for secrets to be created and available for secret-only charts
func (h *HealthChecker) WaitForSecretsReady(ctx context.Context, chartName, namespace string) error {
	h.logger.InfoWithContext("waiting for secrets to be ready", map[string]interface{}{
		"chart":     chartName,
		"namespace": namespace,
	})

	maxRetries := 12 // 2 minutes with 10 second intervals (shorter for secret validation)
	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			h.logger.ErrorWithContext("secrets readiness check cancelled", map[string]interface{}{
				"chart":     chartName,
				"namespace": namespace,
				"attempt":   i + 1,
				"error":     ctx.Err().Error(),
			})
			return ctx.Err()
		default:
		}

		// Log current attempt
		h.logger.InfoWithContext("checking secrets existence", map[string]interface{}{
			"chart":       chartName,
			"namespace":   namespace,
			"attempt":     i + 1,
			"max_retries": maxRetries,
		})

		// Check if secrets exist and are valid
		if err := h.checkSecretsReady(namespace, chartName); err == nil {
			h.logger.InfoWithContext("secrets are ready", map[string]interface{}{
				"chart":     chartName,
				"namespace": namespace,
				"attempts":  i + 1,
			})
			return nil
		} else {
			h.logger.WarnWithContext("secrets not ready, retrying", map[string]interface{}{
				"chart":       chartName,
				"namespace":   namespace,
				"attempt":     i + 1,
				"max_retries": maxRetries,
				"error":       err.Error(),
				"retry_delay": "10s",
			})
		}

		time.Sleep(10 * time.Second)
	}

	h.logger.ErrorWithContext("secrets not ready after maximum attempts", map[string]interface{}{
		"chart":          chartName,
		"namespace":      namespace,
		"max_attempts":   maxRetries,
		"total_duration": "2m",
	})
	return fmt.Errorf("secrets for chart %s in namespace %s not ready after %d attempts", chartName, namespace, maxRetries)
}

// checkSecretsReady checks if secrets managed by the chart exist and are valid
func (h *HealthChecker) checkSecretsReady(namespace, chartName string) error {
	h.logger.DebugWithContext("checking secrets for chart", map[string]interface{}{
		"chart":     chartName,
		"namespace": namespace,
	})

	// Get all secrets in the namespace managed by this chart
	cmd := exec.Command("kubectl", "get", "secrets", "-n", namespace, "-l", fmt.Sprintf("app.kubernetes.io/name=%s", chartName), "-o", "name")
	output, err := cmd.CombinedOutput()
	if err != nil {
		h.logger.DebugWithContext("failed to get secrets", map[string]interface{}{
			"chart":     chartName,
			"namespace": namespace,
			"error":     err.Error(),
			"output":    string(output),
		})
		return fmt.Errorf("failed to get secrets: %w", err)
	}

	secretNames := strings.Fields(strings.TrimSpace(string(output)))
	if len(secretNames) == 0 {
		// If no secrets found with chart label, check for common secret patterns
		return h.checkCommonSecrets(namespace, chartName)
	}

	// Verify that all found secrets are ready (have data)
	for _, secretName := range secretNames {
		// Remove "secret/" prefix if present
		secretName = strings.TrimPrefix(secretName, "secret/")
		
		if err := h.checkSecretData(namespace, secretName); err != nil {
			h.logger.DebugWithContext("secret not ready", map[string]interface{}{
				"chart":      chartName,
				"namespace":  namespace,
				"secret":     secretName,
				"error":      err.Error(),
			})
			return fmt.Errorf("secret %s not ready: %w", secretName, err)
		}
	}

	h.logger.DebugWithContext("all secrets are ready", map[string]interface{}{
		"chart":        chartName,
		"namespace":    namespace,
		"secret_count": len(secretNames),
	})
	return nil
}

// checkCommonSecrets checks for common secret patterns when no labeled secrets found
func (h *HealthChecker) checkCommonSecrets(namespace, chartName string) error {
	commonSecretPatterns := []string{
		chartName + "-secrets",
		chartName,
		"database-secrets",
		"api-secrets",
		"service-secrets",
	}

	for _, secretPattern := range commonSecretPatterns {
		cmd := exec.Command("kubectl", "get", "secret", secretPattern, "-n", namespace)
		if err := cmd.Run(); err == nil {
			// Found a secret with this pattern, verify it has data
			if err := h.checkSecretData(namespace, secretPattern); err == nil {
				h.logger.DebugWithContext("found valid secret with pattern", map[string]interface{}{
					"chart":     chartName,
					"namespace": namespace,
					"secret":    secretPattern,
				})
				return nil
			}
		}
	}

	return fmt.Errorf("no valid secrets found for chart %s", chartName)
}

// checkSecretData verifies that a secret exists and contains data
func (h *HealthChecker) checkSecretData(namespace, secretName string) error {
	cmd := exec.Command("kubectl", "get", "secret", secretName, "-n", namespace, "-o", "jsonpath={.data}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("secret %s not found or inaccessible: %w", secretName, err)
	}

	// Check if secret has any data
	data := strings.TrimSpace(string(output))
	if data == "" || data == "{}" {
		return fmt.Errorf("secret %s exists but contains no data", secretName)
	}

	return nil
}

// isClickHouseSSLEnabled checks if SSL is enabled for ClickHouse by examining the configuration
func (h *HealthChecker) isClickHouseSSLEnabled(namespace, podName string) (bool, error) {
	// Check if SSL-related environment variables or configuration exist
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First, try to check if HTTPS port is listening
	cmd := exec.CommandContext(ctx, "kubectl", "exec", "-n", namespace, podName, "--", "netstat", "-ln", "|", "grep", ":8443")
	output, err := cmd.CombinedOutput()
	if err == nil && len(output) > 0 {
		h.logger.DebugWithContext("detected ClickHouse SSL port 8443", map[string]interface{}{
			"namespace": namespace,
			"pod":       podName,
			"output":    string(output),
		})
		return true, nil
	}

	// Fallback: check for SSL configuration files
	cmd = exec.CommandContext(ctx, "kubectl", "exec", "-n", namespace, podName, "--", "ls", "/ssl/server.crt")
	_, err = cmd.CombinedOutput()
	if err == nil {
		h.logger.DebugWithContext("detected ClickHouse SSL certificate", map[string]interface{}{
			"namespace": namespace,
			"pod":       podName,
		})
		return true, nil
	}

	// No SSL detected
	h.logger.DebugWithContext("ClickHouse SSL not detected, using HTTP", map[string]interface{}{
		"namespace": namespace,
		"pod":       podName,
	})
	return false, nil
}

// detectClickHouseConfigurationConflicts checks for common ClickHouse configuration issues
func (h *HealthChecker) detectClickHouseConfigurationConflicts(namespace, serviceName string) []string {
	var conflicts []string

	// Check for SSL/TLS configuration consistency
	if sslConflict := h.checkSSLConfigurationConflict(namespace, serviceName); sslConflict != "" {
		conflicts = append(conflicts, sslConflict)
	}

	// Check for authentication method conflicts
	if authConflict := h.checkAuthenticationConflict(namespace, serviceName); authConflict != "" {
		conflicts = append(conflicts, authConflict)
	}

	// Check for secret name conflicts
	if secretConflict := h.checkSecretNameConflict(namespace, serviceName); secretConflict != "" {
		conflicts = append(conflicts, secretConflict)
	}

	return conflicts
}

// checkSSLConfigurationConflict detects SSL configuration mismatches
func (h *HealthChecker) checkSSLConfigurationConflict(namespace, serviceName string) string {
	podName := serviceName + "-0"

	// Check if SSL is enabled but health checks are using HTTP
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if SSL port is listening
	cmd := exec.CommandContext(ctx, "kubectl", "exec", "-n", namespace, podName, "--", "netstat", "-ln")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "" // Can't determine, skip conflict detection
	}

	outputStr := string(output)
	httpsPortOpen := strings.Contains(outputStr, ":8443")
	httpPortOpen := strings.Contains(outputStr, ":8123")

	if httpsPortOpen && !httpPortOpen {
		return "SSL-only configuration detected but health check may be using HTTP port"
	}

	return ""
}

// checkAuthenticationConflict detects authentication configuration issues
func (h *HealthChecker) checkAuthenticationConflict(namespace, serviceName string) string {
	// Check if both environment variables and users.xml are configured
	podName := serviceName + "-0"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check for CLICKHOUSE_USER environment variable
	cmd := exec.CommandContext(ctx, "kubectl", "exec", "-n", namespace, podName, "--", "env")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "" // Can't determine
	}

	envHasClickHouseUser := strings.Contains(string(output), "CLICKHOUSE_USER=")
	envHasClickHousePassword := strings.Contains(string(output), "CLICKHOUSE_PASSWORD=")

	// Check for users.xml configuration
	cmd = exec.CommandContext(ctx, "kubectl", "exec", "-n", namespace, podName, "--", "cat", "/etc/clickhouse-server/users.xml")
	usersOutput, err := cmd.CombinedOutput()
	if err != nil {
		return "" // Can't determine
	}

	usersXmlHasUsers := strings.Contains(string(usersOutput), "<clickhouse_user>")

	if (envHasClickHouseUser || envHasClickHousePassword) && usersXmlHasUsers {
		return "Both environment variable and users.xml authentication detected - may cause conflicts"
	}

	return ""
}

// checkSecretNameConflict detects secret naming inconsistencies
func (h *HealthChecker) checkSecretNameConflict(namespace, serviceName string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if both old and new secret names exist
	oldSecretCmd := exec.CommandContext(ctx, "kubectl", "get", "secret", "clickhouse-secrets", "-n", namespace)
	newSecretCmd := exec.CommandContext(ctx, "kubectl", "get", "secret", "clickhouse-secrets", "-n", namespace)

	oldExists := oldSecretCmd.Run() == nil
	newExists := newSecretCmd.Run() == nil

	if oldExists && newExists {
		return "Both 'clickhouse-secrets' and 'clickhouse-secrets' exist - may cause confusion"
	}

	if !oldExists && !newExists {
		return "Neither 'clickhouse-secrets' nor 'clickhouse-secrets' found - deployment may fail"
	}

	return ""
}
