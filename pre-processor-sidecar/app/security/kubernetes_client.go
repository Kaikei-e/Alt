// ABOUTME: Kubernetes API client management for OAuth2 token storage
// ABOUTME: Provides in-cluster configuration and clientset creation

package security

import (
	"context"
	"fmt"
	"log/slog"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// KubernetesAuthManager provides Kubernetes API client management
type KubernetesAuthManager struct {
	logger    *slog.Logger
	config    *rest.Config
	clientset kubernetes.Interface
}

// NewKubernetesAuthManager creates a new Kubernetes auth manager
func NewKubernetesAuthManager(logger *slog.Logger) (*KubernetesAuthManager, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Create in-cluster config (for Pod environment)
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Error("Failed to create in-cluster config", "error", err)
		return nil, fmt.Errorf("failed to create Kubernetes config: %w", err)
	}

	// Create clientset from config
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Error("Failed to create Kubernetes clientset", "error", err)
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	logger.Info("Kubernetes auth manager initialized successfully")

	return &KubernetesAuthManager{
		logger:    logger,
		config:    config,
		clientset: clientset,
	}, nil
}

// GetKubernetesClientset returns the Kubernetes clientset
func (m *KubernetesAuthManager) GetKubernetesClientset() (kubernetes.Interface, error) {
	if m.clientset == nil {
		return nil, fmt.Errorf("Kubernetes clientset not initialized")
	}
	return m.clientset, nil
}

// TestConnectivity tests if we can connect to Kubernetes API
func (m *KubernetesAuthManager) TestConnectivity(ctx context.Context) error {
	// Try to get server version as a simple connectivity test
	version, err := m.clientset.Discovery().ServerVersion()
	if err != nil {
		m.logger.Error("Kubernetes API connectivity test failed", "error", err)
		return fmt.Errorf("Kubernetes API connectivity test failed: %w", err)
	}

	m.logger.Info("Kubernetes API connectivity confirmed",
		"server_version", version.String())
	return nil
}