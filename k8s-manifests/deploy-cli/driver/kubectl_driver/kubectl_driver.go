package kubectl_driver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"deploy-cli/port/kubectl_port"
)

// KubectlDriver implements Kubernetes operations using kubectl CLI
type KubectlDriver struct{}

// Ensure KubectlDriver implements KubectlPort interface
var _ kubectl_port.KubectlPort = (*KubectlDriver)(nil)

// NewKubectlDriver creates a new kubectl driver
func NewKubectlDriver() *KubectlDriver {
	return &KubectlDriver{}
}

// GetNodes returns cluster nodes
func (k *KubectlDriver) GetNodes(ctx context.Context) ([]kubectl_port.KubernetesNode, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "nodes", "--no-headers", "-o", "custom-columns=NAME:.metadata.name,STATUS:.status.conditions[-1].type,ROLE:.metadata.labels.kubernetes\\.io/role")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl get nodes failed: %w, output: %s", err, string(output))
	}

	var nodes []kubectl_port.KubernetesNode
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			node := kubectl_port.KubernetesNode{
				Name:   fields[0],
				Status: fields[1],
			}
			if len(fields) >= 3 {
				node.Role = fields[2]
			}
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

// GetPods returns pods in the specified namespace
func (k *KubectlDriver) GetPods(ctx context.Context, namespace string, selector string) ([]kubectl_port.KubernetesPod, error) {
	args := []string{"get", "pods", "--no-headers", "-o", "custom-columns=NAME:.metadata.name,NAMESPACE:.metadata.namespace,STATUS:.status.phase,READY:.status.conditions[-1].status,RESTARTS:.status.containerStatuses[0].restartCount,AGE:.metadata.creationTimestamp"}

	if namespace != "" {
		args = append(args, "--namespace", namespace)
	} else {
		args = append(args, "--all-namespaces")
	}

	if selector != "" {
		// Check if it's a field selector or label selector
		if strings.Contains(selector, "status.phase") || strings.Contains(selector, "metadata.") {
			args = append(args, "--field-selector", selector)
		} else {
			args = append(args, "--selector", selector)
		}
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl get pods failed: %w, output: %s", err, string(output))
	}

	var pods []kubectl_port.KubernetesPod
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 6 {
			restarts, _ := strconv.Atoi(fields[4])
			pod := kubectl_port.KubernetesPod{
				Name:      fields[0],
				Namespace: fields[1],
				Status:    fields[2],
				Ready:     fields[3],
				Restarts:  restarts,
				Age:       fields[5],
			}
			pods = append(pods, pod)
		}
	}

	return pods, nil
}

// GetNamespaces returns all namespaces
func (k *KubectlDriver) GetNamespaces(ctx context.Context) ([]kubectl_port.KubernetesNamespace, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "namespaces", "--no-headers", "-o", "custom-columns=NAME:.metadata.name,STATUS:.status.phase,AGE:.metadata.creationTimestamp")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl get namespaces failed: %w, output: %s", err, string(output))
	}

	var namespaces []kubectl_port.KubernetesNamespace
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			namespace := kubectl_port.KubernetesNamespace{
				Name:   fields[0],
				Status: fields[1],
				Age:    fields[2],
			}
			namespaces = append(namespaces, namespace)
		}
	}

	return namespaces, nil
}

// CreateNamespace creates a new namespace
func (k *KubectlDriver) CreateNamespace(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "create", "namespace", name)
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "already exists") {
		return fmt.Errorf("kubectl create namespace failed: %w, output: %s", err, string(output))
	}
	return nil
}

// DeleteNamespace deletes a namespace
func (k *KubectlDriver) DeleteNamespace(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "delete", "namespace", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl delete namespace failed: %w, output: %s", err, string(output))
	}
	return nil
}

// GetSecrets returns secrets in the specified namespace
func (k *KubectlDriver) GetSecrets(ctx context.Context, namespace string) ([]kubectl_port.KubernetesSecret, error) {
	args := []string{"get", "secrets", "--no-headers", "-o", "custom-columns=NAME:.metadata.name,TYPE:.type"}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl get secrets failed: %w, output: %s", err, string(output))
	}

	var secrets []kubectl_port.KubernetesSecret
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			secret := kubectl_port.KubernetesSecret{
				Name:      fields[0],
				Namespace: namespace,
				Type:      fields[1],
				Data:      make(map[string][]byte),
			}
			secrets = append(secrets, secret)
		}
	}

	return secrets, nil
}

// GetSecretsWithMetadata returns secrets with helm metadata across all namespaces
func (k *KubectlDriver) GetSecretsWithMetadata(ctx context.Context) ([]kubectl_port.KubernetesSecretWithMetadata, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "secrets", "--all-namespaces", "--no-headers", "-o", "custom-columns=NAMESPACE:.metadata.namespace,NAME:.metadata.name,TYPE:.type,RELEASE:.metadata.annotations.meta\\.helm\\.sh/release-name,RELEASE_NS:.metadata.annotations.meta\\.helm\\.sh/release-namespace,AGE:.metadata.creationTimestamp")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl get secrets with metadata failed: %w, output: %s", err, string(output))
	}

	var secrets []kubectl_port.KubernetesSecretWithMetadata
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			secret := kubectl_port.KubernetesSecretWithMetadata{
				Namespace: fields[0],
				Name:      fields[1],
				Type:      fields[2],
				Data:      make(map[string][]byte),
			}

			// Parse optional fields
			if len(fields) >= 4 && fields[3] != "<none>" {
				secret.ReleaseName = fields[3]
			}
			if len(fields) >= 5 && fields[4] != "<none>" {
				secret.ReleaseNamespace = fields[4]
			}
			if len(fields) >= 6 && fields[5] != "<none>" {
				secret.Age = fields[5]
			}

			secrets = append(secrets, secret)
		}
	}

	return secrets, nil
}

// CreateSecret creates a new secret using ApplySecret to ensure labels and annotations are properly set
func (k *KubectlDriver) CreateSecret(ctx context.Context, secret kubectl_port.KubernetesSecret) error {
	// Use ApplySecret which properly handles labels and annotations
	// This ensures compatibility with secret validation requirements
	return k.ApplySecret(ctx, &secret)
}

// UpdateSecret updates an existing secret
func (k *KubectlDriver) UpdateSecret(ctx context.Context, secret kubectl_port.KubernetesSecret) error {
	// First try to create, if it fails, patch it
	createErr := k.CreateSecret(ctx, secret)
	if createErr != nil && strings.Contains(createErr.Error(), "already exists") {
		// Build patch data
		patchData := make(map[string]string)
		for key, value := range secret.Data {
			patchData[key] = string(value)
		}

		patchJSON, err := json.Marshal(map[string]interface{}{
			"data": patchData,
		})
		if err != nil {
			return fmt.Errorf("failed to marshal patch data: %w", err)
		}

		cmd := exec.CommandContext(ctx, "kubectl", "patch", "secret", secret.Name, "-n", secret.Namespace, "-p", string(patchJSON))
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("kubectl patch secret failed: %w, output: %s", err, string(output))
		}
	}
	return createErr
}

// GetSecret returns a specific secret
func (k *KubectlDriver) GetSecret(ctx context.Context, name, namespace string) (*kubectl_port.KubernetesSecret, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "secret", name, "--namespace", namespace, "-o", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl get secret failed: %w, output: %s", err, string(output))
	}

	var secretData struct {
		Metadata struct {
			Name        string            `json:"name"`
			Namespace   string            `json:"namespace"`
			Labels      map[string]string `json:"labels"`
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
		Type string            `json:"type"`
		Data map[string]string `json:"data"`
	}

	if err := json.Unmarshal(output, &secretData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal secret: %w", err)
	}

	// Convert data from base64 strings to bytes
	data := make(map[string][]byte)
	for key, value := range secretData.Data {
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, fmt.Errorf("failed to decode secret data for key %s: %w", key, err)
		}
		data[key] = decoded
	}

	return &kubectl_port.KubernetesSecret{
		Name:        secretData.Metadata.Name,
		Namespace:   secretData.Metadata.Namespace,
		Type:        secretData.Type,
		Data:        data,
		Labels:      secretData.Metadata.Labels,
		Annotations: secretData.Metadata.Annotations,
	}, nil
}

// ApplySecret applies or updates a secret with proper handling of immutable type field
func (k *KubectlDriver) ApplySecret(ctx context.Context, secret *kubectl_port.KubernetesSecret) error {
	// Check if secret already exists
	existingSecret, err := k.GetSecret(ctx, secret.Name, secret.Namespace)
	if err != nil {
		// Secret doesn't exist, create new one
		return k.createNewSecret(ctx, secret)
	}

	// Secret exists, check if type needs to be changed
	if existingSecret.Type != secret.Type {
		// Type is immutable, need to delete and recreate
		if err := k.deleteAndRecreateSecret(ctx, secret); err != nil {
			return fmt.Errorf("failed to recreate secret with new type: %w", err)
		}
		return nil
	}

	// Type is same, can patch data and metadata
	return k.patchSecretData(ctx, secret)
}

// createNewSecret creates a new secret using kubectl apply
func (k *KubectlDriver) createNewSecret(ctx context.Context, secret *kubectl_port.KubernetesSecret) error {
	manifestJSON, err := k.buildSecretManifest(secret)
	if err != nil {
		return err
	}

	// Apply using kubectl
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(string(manifestJSON))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply secret failed: %w, output: %s", err, string(output))
	}

	return nil
}

// deleteAndRecreateSecret deletes existing secret and creates new one with different type
func (k *KubectlDriver) deleteAndRecreateSecret(ctx context.Context, secret *kubectl_port.KubernetesSecret) error {
	// Delete existing secret
	deleteCmd := exec.CommandContext(ctx, "kubectl", "delete", "secret", secret.Name, "-n", secret.Namespace, "--ignore-not-found=true")
	if output, err := deleteCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete existing secret: %w, output: %s", err, string(output))
	}

	// Wait a moment for deletion to complete
	time.Sleep(2 * time.Second)

	// Create new secret
	return k.createNewSecret(ctx, secret)
}

// patchSecretData patches only the data of existing secret (type unchanged)
func (k *KubectlDriver) patchSecretData(ctx context.Context, secret *kubectl_port.KubernetesSecret) error {
	// Convert secret data to base64 encoded strings
	data := make(map[string]string)
	for key, value := range secret.Data {
		encoded := base64.StdEncoding.EncodeToString(value)
		data[key] = encoded
	}

	// Create patch for data and metadata only
	patchData := map[string]interface{}{
		"data": data,
	}

	// Add metadata updates if needed
	metadata := make(map[string]interface{})
	if len(secret.Labels) > 0 {
		metadata["labels"] = secret.Labels
	}
	if len(secret.Annotations) > 0 {
		metadata["annotations"] = secret.Annotations
	}
	if len(metadata) > 0 {
		patchData["metadata"] = metadata
	}

	patchJSON, err := json.Marshal(patchData)
	if err != nil {
		return fmt.Errorf("failed to marshal patch data: %w", err)
	}

	// Apply patch using kubectl
	cmd := exec.CommandContext(ctx, "kubectl", "patch", "secret", secret.Name, "-n", secret.Namespace, "--type=strategic", "-p", string(patchJSON))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl patch secret failed: %w, output: %s", err, string(output))
	}

	return nil
}

// buildSecretManifest builds the complete secret manifest
func (k *KubectlDriver) buildSecretManifest(secret *kubectl_port.KubernetesSecret) ([]byte, error) {
	// Convert secret data to base64 encoded strings
	data := make(map[string]string)
	for key, value := range secret.Data {
		encoded := base64.StdEncoding.EncodeToString(value)
		data[key] = encoded
	}

	// Create the secret manifest
	secretManifest := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      secret.Name,
			"namespace": secret.Namespace,
		},
		"type": secret.Type,
		"data": data,
	}

	// Add labels if they exist
	if len(secret.Labels) > 0 {
		metadata := secretManifest["metadata"].(map[string]interface{})
		metadata["labels"] = secret.Labels
	}

	// Add annotations if they exist
	if len(secret.Annotations) > 0 {
		metadata := secretManifest["metadata"].(map[string]interface{})
		metadata["annotations"] = secret.Annotations
	}

	// Convert to JSON
	return json.Marshal(secretManifest)
}

// DeleteSecret deletes a secret
func (k *KubectlDriver) DeleteSecret(ctx context.Context, name, namespace string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "delete", "secret", name, "--namespace", namespace, "--force", "--grace-period=0")
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "not found") {
		return fmt.Errorf("kubectl delete secret failed: %w, output: %s", err, string(output))
	}
	return nil
}

// GetResourcesWithMetadata returns any resource type with Helm metadata across all namespaces
func (k *KubectlDriver) GetResourcesWithMetadata(ctx context.Context, resourceType string) ([]kubectl_port.KubernetesResourceWithMetadata, error) {
	var resources []kubectl_port.KubernetesResourceWithMetadata

	// Build kubectl command for the specific resource type
	cmd := exec.CommandContext(ctx, "kubectl", "get", resourceType, "--all-namespaces", "--no-headers", "-o", "custom-columns=NAMESPACE:.metadata.namespace,NAME:.metadata.name,RELEASE:.metadata.annotations.meta\\.helm\\.sh/release-name,RELEASE_NS:.metadata.annotations.meta\\.helm\\.sh/release-namespace,AGE:.metadata.creationTimestamp")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl get %s with metadata failed: %w, output: %s", resourceType, err, string(output))
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 5 {
			namespace := fields[0]
			name := fields[1]
			releaseName := fields[2]
			releaseNamespace := fields[3]
			age := fields[4]

			// Skip resources without Helm metadata
			if releaseName == "<none>" || releaseNamespace == "<none>" {
				continue
			}

			resource := kubectl_port.KubernetesResourceWithMetadata{
				ResourceType:     resourceType,
				Name:             name,
				Namespace:        namespace,
				ReleaseName:      releaseName,
				ReleaseNamespace: releaseNamespace,
				Age:              age,
			}

			resources = append(resources, resource)
		}
	}

	return resources, nil
}

// DeleteResource deletes any resource type
func (k *KubectlDriver) DeleteResource(ctx context.Context, resourceType, name, namespace string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "delete", resourceType, name, "--namespace", namespace, "--force", "--grace-period=0")
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "not found") {
		return fmt.Errorf("kubectl delete %s failed: %w, output: %s", resourceType, err, string(output))
	}
	return nil
}

// GetPersistentVolumes returns persistent volumes
func (k *KubectlDriver) GetPersistentVolumes(ctx context.Context) ([]kubectl_port.KubernetesPersistentVolume, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "pv", "--no-headers", "-o", "custom-columns=NAME:.metadata.name,CAPACITY:.spec.capacity.storage,ACCESS:.spec.accessModes[0],STATUS:.status.phase,STORAGECLASS:.spec.storageClassName")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl get pv failed: %w, output: %s", err, string(output))
	}

	var pvs []kubectl_port.KubernetesPersistentVolume
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			pv := kubectl_port.KubernetesPersistentVolume{
				Name:         fields[0],
				Capacity:     fields[1],
				AccessModes:  []string{fields[2]},
				Status:       fields[3],
				StorageClass: fields[4],
			}
			pvs = append(pvs, pv)
		}
	}

	return pvs, nil
}

// CreatePersistentVolume creates a new persistent volume
func (k *KubectlDriver) CreatePersistentVolume(ctx context.Context, pv kubectl_port.KubernetesPersistentVolume) error {
	// Check if PV already exists
	existingPVs, err := k.GetPersistentVolumes(ctx)
	if err != nil {
		return fmt.Errorf("failed to check existing PVs: %w", err)
	}

	for _, existingPV := range existingPVs {
		if existingPV.Name == pv.Name {
			// PV already exists, check if it needs to be updated
			if existingPV.Capacity != pv.Capacity || existingPV.StorageClass != pv.StorageClass {
				// Delete and recreate with correct configuration
				if err := k.DeletePersistentVolume(ctx, pv.Name); err != nil {
					return fmt.Errorf("failed to delete existing PV for recreation: %w", err)
				}
				// Wait a moment for deletion to complete
				time.Sleep(2 * time.Second)
			} else {
				// PV exists with correct configuration, skip creation
				return nil
			}
		}
	}

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")

	// Build YAML manifest using local storage instead of hostPath for local-storage class
	var storageSpec string
	if pv.StorageClass == "local-storage" {
		// Use local storage specification with node affinity
		storageSpec = fmt.Sprintf(`  local:
    path: %s
  nodeAffinity:
    required:
      nodeSelectorTerms:
      - matchExpressions:
        - key: kubernetes.io/hostname
          operator: In
          values: ["koko-b"]`, pv.HostPath)
	} else {
		// Use hostPath for other storage classes
		storageSpec = fmt.Sprintf(`  hostPath:
    path: %s`, pv.HostPath)
	}

	yaml := fmt.Sprintf(`apiVersion: v1
kind: PersistentVolume
metadata:
  name: %s
  labels:
    app.kubernetes.io/name: alt
    app.kubernetes.io/version: v1.0.0
spec:
  capacity:
    storage: %s
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  storageClassName: %s
%s
`, pv.Name, pv.Capacity, pv.StorageClass, storageSpec)

	cmd.Stdin = strings.NewReader(yaml)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl create pv failed: %w, output: %s", err, string(output))
	}
	return nil
}

// DeletePersistentVolume deletes a persistent volume
func (k *KubectlDriver) DeletePersistentVolume(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "delete", "pv", name)
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "not found") {
		return fmt.Errorf("kubectl delete pv failed: %w, output: %s", err, string(output))
	}
	return nil
}

// GetPersistentVolumeClaims returns persistent volume claims
func (k *KubectlDriver) GetPersistentVolumeClaims(ctx context.Context, namespace string) ([]kubectl_port.KubernetesPersistentVolumeClaim, error) {
	args := []string{"get", "pvc", "--no-headers", "-o", "custom-columns=NAME:.metadata.name,STATUS:.status.phase,VOLUME:.spec.volumeName,CAPACITY:.spec.resources.requests.storage"}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	} else {
		args = append(args, "--all-namespaces")
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl get pvc failed: %w, output: %s", err, string(output))
	}

	var pvcs []kubectl_port.KubernetesPersistentVolumeClaim
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			pvc := kubectl_port.KubernetesPersistentVolumeClaim{
				Name:      fields[0],
				Namespace: namespace,
				Status:    fields[1],
				Volume:    fields[2],
				Capacity:  fields[3],
			}
			pvcs = append(pvcs, pvc)
		}
	}

	return pvcs, nil
}

// GetStorageClasses returns storage classes
func (k *KubectlDriver) GetStorageClasses(ctx context.Context) ([]kubectl_port.KubernetesStorageClass, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "storageclass", "--no-headers", "-o", "custom-columns=NAME:.metadata.name,PROVISIONER:.provisioner")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl get storageclass failed: %w, output: %s", err, string(output))
	}

	var scs []kubectl_port.KubernetesStorageClass
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			sc := kubectl_port.KubernetesStorageClass{
				Name:        fields[0],
				Provisioner: fields[1],
				Parameters:  make(map[string]string),
			}
			scs = append(scs, sc)
		}
	}

	return scs, nil
}

// CreateStorageClass creates a new storage class
func (k *KubectlDriver) CreateStorageClass(ctx context.Context, sc kubectl_port.KubernetesStorageClass) error {
	yaml := fmt.Sprintf(`apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: %s
provisioner: %s
`, sc.Name, sc.Provisioner)

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(yaml)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl create storageclass failed: %w, output: %s", err, string(output))
	}
	return nil
}

// GetStatefulSets returns stateful sets in the specified namespace
func (k *KubectlDriver) GetStatefulSets(ctx context.Context, namespace string) ([]kubectl_port.KubernetesStatefulSet, error) {
	args := []string{"get", "statefulset", "--no-headers", "-o", "custom-columns=NAME:.metadata.name,READY:.status.readyReplicas,AGE:.metadata.creationTimestamp"}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl get statefulset failed: %w, output: %s", err, string(output))
	}

	var sts []kubectl_port.KubernetesStatefulSet
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			ss := kubectl_port.KubernetesStatefulSet{
				Name:      fields[0],
				Namespace: namespace,
				Ready:     fields[1],
				Age:       fields[2],
			}
			sts = append(sts, ss)
		}
	}

	return sts, nil
}

// DeleteStatefulSet deletes a stateful set
func (k *KubectlDriver) DeleteStatefulSet(ctx context.Context, name, namespace string, force bool) error {
	args := []string{"delete", "statefulset", name, "--namespace", namespace}
	if force {
		args = append(args, "--force", "--grace-period=0")
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "not found") {
		return fmt.Errorf("kubectl delete statefulset failed: %w, output: %s", err, string(output))
	}
	return nil
}

// GetDeployments returns deployments in the specified namespace
func (k *KubectlDriver) GetDeployments(ctx context.Context, namespace string) ([]kubectl_port.KubernetesDeployment, error) {
	args := []string{"get", "deployment", "--no-headers", "-o", "custom-columns=NAME:.metadata.name,READY:.status.readyReplicas,UP-TO-DATE:.status.updatedReplicas,AVAILABLE:.status.availableReplicas,AGE:.metadata.creationTimestamp"}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl get deployment failed: %w, output: %s", err, string(output))
	}

	var deployments []kubectl_port.KubernetesDeployment
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			deployment := kubectl_port.KubernetesDeployment{
				Name:      fields[0],
				Namespace: namespace,
				Ready:     fields[1],
				UpToDate:  fields[2],
				Available: fields[3],
				Age:       fields[4],
			}
			deployments = append(deployments, deployment)
		}
	}

	return deployments, nil
}

// RolloutRestart restarts a deployment
func (k *KubectlDriver) RolloutRestart(ctx context.Context, resourceType, name, namespace string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "rollout", "restart", fmt.Sprintf("%s/%s", resourceType, name), "--namespace", namespace)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl rollout restart failed: %w, output: %s", err, string(output))
	}
	return nil
}

// RolloutStatus returns the rollout status
func (k *KubectlDriver) RolloutStatus(ctx context.Context, resourceType, name, namespace string, timeout time.Duration) error {
	args := []string{"rollout", "status", fmt.Sprintf("%s/%s", resourceType, name), "--namespace", namespace}
	if timeout > 0 {
		args = append(args, "--timeout", timeout.String())
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl rollout status failed: %w, output: %s", err, string(output))
	}
	return nil
}

// WaitForRollout waits for a rollout to complete
func (k *KubectlDriver) WaitForRollout(ctx context.Context, resourceType, name, namespace string, timeout time.Duration) error {
	// Create a timeout context for the rollout wait
	timeoutCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		timeoutCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	args := []string{"rollout", "status", fmt.Sprintf("%s/%s", resourceType, name), "--namespace", namespace, "--watch=false"}
	if timeout > 0 {
		args = append(args, "--timeout", timeout.String())
	}

	cmd := exec.CommandContext(timeoutCtx, "kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Detailed error classification
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return &RolloutTimeoutError{
				ResourceType: resourceType,
				Name:         name,
				Namespace:    namespace,
				Timeout:      timeout,
				Output:       string(output),
			}
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			return &KubectlCommandError{
				Command:  "rollout status",
				Args:     args,
				ExitCode: exitErr.ExitCode(),
				Output:   string(output),
				Cause:    err,
			}
		}

		return fmt.Errorf("kubectl rollout status wait failed: %w, output: %s", err, string(output))
	}
	return nil
}

// Custom error types for better error handling

// RolloutTimeoutError represents a rollout timeout error
type RolloutTimeoutError struct {
	ResourceType string
	Name         string
	Namespace    string
	Timeout      time.Duration
	Output       string
}

func (e *RolloutTimeoutError) Error() string {
	return fmt.Sprintf("rollout wait timed out after %v for %s/%s in namespace %s",
		e.Timeout, e.ResourceType, e.Name, e.Namespace)
}

func (e *RolloutTimeoutError) IsRetriable() bool {
	return true
}

// KubectlCommandError represents a kubectl command execution error
type KubectlCommandError struct {
	Command  string
	Args     []string
	ExitCode int
	Output   string
	Cause    error
}

func (e *KubectlCommandError) Error() string {
	return fmt.Sprintf("kubectl %s failed with exit code %d: %s",
		e.Command, e.ExitCode, e.Output)
}

func (e *KubectlCommandError) IsRetriable() bool {
	// Certain exit codes are retriable
	retriableExitCodes := []int{1, 130} // General errors, interrupted by signal
	for _, code := range retriableExitCodes {
		if e.ExitCode == code {
			return true
		}
	}
	return false
}

func (e *KubectlCommandError) Unwrap() error {
	return e.Cause
}

// ApplyYAML applies a YAML configuration
func (k *KubectlDriver) ApplyYAML(ctx context.Context, yamlContent string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(yamlContent)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply failed: %w, output: %s", err, string(output))
	}
	return nil
}

// ApplyFile applies a YAML file
func (k *KubectlDriver) ApplyFile(ctx context.Context, filename string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", filename)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply file failed: %w, output: %s", err, string(output))
	}
	return nil
}

// Version returns kubectl version
func (k *KubectlDriver) Version(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "version", "--client")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("kubectl version failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// IsInstalled checks if kubectl is installed
func (k *KubectlDriver) IsInstalled() bool {
	_, err := exec.LookPath("kubectl")
	return err == nil
}

// GetNamespace returns a specific namespace
func (k *KubectlDriver) GetNamespace(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "namespace", name)
	_, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl get namespace %s failed: %w", name, err)
	}
	return nil
}

// ListSecrets lists secrets in the specified namespace (for access testing)
func (k *KubectlDriver) ListSecrets(ctx context.Context, namespace string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "secrets", "-n", namespace)
	_, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl list secrets in namespace %s failed: %w", namespace, err)
	}
	return nil
}
