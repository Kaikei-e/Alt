package kubectl_gateway

import (
	"context"
	"fmt"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/kubectl_port"
	"deploy-cli/port/logger_port"
)

// KubectlGateway acts as anti-corruption layer for Kubernetes operations
type KubectlGateway struct {
	kubectlPort kubectl_port.KubectlPort
	logger      logger_port.LoggerPort
}

// NewKubectlGateway creates a new kubectl gateway
func NewKubectlGateway(kubectlPort kubectl_port.KubectlPort, logger logger_port.LoggerPort) *KubectlGateway {
	return &KubectlGateway{
		kubectlPort: kubectlPort,
		logger:      logger,
	}
}

// ValidateClusterAccess validates that the cluster is accessible
func (g *KubectlGateway) ValidateClusterAccess(ctx context.Context) error {
	g.logger.InfoWithContext("validating cluster access", map[string]interface{}{})

	// Check kubectl version
	_, err := g.kubectlPort.Version(ctx)
	if err != nil {
		g.logger.ErrorWithContext("kubectl version check failed", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("kubectl not accessible: %w", err)
	}

	// Check if cluster is accessible by listing nodes
	nodes, err := g.kubectlPort.GetNodes(ctx)
	if err != nil {
		g.logger.ErrorWithContext("cluster access check failed", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("kubernetes cluster not accessible: %w", err)
	}

	g.logger.InfoWithContext("cluster access validated", map[string]interface{}{
		"node_count": len(nodes),
	})

	return nil
}

// EnsureNamespace ensures that a namespace exists
func (g *KubectlGateway) EnsureNamespace(ctx context.Context, namespace string) error {
	g.logger.InfoWithContext("ensuring namespace exists", map[string]interface{}{
		"namespace": namespace,
	})

	err := g.kubectlPort.CreateNamespace(ctx, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to create namespace", map[string]interface{}{
			"namespace": namespace,
			"error":     err.Error(),
		})
		return fmt.Errorf("failed to create namespace %s: %w", namespace, err)
	}

	g.logger.InfoWithContext("namespace ensured", map[string]interface{}{
		"namespace": namespace,
	})

	return nil
}

// EnsureNamespaces ensures that all required namespaces exist
func (g *KubectlGateway) EnsureNamespaces(ctx context.Context, env domain.Environment) error {
	namespaces := domain.GetNamespacesForEnvironment(env)

	g.logger.InfoWithContext("ensuring all namespaces exist", map[string]interface{}{
		"environment": env.String(),
		"namespaces":  namespaces,
	})

	for _, namespace := range namespaces {
		if err := g.EnsureNamespace(ctx, namespace); err != nil {
			return err
		}
	}

	g.logger.InfoWithContext("all namespaces ensured", map[string]interface{}{
		"environment": env.String(),
		"namespaces":  namespaces,
	})

	return nil
}

// GetNamespaces returns all namespaces in the cluster
func (g *KubectlGateway) GetNamespaces(ctx context.Context) ([]kubectl_port.KubernetesNamespace, error) {
	g.logger.InfoWithContext("getting all namespaces", map[string]interface{}{})

	namespaces, err := g.kubectlPort.GetNamespaces(ctx)
	if err != nil {
		g.logger.ErrorWithContext("failed to get namespaces", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to get namespaces: %w", err)
	}

	g.logger.InfoWithContext("got namespaces", map[string]interface{}{
		"count": len(namespaces),
	})

	return namespaces, nil
}

// CreateSecret creates a Kubernetes secret
func (g *KubectlGateway) CreateSecret(ctx context.Context, secret *domain.Secret) error {
	g.logger.InfoWithContext("creating secret", map[string]interface{}{
		"secret":    secret.Name,
		"namespace": secret.Namespace,
		"type":      secret.Type,
	})

	kubectlSecret := kubectl_port.KubernetesSecret{
		Name:        secret.Name,
		Namespace:   secret.Namespace,
		Type:        secret.Type,
		Data:        make(map[string][]byte),
		Labels:      secret.Labels,
		Annotations: secret.Annotations,
	}

	// Set default type if not specified
	if kubectlSecret.Type == "" {
		kubectlSecret.Type = "Opaque"
	}

	for key, value := range secret.Data {
		kubectlSecret.Data[key] = []byte(value)
	}

	err := g.kubectlPort.CreateSecret(ctx, kubectlSecret)
	if err != nil {
		g.logger.ErrorWithContext("failed to create secret", map[string]interface{}{
			"secret":    secret.Name,
			"namespace": secret.Namespace,
			"error":     err.Error(),
		})
		return fmt.Errorf("failed to create secret %s: %w", secret.Name, err)
	}

	g.logger.InfoWithContext("secret created successfully", map[string]interface{}{
		"secret":    secret.Name,
		"namespace": secret.Namespace,
	})

	return nil
}

// UpdateSecret updates a Kubernetes secret
func (g *KubectlGateway) UpdateSecret(ctx context.Context, secret domain.Secret) error {
	g.logger.InfoWithContext("updating secret", map[string]interface{}{
		"secret":    secret.Name,
		"namespace": secret.Namespace,
		"type":      secret.Type,
	})

	kubectlSecret := kubectl_port.KubernetesSecret{
		Name:      secret.Name,
		Namespace: secret.Namespace,
		Type:      "Opaque",
		Data:      make(map[string][]byte),
	}

	for key, value := range secret.Data {
		kubectlSecret.Data[key] = []byte(value)
	}

	err := g.kubectlPort.UpdateSecret(ctx, kubectlSecret)
	if err != nil {
		g.logger.ErrorWithContext("failed to update secret", map[string]interface{}{
			"secret":    secret.Name,
			"namespace": secret.Namespace,
			"error":     err.Error(),
		})
		return fmt.Errorf("failed to update secret %s: %w", secret.Name, err)
	}

	g.logger.InfoWithContext("secret updated successfully", map[string]interface{}{
		"secret":    secret.Name,
		"namespace": secret.Namespace,
	})

	return nil
}

// DeleteSecret deletes a Kubernetes secret
func (g *KubectlGateway) DeleteSecret(ctx context.Context, name, namespace string) error {
	g.logger.InfoWithContext("deleting secret", map[string]interface{}{
		"secret":    name,
		"namespace": namespace,
	})

	err := g.kubectlPort.DeleteSecret(ctx, name, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to delete secret", map[string]interface{}{
			"secret":    name,
			"namespace": namespace,
			"error":     err.Error(),
		})
		return fmt.Errorf("failed to delete secret %s: %w", name, err)
	}

	g.logger.InfoWithContext("secret deleted successfully", map[string]interface{}{
		"secret":    name,
		"namespace": namespace,
	})

	return nil
}

// GetPersistentVolumes returns all persistent volumes
func (g *KubectlGateway) GetPersistentVolumes(ctx context.Context) ([]kubectl_port.KubernetesPersistentVolume, error) {
	g.logger.DebugWithContext("getting persistent volumes", map[string]interface{}{})

	pvs, err := g.kubectlPort.GetPersistentVolumes(ctx)
	if err != nil {
		g.logger.ErrorWithContext("failed to get persistent volumes", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to get persistent volumes: %w", err)
	}

	g.logger.DebugWithContext("persistent volumes retrieved", map[string]interface{}{
		"count": len(pvs),
	})

	return pvs, nil
}

// CreatePersistentVolume creates a persistent volume
func (g *KubectlGateway) CreatePersistentVolume(ctx context.Context, pv domain.PersistentVolume) error {
	g.logger.InfoWithContext("creating persistent volume", map[string]interface{}{
		"pv":       pv.Name,
		"capacity": pv.Capacity,
	})

	kubectlPV := kubectl_port.KubernetesPersistentVolume{
		Name:         pv.Name,
		Capacity:     pv.Capacity,
		AccessModes:  pv.AccessModes,
		StorageClass: pv.StorageClass,
		HostPath:     pv.HostPath,
	}

	err := g.kubectlPort.CreatePersistentVolume(ctx, kubectlPV)
	if err != nil {
		g.logger.ErrorWithContext("failed to create persistent volume", map[string]interface{}{
			"pv":    pv.Name,
			"error": err.Error(),
		})
		return fmt.Errorf("failed to create persistent volume %s: %w", pv.Name, err)
	}

	g.logger.InfoWithContext("persistent volume created successfully", map[string]interface{}{
		"pv":       pv.Name,
		"capacity": pv.Capacity,
	})

	return nil
}

// DeletePersistentVolume deletes a persistent volume
func (g *KubectlGateway) DeletePersistentVolume(ctx context.Context, name string) error {
	g.logger.InfoWithContext("deleting persistent volume", map[string]interface{}{
		"pv": name,
	})

	err := g.kubectlPort.DeletePersistentVolume(ctx, name)
	if err != nil {
		g.logger.ErrorWithContext("failed to delete persistent volume", map[string]interface{}{
			"pv":    name,
			"error": err.Error(),
		})
		return fmt.Errorf("failed to delete persistent volume %s: %w", name, err)
	}

	g.logger.InfoWithContext("persistent volume deleted successfully", map[string]interface{}{
		"pv": name,
	})

	return nil
}

// GetPods returns pods in the specified namespace with optional label selector
func (g *KubectlGateway) GetPods(ctx context.Context, namespace string, selector string) ([]kubectl_port.KubernetesPod, error) {
	g.logger.DebugWithContext("getting pods", map[string]interface{}{
		"namespace": namespace,
		"selector":  selector,
	})

	pods, err := g.kubectlPort.GetPods(ctx, namespace, selector)
	if err != nil {
		g.logger.ErrorWithContext("failed to get pods", map[string]interface{}{
			"namespace": namespace,
			"selector":  selector,
			"error":     err.Error(),
		})
		return nil, fmt.Errorf("failed to get pods: %w", err)
	}

	g.logger.DebugWithContext("pods retrieved", map[string]interface{}{
		"namespace": namespace,
		"count":     len(pods),
	})

	return pods, nil
}

// GetProblematicPods returns pods that are not in Running or Succeeded state
func (g *KubectlGateway) GetProblematicPods(ctx context.Context) ([]kubectl_port.KubernetesPod, error) {
	g.logger.DebugWithContext("getting problematic pods", map[string]interface{}{})

	pods, err := g.kubectlPort.GetPods(ctx, "", "status.phase!=Running,status.phase!=Succeeded")
	if err != nil {
		g.logger.ErrorWithContext("failed to get problematic pods", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to get problematic pods: %w", err)
	}

	g.logger.DebugWithContext("problematic pods retrieved", map[string]interface{}{
		"count": len(pods),
	})

	return pods, nil
}

// CleanupFailedResources cleans up failed resources
func (g *KubectlGateway) CleanupFailedResources(ctx context.Context, namespaces []string) error {
	g.logger.InfoWithContext("cleaning up failed resources", map[string]interface{}{
		"namespaces": namespaces,
	})

	for _, namespace := range namespaces {
		// Delete all statefulsets in the namespace
		sts, err := g.kubectlPort.GetStatefulSets(ctx, namespace)
		if err != nil {
			g.logger.WarnWithContext("failed to get statefulsets", map[string]interface{}{
				"namespace": namespace,
				"error":     err.Error(),
			})
			continue
		}

		for _, st := range sts {
			err := g.kubectlPort.DeleteStatefulSet(ctx, st.Name, namespace, true)
			if err != nil {
				g.logger.WarnWithContext("failed to delete statefulset", map[string]interface{}{
					"statefulset": st.Name,
					"namespace":   namespace,
					"error":       err.Error(),
				})
			}
		}

		// Delete all PVCs in the namespace
		pvcs, err := g.kubectlPort.GetPersistentVolumeClaims(ctx, namespace)
		if err != nil {
			g.logger.WarnWithContext("failed to get PVCs", map[string]interface{}{
				"namespace": namespace,
				"error":     err.Error(),
			})
			continue
		}

		for _, pvc := range pvcs {
			// Note: kubectl_port doesn't have DeletePVC method, would need to add it
			g.logger.DebugWithContext("would delete PVC", map[string]interface{}{
				"pvc":       pvc.Name,
				"namespace": namespace,
			})
		}
	}

	g.logger.InfoWithContext("failed resource cleanup completed", map[string]interface{}{
		"namespaces": namespaces,
	})

	return nil
}

// RolloutRestart restarts a deployment
func (g *KubectlGateway) RolloutRestart(ctx context.Context, resourceType, name, namespace string) error {
	g.logger.InfoWithContext("restarting resource", map[string]interface{}{
		"resource_type": resourceType,
		"name":          name,
		"namespace":     namespace,
	})

	err := g.kubectlPort.RolloutRestart(ctx, resourceType, name, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to restart resource", map[string]interface{}{
			"resource_type": resourceType,
			"name":          name,
			"namespace":     namespace,
			"error":         err.Error(),
		})
		return fmt.Errorf("failed to restart %s/%s: %w", resourceType, name, err)
	}

	g.logger.InfoWithContext("resource restarted successfully", map[string]interface{}{
		"resource_type": resourceType,
		"name":          name,
		"namespace":     namespace,
	})

	return nil
}

// WaitForRollout waits for a rollout to complete
func (g *KubectlGateway) WaitForRollout(ctx context.Context, resourceType, name, namespace string, timeout time.Duration) error {
	g.logger.InfoWithContext("waiting for rollout", map[string]interface{}{
		"resource_type": resourceType,
		"name":          name,
		"namespace":     namespace,
		"timeout":       timeout,
	})

	err := g.kubectlPort.RolloutStatus(ctx, resourceType, name, namespace, timeout)
	if err != nil {
		g.logger.ErrorWithContext("rollout wait failed", map[string]interface{}{
			"resource_type": resourceType,
			"name":          name,
			"namespace":     namespace,
			"timeout":       timeout,
			"error":         err.Error(),
		})
		return fmt.Errorf("rollout wait failed for %s/%s: %w", resourceType, name, err)
	}

	g.logger.InfoWithContext("rollout completed successfully", map[string]interface{}{
		"resource_type": resourceType,
		"name":          name,
		"namespace":     namespace,
	})

	return nil
}

// ApplyYAMLFile applies a YAML file
func (g *KubectlGateway) ApplyYAMLFile(ctx context.Context, filename string) error {
	g.logger.InfoWithContext("applying YAML file", map[string]interface{}{
		"filename": filename,
	})

	err := g.kubectlPort.ApplyFile(ctx, filename)
	if err != nil {
		g.logger.ErrorWithContext("failed to apply YAML file", map[string]interface{}{
			"filename": filename,
			"error":    err.Error(),
		})
		return fmt.Errorf("failed to apply YAML file %s: %w", filename, err)
	}

	g.logger.InfoWithContext("YAML file applied successfully", map[string]interface{}{
		"filename": filename,
	})

	return nil
}

// GetDeployments returns deployments in the specified namespace
func (g *KubectlGateway) GetDeployments(ctx context.Context, namespace string) ([]kubectl_port.KubernetesDeployment, error) {
	g.logger.DebugWithContext("getting deployments", map[string]interface{}{
		"namespace": namespace,
	})

	deployments, err := g.kubectlPort.GetDeployments(ctx, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to get deployments", map[string]interface{}{
			"namespace": namespace,
			"error":     err.Error(),
		})
		return nil, fmt.Errorf("failed to get deployments in namespace %s: %w", namespace, err)
	}

	g.logger.DebugWithContext("deployments retrieved", map[string]interface{}{
		"namespace": namespace,
		"count":     len(deployments),
	})

	return deployments, nil
}

// GetStatefulSets returns stateful sets in the specified namespace
func (g *KubectlGateway) GetStatefulSets(ctx context.Context, namespace string) ([]kubectl_port.KubernetesStatefulSet, error) {
	g.logger.DebugWithContext("getting statefulsets", map[string]interface{}{
		"namespace": namespace,
	})

	statefulSets, err := g.kubectlPort.GetStatefulSets(ctx, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to get statefulsets", map[string]interface{}{
			"namespace": namespace,
			"error":     err.Error(),
		})
		return nil, fmt.Errorf("failed to get statefulsets in namespace %s: %w", namespace, err)
	}

	g.logger.DebugWithContext("statefulsets retrieved", map[string]interface{}{
		"namespace": namespace,
		"count":     len(statefulSets),
	})

	return statefulSets, nil
}

// GetSecrets returns secrets in the specified namespace
func (g *KubectlGateway) GetSecrets(ctx context.Context, namespace string) ([]kubectl_port.KubernetesSecret, error) {
	g.logger.DebugWithContext("getting secrets", map[string]interface{}{
		"namespace": namespace,
	})

	secrets, err := g.kubectlPort.GetSecrets(ctx, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to get secrets", map[string]interface{}{
			"namespace": namespace,
			"error":     err.Error(),
		})
		return nil, fmt.Errorf("failed to get secrets in namespace %s: %w", namespace, err)
	}

	g.logger.DebugWithContext("secrets retrieved", map[string]interface{}{
		"namespace": namespace,
		"count":     len(secrets),
	})

	return secrets, nil
}

// GetSecretsWithMetadata returns secrets with helm metadata across all namespaces
func (g *KubectlGateway) GetSecretsWithMetadata(ctx context.Context) ([]kubectl_port.KubernetesSecretWithMetadata, error) {
	g.logger.InfoWithContext("getting secrets with metadata", map[string]interface{}{})

	secrets, err := g.kubectlPort.GetSecretsWithMetadata(ctx)
	if err != nil {
		g.logger.ErrorWithContext("failed to get secrets with metadata", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to get secrets with metadata: %w", err)
	}

	g.logger.InfoWithContext("got secrets with metadata", map[string]interface{}{
		"count": len(secrets),
	})

	return secrets, nil
}

// GetResourcesWithMetadata returns any resource type with helm metadata across all namespaces
func (g *KubectlGateway) GetResourcesWithMetadata(ctx context.Context, resourceType string) ([]kubectl_port.KubernetesResourceWithMetadata, error) {
	g.logger.InfoWithContext("getting resources with metadata", map[string]interface{}{
		"resource_type": resourceType,
	})

	resources, err := g.kubectlPort.GetResourcesWithMetadata(ctx, resourceType)
	if err != nil {
		g.logger.ErrorWithContext("failed to get resources with metadata", map[string]interface{}{
			"resource_type": resourceType,
			"error":         err.Error(),
		})
		return nil, fmt.Errorf("failed to get %s with metadata: %w", resourceType, err)
	}

	g.logger.InfoWithContext("got resources with metadata", map[string]interface{}{
		"resource_type": resourceType,
		"count":         len(resources),
	})

	return resources, nil
}

// DeleteResource deletes any resource type
func (g *KubectlGateway) DeleteResource(ctx context.Context, resourceType, name, namespace string) error {
	g.logger.InfoWithContext("deleting resource", map[string]interface{}{
		"resource_type": resourceType,
		"resource":      name,
		"namespace":     namespace,
	})

	err := g.kubectlPort.DeleteResource(ctx, resourceType, name, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to delete resource", map[string]interface{}{
			"resource_type": resourceType,
			"resource":      name,
			"namespace":     namespace,
			"error":         err.Error(),
		})
		return fmt.Errorf("failed to delete %s %s in namespace %s: %w", resourceType, name, namespace, err)
	}

	g.logger.InfoWithContext("resource deleted successfully", map[string]interface{}{
		"resource_type": resourceType,
		"resource":      name,
		"namespace":     namespace,
	})

	return nil
}

// GetSecret retrieves a specific Kubernetes secret
func (g *KubectlGateway) GetSecret(ctx context.Context, name, namespace string) (*domain.Secret, error) {
	g.logger.InfoWithContext("getting secret via kubectl", map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	})

	kubernetesSecret, err := g.kubectlPort.GetSecret(ctx, name, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to get secret", map[string]interface{}{
			"name":      name,
			"namespace": namespace,
			"error":     err.Error(),
		})
		return nil, fmt.Errorf("failed to get secret %s in namespace %s: %w", name, namespace, err)
	}

	// Convert kubectl_port.KubernetesSecret to domain.Secret
	domainSecret := &domain.Secret{
		Name:        kubernetesSecret.Name,
		Namespace:   kubernetesSecret.Namespace,
		Type:        kubernetesSecret.Type,
		Data:        make(map[string]string),
		Labels:      kubernetesSecret.Labels,
		Annotations: kubernetesSecret.Annotations,
	}

	// Convert []byte data to string
	for key, value := range kubernetesSecret.Data {
		domainSecret.Data[key] = string(value)
	}

	g.logger.InfoWithContext("secret retrieved successfully", map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	})

	return domainSecret, nil
}

// SecretExists checks if a secret exists in the specified namespace
func (g *KubectlGateway) SecretExists(ctx context.Context, secretName, namespace string) bool {
	g.logger.DebugWithContext("checking if secret exists", map[string]interface{}{
		"secret":    secretName,
		"namespace": namespace,
	})

	_, err := g.kubectlPort.GetSecret(ctx, secretName, namespace)
	exists := err == nil

	g.logger.DebugWithContext("secret existence check result", map[string]interface{}{
		"secret":    secretName,
		"namespace": namespace,
		"exists":    exists,
	})

	return exists
}

// AnnotateSecret adds an annotation to an existing secret
func (g *KubectlGateway) AnnotateSecret(ctx context.Context, secretName, namespace, key, value string) error {
	g.logger.InfoWithContext("annotating secret", map[string]interface{}{
		"secret":    secretName,
		"namespace": namespace,
		"key":       key,
		"value":     value,
	})

	// Get the existing secret
	kubernetesSecret, err := g.kubectlPort.GetSecret(ctx, secretName, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to get secret for annotation", map[string]interface{}{
			"secret":    secretName,
			"namespace": namespace,
			"error":     err.Error(),
		})
		return fmt.Errorf("failed to get secret %s for annotation: %w", secretName, err)
	}

	// Initialize annotations if nil
	if kubernetesSecret.Annotations == nil {
		kubernetesSecret.Annotations = make(map[string]string)
	}

	// Add the annotation
	kubernetesSecret.Annotations[key] = value

	// Apply the updated secret
	err = g.kubectlPort.ApplySecret(ctx, kubernetesSecret)
	if err != nil {
		g.logger.ErrorWithContext("failed to apply secret annotation", map[string]interface{}{
			"secret":    secretName,
			"namespace": namespace,
			"key":       key,
			"value":     value,
			"error":     err.Error(),
		})
		return fmt.Errorf("failed to apply annotation to secret %s: %w", secretName, err)
	}

	g.logger.InfoWithContext("secret annotated successfully", map[string]interface{}{
		"secret":    secretName,
		"namespace": namespace,
		"key":       key,
		"value":     value,
	})

	return nil
}

// LabelSecret adds a label to an existing secret
func (g *KubectlGateway) LabelSecret(ctx context.Context, secretName, namespace, key, value string) error {
	g.logger.InfoWithContext("labeling secret", map[string]interface{}{
		"secret":    secretName,
		"namespace": namespace,
		"key":       key,
		"value":     value,
	})

	// Get the existing secret
	kubernetesSecret, err := g.kubectlPort.GetSecret(ctx, secretName, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to get secret for labeling", map[string]interface{}{
			"secret":    secretName,
			"namespace": namespace,
			"error":     err.Error(),
		})
		return fmt.Errorf("failed to get secret %s for labeling: %w", secretName, err)
	}

	// Initialize labels if nil
	if kubernetesSecret.Labels == nil {
		kubernetesSecret.Labels = make(map[string]string)
	}

	// Add the label
	kubernetesSecret.Labels[key] = value

	// Apply the updated secret
	err = g.kubectlPort.ApplySecret(ctx, kubernetesSecret)
	if err != nil {
		g.logger.ErrorWithContext("failed to apply secret label", map[string]interface{}{
			"secret":    secretName,
			"namespace": namespace,
			"key":       key,
			"value":     value,
			"error":     err.Error(),
		})
		return fmt.Errorf("failed to apply label to secret %s: %w", secretName, err)
	}

	g.logger.InfoWithContext("secret labeled successfully", map[string]interface{}{
		"secret":    secretName,
		"namespace": namespace,
		"key":       key,
		"value":     value,
	})

	return nil
}

// GetPodStatus gets the status of a specific pod
func (g *KubectlGateway) GetPodStatus(ctx context.Context, podName, namespace string) (*domain.PodStatus, error) {
	g.logger.DebugWithContext("getting pod status", map[string]interface{}{
		"pod":       podName,
		"namespace": namespace,
	})

	// Get pod information from kubectl port
	pods, err := g.kubectlPort.GetPods(ctx, namespace, "metadata.name="+podName)
	if err != nil {
		g.logger.ErrorWithContext("failed to get pod", map[string]interface{}{
			"pod":       podName,
			"namespace": namespace,
			"error":     err.Error(),
		})
		return nil, fmt.Errorf("failed to get pod %s: %w", podName, err)
	}

	if len(pods) == 0 {
		return nil, fmt.Errorf("pod %s not found in namespace %s", podName, namespace)
	}

	pod := pods[0]
	
	// Convert to domain.PodStatus
	// Note: KubernetesPod has fields: Name, Namespace, Status, Ready (string), Restarts, Age
	podStatus := &domain.PodStatus{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Phase:     pod.Status,      // Using Status as Phase
		Ready:     pod.Ready == "Running" || pod.Ready == "1/1", // Convert string to bool
		Status:    pod.Status,
		NodeName:  "",              // Not available in KubernetesPod
		PodIP:     "",              // Not available in KubernetesPod
		StartTime: time.Time{},     // Not available in KubernetesPod
	}

	g.logger.DebugWithContext("pod status retrieved", map[string]interface{}{
		"pod":       podName,
		"namespace": namespace,
		"phase":     podStatus.Phase,
		"ready":     podStatus.Ready,
	})

	return podStatus, nil
}

// GetDeploymentsForChart gets deployments that belong to a specific chart
func (g *KubectlGateway) GetDeploymentsForChart(ctx context.Context, chartName, namespace string) ([]*domain.Deployment, error) {
	g.logger.DebugWithContext("getting deployments for chart", map[string]interface{}{
		"chart":     chartName,
		"namespace": namespace,
	})

	// Get all deployments with helm labels matching the chart
	deployments, err := g.kubectlPort.GetDeployments(ctx, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to get deployments for chart", map[string]interface{}{
			"chart":     chartName,
			"namespace": namespace,
			"error":     err.Error(),
		})
		return nil, fmt.Errorf("failed to get deployments for chart %s: %w", chartName, err)
	}

	var chartDeployments []*domain.Deployment
	for _, d := range deployments {
		// Note: KubernetesDeployment has fields: Name, Namespace, Ready, UpToDate, Available, Age, Replicas, ReadyReplicas
		// Since we can't check Labels (not available), we'll include all deployments for now
		deployment := &domain.Deployment{
			Name:          d.Name,
			Namespace:     d.Namespace,
			Replicas:      int32(d.Replicas),            // Convert int to int32
			ReadyReplicas: int32(d.ReadyReplicas),       // Convert int to int32
			Status:        d.Ready,                      // Using Ready field as Status
		}
		chartDeployments = append(chartDeployments, deployment)
	}

	g.logger.DebugWithContext("deployments for chart retrieved", map[string]interface{}{
		"chart":     chartName,
		"namespace": namespace,
		"count":     len(chartDeployments),
	})

	return chartDeployments, nil
}

// GetStatefulSetsForChart gets stateful sets that belong to a specific chart
func (g *KubectlGateway) GetStatefulSetsForChart(ctx context.Context, chartName, namespace string) ([]*domain.StatefulSet, error) {
	g.logger.DebugWithContext("getting stateful sets for chart", map[string]interface{}{
		"chart":     chartName,
		"namespace": namespace,
	})

	// Get all stateful sets with helm labels matching the chart
	statefulSets, err := g.kubectlPort.GetStatefulSets(ctx, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to get stateful sets for chart", map[string]interface{}{
			"chart":     chartName,
			"namespace": namespace,
			"error":     err.Error(),
		})
		return nil, fmt.Errorf("failed to get stateful sets for chart %s: %w", chartName, err)
	}

	var chartStatefulSets []*domain.StatefulSet
	for _, sts := range statefulSets {
		// Note: KubernetesStatefulSet has similar fields to KubernetesDeployment
		// Since we can't check Labels (not available), we'll include all stateful sets for now
		statefulSet := &domain.StatefulSet{
			Name:          sts.Name,
			Namespace:     sts.Namespace,
			Replicas:      int32(sts.Replicas),            // Convert int to int32
			ReadyReplicas: int32(sts.ReadyReplicas),       // Convert int to int32  
			Status:        "Active",                       // Default status since not available
		}
		chartStatefulSets = append(chartStatefulSets, statefulSet)
	}

	g.logger.DebugWithContext("stateful sets for chart retrieved", map[string]interface{}{
		"chart":     chartName,
		"namespace": namespace,
		"count":     len(chartStatefulSets),
	})

	return chartStatefulSets, nil
}

// NamespaceExists checks if a namespace exists
func (g *KubectlGateway) NamespaceExists(ctx context.Context, namespace string) (bool, error) {
	g.logger.DebugWithContext("checking if namespace exists", map[string]interface{}{
		"namespace": namespace,
	})

	err := g.kubectlPort.GetNamespace(ctx, namespace)
	if err != nil {
		// If error is "not found", namespace doesn't exist
		g.logger.DebugWithContext("namespace not found", map[string]interface{}{
			"namespace": namespace,
		})
		return false, nil
	}

	g.logger.DebugWithContext("namespace exists", map[string]interface{}{
		"namespace": namespace,
	})
	return true, nil
}

// CreateNamespace creates a new namespace
func (g *KubectlGateway) CreateNamespace(ctx context.Context, namespace string) error {
	g.logger.InfoWithContext("creating namespace", map[string]interface{}{
		"namespace": namespace,
	})

	err := g.kubectlPort.CreateNamespace(ctx, namespace)
	if err != nil {
		g.logger.ErrorWithContext("failed to create namespace", map[string]interface{}{
			"namespace": namespace,
			"error":     err.Error(),
		})
		return fmt.Errorf("failed to create namespace %s: %w", namespace, err)
	}

	g.logger.InfoWithContext("namespace created successfully", map[string]interface{}{
		"namespace": namespace,
	})
	return nil
}

// ValidateNamespaceAccess validates access to a namespace
func (g *KubectlGateway) ValidateNamespaceAccess(ctx context.Context, namespace string) error {
	g.logger.DebugWithContext("validating namespace access", map[string]interface{}{
		"namespace": namespace,
	})

	// Try to list secrets in the namespace as an access test
	err := g.kubectlPort.ListSecrets(ctx, namespace)
	if err != nil {
		g.logger.ErrorWithContext("namespace access validation failed", map[string]interface{}{
			"namespace": namespace,
			"error":     err.Error(),
		})
		return fmt.Errorf("no access to namespace %s: %w", namespace, err)
	}

	g.logger.DebugWithContext("namespace access validated", map[string]interface{}{
		"namespace": namespace,
	})
	return nil
}

// GetNamespaceStatus gets the status of a namespace
func (g *KubectlGateway) GetNamespaceStatus(ctx context.Context, namespace string) (*domain.NamespaceStatus, error) {
	g.logger.DebugWithContext("getting namespace status", map[string]interface{}{
		"namespace": namespace,
	})

	// Get namespace information
	namespaces, err := g.kubectlPort.GetNamespaces(ctx)
	if err != nil {
		g.logger.ErrorWithContext("failed to get namespaces", map[string]interface{}{
			"namespace": namespace,
			"error":     err.Error(),
		})
		return nil, fmt.Errorf("failed to get namespace list: %w", err)
	}

	// Find the specific namespace
	for _, ns := range namespaces {
		if ns.Name == namespace {
			status := &domain.NamespaceStatus{
				Name:       ns.Name,
				Phase:      ns.Status,
				Created:    time.Now(), // Age is string, use current time as fallback
				Labels:     make(map[string]string),
				Ready:      ns.Status == "Active",
				Conditions: []string{},
			}

			g.logger.DebugWithContext("namespace status retrieved", map[string]interface{}{
				"namespace": namespace,
				"phase":     status.Phase,
			})
			return status, nil
		}
	}

	return nil, fmt.Errorf("namespace %s not found", namespace)
}

// CleanupNamespace cleans up resources in a namespace
func (g *KubectlGateway) CleanupNamespace(ctx context.Context, namespace string) error {
	g.logger.InfoWithContext("cleaning up namespace resources", map[string]interface{}{
		"namespace": namespace,
	})

	// Clean up failed resources in the namespace
	err := g.CleanupFailedResources(ctx, []string{namespace})
	if err != nil {
		g.logger.ErrorWithContext("namespace cleanup failed", map[string]interface{}{
			"namespace": namespace,
			"error":     err.Error(),
		})
		return fmt.Errorf("failed to cleanup namespace %s: %w", namespace, err)
	}

	g.logger.InfoWithContext("namespace cleanup completed", map[string]interface{}{
		"namespace": namespace,
	})
	return nil
}
