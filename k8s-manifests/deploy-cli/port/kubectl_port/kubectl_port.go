package kubectl_port

import (
	"context"
	"time"
)

// KubectlPort defines the interface for Kubernetes operations
type KubectlPort interface {
	// GetNodes returns cluster nodes
	GetNodes(ctx context.Context) ([]KubernetesNode, error)
	
	// GetPods returns pods in the specified namespace
	GetPods(ctx context.Context, namespace string, fieldSelector string) ([]KubernetesPod, error)
	
	// GetNamespaces returns all namespaces
	GetNamespaces(ctx context.Context) ([]KubernetesNamespace, error)
	
	// GetNamespace returns a specific namespace
	GetNamespace(ctx context.Context, name string) error
	
	// CreateNamespace creates a new namespace
	CreateNamespace(ctx context.Context, name string) error
	
	// DeleteNamespace deletes a namespace
	DeleteNamespace(ctx context.Context, name string) error
	
	// GetSecrets returns secrets in the specified namespace
	GetSecrets(ctx context.Context, namespace string) ([]KubernetesSecret, error)
	
	// ListSecrets lists secrets in the specified namespace (for access testing)
	ListSecrets(ctx context.Context, namespace string) error
	
	// GetSecretsWithMetadata returns secrets with helm metadata across all namespaces
	GetSecretsWithMetadata(ctx context.Context) ([]KubernetesSecretWithMetadata, error)
	
	// GetResourcesWithMetadata returns any resource type with Helm metadata across all namespaces
	GetResourcesWithMetadata(ctx context.Context, resourceType string) ([]KubernetesResourceWithMetadata, error)
	
	// DeleteResource deletes any resource type
	DeleteResource(ctx context.Context, resourceType, name, namespace string) error
	
	// CreateSecret creates a new secret
	CreateSecret(ctx context.Context, secret KubernetesSecret) error
	
	// UpdateSecret updates an existing secret
	UpdateSecret(ctx context.Context, secret KubernetesSecret) error
	
	// GetSecret returns a specific secret
	GetSecret(ctx context.Context, name, namespace string) (*KubernetesSecret, error)
	
	// ApplySecret applies or updates a secret
	ApplySecret(ctx context.Context, secret *KubernetesSecret) error
	
	// DeleteSecret deletes a secret
	DeleteSecret(ctx context.Context, name, namespace string) error
	
	// GetPersistentVolumes returns persistent volumes
	GetPersistentVolumes(ctx context.Context) ([]KubernetesPersistentVolume, error)
	
	// CreatePersistentVolume creates a new persistent volume
	CreatePersistentVolume(ctx context.Context, pv KubernetesPersistentVolume) error
	
	// DeletePersistentVolume deletes a persistent volume
	DeletePersistentVolume(ctx context.Context, name string) error
	
	// GetPersistentVolumeClaims returns persistent volume claims
	GetPersistentVolumeClaims(ctx context.Context, namespace string) ([]KubernetesPersistentVolumeClaim, error)
	
	// GetStorageClasses returns storage classes
	GetStorageClasses(ctx context.Context) ([]KubernetesStorageClass, error)
	
	// CreateStorageClass creates a new storage class
	CreateStorageClass(ctx context.Context, sc KubernetesStorageClass) error
	
	// GetStatefulSets returns stateful sets in the specified namespace
	GetStatefulSets(ctx context.Context, namespace string) ([]KubernetesStatefulSet, error)
	
	// DeleteStatefulSet deletes a stateful set
	DeleteStatefulSet(ctx context.Context, name, namespace string, force bool) error
	
	// GetDeployments returns deployments in the specified namespace
	GetDeployments(ctx context.Context, namespace string) ([]KubernetesDeployment, error)
	
	// RolloutRestart restarts a deployment
	RolloutRestart(ctx context.Context, resourceType, name, namespace string) error
	
	// RolloutStatus returns the rollout status
	RolloutStatus(ctx context.Context, resourceType, name, namespace string, timeout time.Duration) error
	
	// WaitForRollout waits for a rollout to complete
	WaitForRollout(ctx context.Context, resourceType, name, namespace string, timeout time.Duration) error
	
	// ApplyYAML applies a YAML configuration
	ApplyYAML(ctx context.Context, yamlContent string) error
	
	// ApplyFile applies a YAML file
	ApplyFile(ctx context.Context, filename string) error
	
	// Version returns kubectl version
	Version(ctx context.Context) (string, error)
}

// KubernetesNode represents a Kubernetes node
type KubernetesNode struct {
	Name   string
	Status string
	Role   string
}

// KubernetesPod represents a Kubernetes pod
type KubernetesPod struct {
	Name      string
	Namespace string
	Status    string
	Ready     string
	Restarts  int
	Age       string
}

// KubernetesNamespace represents a Kubernetes namespace
type KubernetesNamespace struct {
	Name   string
	Status string
	Age    string
}

// KubernetesSecret represents a Kubernetes secret
type KubernetesSecret struct {
	Name        string
	Namespace   string
	Type        string
	Data        map[string][]byte
	Labels      map[string]string
	Annotations map[string]string
}

// KubernetesSecretWithMetadata represents a Kubernetes secret with Helm metadata
type KubernetesSecretWithMetadata struct {
	Name             string
	Namespace        string
	Type             string
	Data             map[string][]byte
	ReleaseName      string
	ReleaseNamespace string
	Age              string
}

// KubernetesResourceWithMetadata represents any Kubernetes resource with Helm metadata
type KubernetesResourceWithMetadata struct {
	ResourceType     string
	Name             string
	Namespace        string
	ReleaseName      string
	ReleaseNamespace string
	Age              string
}

// KubernetesPersistentVolume represents a Kubernetes persistent volume
type KubernetesPersistentVolume struct {
	Name         string
	Capacity     string
	AccessModes  []string
	Status       string
	StorageClass string
	HostPath     string
}

// KubernetesPersistentVolumeClaim represents a Kubernetes persistent volume claim
type KubernetesPersistentVolumeClaim struct {
	Name      string
	Namespace string
	Status    string
	Volume    string
	Capacity  string
	AccessModes []string
}

// KubernetesStorageClass represents a Kubernetes storage class
type KubernetesStorageClass struct {
	Name        string
	Provisioner string
	Parameters  map[string]string
}

// KubernetesStatefulSet represents a Kubernetes stateful set
type KubernetesStatefulSet struct {
	Name           string
	Namespace      string
	Ready          string
	Age            string
	Replicas       int
	ReadyReplicas  int
}

// KubernetesDeployment represents a Kubernetes deployment
type KubernetesDeployment struct {
	Name          string
	Namespace     string
	Ready         string
	UpToDate      string
	Available     string
	Age           string
	Replicas      int
	ReadyReplicas int
}