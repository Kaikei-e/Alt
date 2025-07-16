package kubectl_driver

import (
	"context"
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

// CreateSecret creates a new secret
func (k *KubectlDriver) CreateSecret(ctx context.Context, secret kubectl_port.KubernetesSecret) error {
	args := []string{"create", "secret", "generic", secret.Name, "--namespace", secret.Namespace}
	
	for key, value := range secret.Data {
		args = append(args, "--from-literal", fmt.Sprintf("%s=%s", key, string(value)))
	}
	
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "already exists") {
		return fmt.Errorf("kubectl create secret failed: %w, output: %s", err, string(output))
	}
	return nil
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

// DeleteSecret deletes a secret
func (k *KubectlDriver) DeleteSecret(ctx context.Context, name, namespace string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "delete", "secret", name, "--namespace", namespace, "--force", "--grace-period=0")
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "not found") {
		return fmt.Errorf("kubectl delete secret failed: %w, output: %s", err, string(output))
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
	// This would typically create a YAML manifest and apply it
	// For now, we'll implement basic creation
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
	
	// Build YAML manifest
	yaml := fmt.Sprintf(`apiVersion: v1
kind: PersistentVolume
metadata:
  name: %s
spec:
  capacity:
    storage: %s
  accessModes:
    - ReadWriteOnce
  storageClassName: %s
  hostPath:
    path: %s
`, pv.Name, pv.Capacity, pv.StorageClass, pv.HostPath)
	
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