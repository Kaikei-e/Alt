// PHASE R1: Namespace management functionality
package core

import (
	"context"
	"fmt"

	"deploy-cli/domain"
	"deploy-cli/gateway/kubectl_gateway"
	"deploy-cli/port/logger_port"
)

// NamespaceManager handles namespace operations
type NamespaceManager struct {
	kubectlGateway *kubectl_gateway.KubectlGateway
	logger         logger_port.LoggerPort
}

// NamespaceManagerPort defines the interface for namespace management
type NamespaceManagerPort interface {
	EnsureNamespace(ctx context.Context, namespace string) error
	EnsureNamespacesForCharts(ctx context.Context, charts []domain.Chart, options *domain.DeploymentOptions) error
	ValidateNamespaceAccess(ctx context.Context, namespace string) error
	GetNamespaceStatus(ctx context.Context, namespace string) (*domain.NamespaceStatus, error)
	CleanupNamespace(ctx context.Context, namespace string) error
}

// NewNamespaceManager creates a new namespace manager
func NewNamespaceManager(
	kubectlGateway *kubectl_gateway.KubectlGateway,
	logger logger_port.LoggerPort,
) *NamespaceManager {
	return &NamespaceManager{
		kubectlGateway: kubectlGateway,
		logger:         logger,
	}
}

// EnsureNamespace ensures a namespace exists
func (n *NamespaceManager) EnsureNamespace(ctx context.Context, namespace string) error {
	n.logger.DebugWithContext("ensuring namespace exists", map[string]interface{}{
		"namespace": namespace,
	})

	// Check if namespace already exists
	exists, err := n.kubectlGateway.NamespaceExists(ctx, namespace)
	if err != nil {
		n.logger.ErrorWithContext("failed to check namespace existence", map[string]interface{}{
			"namespace": namespace,
			"error":     err.Error(),
		})
		return fmt.Errorf("failed to check namespace existence: %w", err)
	}

	if exists {
		n.logger.DebugWithContext("namespace already exists", map[string]interface{}{
			"namespace": namespace,
		})
		return nil
	}

	// Create namespace
	if err := n.kubectlGateway.CreateNamespace(ctx, namespace); err != nil {
		n.logger.ErrorWithContext("failed to create namespace", map[string]interface{}{
			"namespace": namespace,
			"error":     err.Error(),
		})
		return fmt.Errorf("failed to create namespace %s: %w", namespace, err)
	}

	n.logger.InfoWithContext("namespace created successfully", map[string]interface{}{
		"namespace": namespace,
	})

	return nil
}

// EnsureNamespacesForCharts ensures all namespaces needed for charts exist
func (n *NamespaceManager) EnsureNamespacesForCharts(ctx context.Context, charts []domain.Chart, options *domain.DeploymentOptions) error {
	n.logger.InfoWithContext("ensuring namespaces for charts", map[string]interface{}{
		"chart_count": len(charts),
	})

	// Collect unique namespaces
	namespaces := make(map[string]bool)
	for _, chart := range charts {
		namespace := options.GetNamespace(chart.Name)
		namespaces[namespace] = true

		// Handle multi-namespace charts
		if chart.MultiNamespace {
			for _, targetNamespace := range chart.TargetNamespaces {
				namespaces[targetNamespace] = true
			}
		}
	}

	n.logger.DebugWithContext("unique namespaces identified", map[string]interface{}{
		"namespace_count": len(namespaces),
		"namespaces":      n.getNamespaceList(namespaces),
	})

	// Ensure each namespace exists
	for namespace := range namespaces {
		if err := n.EnsureNamespace(ctx, namespace); err != nil {
			return fmt.Errorf("failed to ensure namespace %s: %w", namespace, err)
		}
	}

	n.logger.InfoWithContext("all namespaces ensured", map[string]interface{}{
		"namespace_count": len(namespaces),
	})

	return nil
}

// ValidateNamespaceAccess validates that we have access to a namespace
func (n *NamespaceManager) ValidateNamespaceAccess(ctx context.Context, namespace string) error {
	n.logger.DebugWithContext("validating namespace access", map[string]interface{}{
		"namespace": namespace,
	})

	// Try to get namespace information
	if err := n.kubectlGateway.ValidateNamespaceAccess(ctx, namespace); err != nil {
		n.logger.ErrorWithContext("namespace access validation failed", map[string]interface{}{
			"namespace": namespace,
			"error":     err.Error(),
		})
		return fmt.Errorf("no access to namespace %s: %w", namespace, err)
	}

	n.logger.DebugWithContext("namespace access validated", map[string]interface{}{
		"namespace": namespace,
	})

	return nil
}

// GetNamespaceStatus gets the status of a namespace
func (n *NamespaceManager) GetNamespaceStatus(ctx context.Context, namespace string) (*domain.NamespaceStatus, error) {
	n.logger.DebugWithContext("getting namespace status", map[string]interface{}{
		"namespace": namespace,
	})

	status, err := n.kubectlGateway.GetNamespaceStatus(ctx, namespace)
	if err != nil {
		n.logger.ErrorWithContext("failed to get namespace status", map[string]interface{}{
			"namespace": namespace,
			"error":     err.Error(),
		})
		return nil, fmt.Errorf("failed to get namespace status: %w", err)
	}

	return status, nil
}

// CleanupNamespace cleans up resources in a namespace
func (n *NamespaceManager) CleanupNamespace(ctx context.Context, namespace string) error {
	n.logger.InfoWithContext("cleaning up namespace", map[string]interface{}{
		"namespace": namespace,
	})

	if err := n.kubectlGateway.CleanupNamespace(ctx, namespace); err != nil {
		n.logger.ErrorWithContext("namespace cleanup failed", map[string]interface{}{
			"namespace": namespace,
			"error":     err.Error(),
		})
		return fmt.Errorf("failed to cleanup namespace %s: %w", namespace, err)
	}

	n.logger.InfoWithContext("namespace cleanup completed", map[string]interface{}{
		"namespace": namespace,
	})

	return nil
}

// getNamespaceList converts namespace map to slice for logging
func (n *NamespaceManager) getNamespaceList(namespaces map[string]bool) []string {
	list := make([]string, 0, len(namespaces))
	for namespace := range namespaces {
		list = append(list, namespace)
	}
	return list
}