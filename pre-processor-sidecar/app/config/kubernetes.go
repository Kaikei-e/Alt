// ABOUTME: This file handles Kubernetes client configuration for OAuth2 token storage
// ABOUTME: Provides in-cluster and out-of-cluster client setup with proper authentication

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// LoadKubernetesConfig loads Kubernetes configuration from environment
func LoadKubernetesConfig() *KubernetesConfig {
	return &KubernetesConfig{
		InCluster:       getEnvOrDefault("KUBERNETES_IN_CLUSTER", "true") == "true",
		Namespace:       getEnvOrDefault("KUBERNETES_NAMESPACE", "alt-processing"),
		TokenSecretName: getEnvOrDefault("OAUTH2_TOKEN_SECRET_NAME", "pre-processor-sidecar-oauth2-token"),
	}
}

// CreateKubernetesClient creates a Kubernetes client based on configuration
func (k *KubernetesConfig) CreateKubernetesClient() (kubernetes.Interface, error) {
	var config *rest.Config
	var err error

	if k.InCluster {
		// Use in-cluster configuration
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
		}
	} else {
		// Use out-of-cluster configuration (kubeconfig file)
		kubeConfigPath := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create config from kubeconfig: %w", err)
		}
	}

	// Create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return clientset, nil
}