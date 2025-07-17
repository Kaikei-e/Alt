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
	h.logger.InfoWithContext("waiting for PostgreSQL service to be ready", map[string]interface{}{
		"namespace": namespace,
		"service": serviceName,
		"max_duration": "5m",
	})

	maxRetries := 30 // 5 minutes with 10 second intervals
	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			h.logger.ErrorWithContext("PostgreSQL wait cancelled", map[string]interface{}{
				"namespace": namespace,
				"service": serviceName,
				"attempt": i + 1,
				"error": ctx.Err().Error(),
			})
			return ctx.Err()
		default:
		}

		// Log current attempt
		h.logger.InfoWithContext("checking PostgreSQL connection", map[string]interface{}{
			"namespace": namespace,
			"service": serviceName,
			"attempt": i + 1,
			"max_retries": maxRetries,
		})

		// Check if PostgreSQL is ready to accept connections
		if err := h.checkPostgreSQLConnection(namespace, serviceName); err == nil {
			h.logger.InfoWithContext("PostgreSQL service is ready", map[string]interface{}{
				"namespace": namespace,
				"service": serviceName,
				"attempts": i + 1,
			})
			return nil
		} else {
			h.logger.WarnWithContext("PostgreSQL not ready, retrying", map[string]interface{}{
				"namespace": namespace,
				"service": serviceName,
				"attempt": i + 1,
				"max_retries": maxRetries,
				"error": err.Error(),
				"retry_delay": "10s",
			})
		}

		time.Sleep(10 * time.Second)
	}

	h.logger.ErrorWithContext("PostgreSQL service not ready after maximum attempts", map[string]interface{}{
		"namespace": namespace,
		"service": serviceName,
		"max_attempts": maxRetries,
		"total_duration": "5m",
	})
	return fmt.Errorf("PostgreSQL service %s in namespace %s not ready after %d attempts", serviceName, namespace, maxRetries)
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
		"service": serviceName,
		"pod": podName,
		"command": "pg_isready",
		"timeout": "30s",
	})

	cmd := exec.CommandContext(ctx, "kubectl", "exec", "-n", namespace, podName, "--", "pg_isready", "-U", "alt_db_user")
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			h.logger.WarnWithContext("PostgreSQL connection check timed out", map[string]interface{}{
				"namespace": namespace,
				"service": serviceName,
				"pod": podName,
				"timeout": "30s",
				"output": string(output),
			})
			return fmt.Errorf("PostgreSQL connection check timed out after 30s")
		}
		h.logger.DebugWithContext("PostgreSQL connection check failed", map[string]interface{}{
			"namespace": namespace,
			"service": serviceName,
			"pod": podName,
			"error": err.Error(),
			"output": string(output),
		})
		return fmt.Errorf("PostgreSQL connection check failed: %w", err)
	}

	// Check if output contains "accepting connections"
	if !strings.Contains(string(output), "accepting connections") {
		h.logger.DebugWithContext("PostgreSQL not accepting connections", map[string]interface{}{
			"namespace": namespace,
			"service": serviceName,
			"pod": podName,
			"output": string(output),
		})
		return fmt.Errorf("PostgreSQL not accepting connections: %s", string(output))
	}

	h.logger.DebugWithContext("PostgreSQL connection check successful", map[string]interface{}{
		"namespace": namespace,
		"service": serviceName,
		"pod": podName,
		"output": string(output),
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
		"service": serviceName,
		"type": serviceType,
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
			"service": serviceName,
			"type": serviceType,
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
		"namespace": namespace,
		"service": serviceName,
		"max_duration": "15m",
	})

	maxRetries := 90 // 15 minutes with 10 second intervals
	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			h.logger.ErrorWithContext("StatefulSet wait cancelled", map[string]interface{}{
				"namespace": namespace,
				"service": serviceName,
				"attempt": i + 1,
				"error": ctx.Err().Error(),
			})
			return ctx.Err()
		default:
		}

		// Log current attempt
		h.logger.InfoWithContext("checking StatefulSet readiness", map[string]interface{}{
			"namespace": namespace,
			"service": serviceName,
			"attempt": i + 1,
			"max_retries": maxRetries,
		})

		// Check if StatefulSet is ready
		if err := h.checkStatefulSetReady(namespace, serviceName); err == nil {
			h.logger.InfoWithContext("StatefulSet is ready", map[string]interface{}{
				"namespace": namespace,
				"service": serviceName,
				"attempts": i + 1,
			})
			return nil
		} else {
			h.logger.WarnWithContext("StatefulSet not ready, retrying", map[string]interface{}{
				"namespace": namespace,
				"service": serviceName,
				"attempt": i + 1,
				"max_retries": maxRetries,
				"error": err.Error(),
				"retry_delay": "10s",
			})
		}

		time.Sleep(10 * time.Second)
	}

	h.logger.ErrorWithContext("StatefulSet not ready after maximum attempts", map[string]interface{}{
		"namespace": namespace,
		"service": serviceName,
		"max_attempts": maxRetries,
		"total_duration": "15m",
	})
	return fmt.Errorf("StatefulSet %s in namespace %s not ready after %d attempts", serviceName, namespace, maxRetries)
}

// checkStatefulSetReady checks if StatefulSet is fully ready
func (h *HealthChecker) checkStatefulSetReady(namespace, serviceName string) error {
	h.logger.DebugWithContext("checking StatefulSet status", map[string]interface{}{
		"namespace": namespace,
		"service": serviceName,
	})

	// Check StatefulSet status
	cmd := exec.Command("kubectl", "get", "statefulset", serviceName, "-n", namespace, "-o", "jsonpath={.status.replicas},{.status.readyReplicas},{.status.currentReplicas}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		h.logger.DebugWithContext("StatefulSet status check failed", map[string]interface{}{
			"namespace": namespace,
			"service": serviceName,
			"error": err.Error(),
			"output": string(output),
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
		"namespace": namespace,
		"service": serviceName,
		"replicas": replicas,
		"ready_replicas": readyReplicas,
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
		"namespace": namespace,
		"service": serviceName,
		"max_duration": "10m",
	})

	maxRetries := 60 // 10 minutes with 10 second intervals
	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			h.logger.ErrorWithContext("ClickHouse wait cancelled", map[string]interface{}{
				"namespace": namespace,
				"service": serviceName,
				"attempt": i + 1,
				"error": ctx.Err().Error(),
			})
			return ctx.Err()
		default:
		}

		// Log current attempt
		h.logger.InfoWithContext("checking ClickHouse connection", map[string]interface{}{
			"namespace": namespace,
			"service": serviceName,
			"attempt": i + 1,
			"max_retries": maxRetries,
		})

		// Check if ClickHouse is ready
		if err := h.checkClickHouseHealth(namespace, serviceName); err == nil {
			h.logger.InfoWithContext("ClickHouse service is ready", map[string]interface{}{
				"namespace": namespace,
				"service": serviceName,
				"attempts": i + 1,
			})
			return nil
		} else {
			h.logger.WarnWithContext("ClickHouse not ready, retrying", map[string]interface{}{
				"namespace": namespace,
				"service": serviceName,
				"attempt": i + 1,
				"max_retries": maxRetries,
				"error": err.Error(),
				"retry_delay": "10s",
			})
		}

		time.Sleep(10 * time.Second)
	}

	h.logger.ErrorWithContext("ClickHouse service not ready after maximum attempts", map[string]interface{}{
		"namespace": namespace,
		"service": serviceName,
		"max_attempts": maxRetries,
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
	h.logger.DebugWithContext("executing ClickHouse health check", map[string]interface{}{
		"namespace": namespace,
		"service": serviceName,
		"pod": podName,
		"endpoint": "ping",
		"timeout": "30s",
	})

	cmd := exec.CommandContext(ctx, "kubectl", "exec", "-n", namespace, podName, "--", "curl", "-f", "http://localhost:8123/ping")
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			h.logger.WarnWithContext("ClickHouse health check timed out", map[string]interface{}{
				"namespace": namespace,
				"service": serviceName,
				"pod": podName,
				"timeout": "30s",
				"output": string(output),
			})
			return fmt.Errorf("ClickHouse health check timed out after 30s")
		}
		h.logger.DebugWithContext("ClickHouse health check failed", map[string]interface{}{
			"namespace": namespace,
			"service": serviceName,
			"pod": podName,
			"error": err.Error(),
			"output": string(output),
		})
		return fmt.Errorf("ClickHouse health check failed: %w", err)
	}

	// Check if output contains "Ok"
	if !strings.Contains(string(output), "Ok") {
		h.logger.DebugWithContext("ClickHouse not responding correctly", map[string]interface{}{
			"namespace": namespace,
			"service": serviceName,
			"pod": podName,
			"output": string(output),
		})
		return fmt.Errorf("ClickHouse not responding correctly: %s", string(output))
	}

	h.logger.DebugWithContext("ClickHouse health check successful", map[string]interface{}{
		"namespace": namespace,
		"service": serviceName,
		"pod": podName,
		"output": string(output),
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