// PHASE 4: Pod creation flow integration test
package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	
	v1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"
)

// TestPodCreationFlow tests the complete pod creation flow as specified in TASK.md
func TestPodCreationFlow(t *testing.T) {
	// Skip integration test if not in CI or explicit test environment
	if !isIntegrationTestEnvironment() {
		t.Skip("Skipping integration test - not in test environment")
	}

	tests := []struct {
		name      string
		chartName string
		namespace string
		timeout   time.Duration
		chartType string // "statefulset", "deployment", "service"
	}{
		{
			name:      "postgres-statefulset",
			chartName: "postgres",
			namespace: "alt-database",
			timeout:   10 * time.Minute,
			chartType: "statefulset",
		},
		{
			name:      "auth-postgres-statefulset",
			chartName: "auth-postgres", 
			namespace: "alt-database",
			timeout:   10 * time.Minute,
			chartType: "statefulset",
		},
		{
			name:      "alt-backend-deployment",
			chartName: "alt-backend",
			namespace: "alt-apps",
			timeout:   5 * time.Minute,
			chartType: "deployment",
		},
		{
			name:      "alt-frontend-deployment", 
			chartName: "alt-frontend",
			namespace: "alt-apps",
			timeout:   5 * time.Minute,
			chartType: "deployment",
		},
		{
			name:      "meilisearch-statefulset",
			chartName: "meilisearch",
			namespace: "alt-search",
			timeout:   8 * time.Minute,
			chartType: "statefulset",
		},
	}

	// Initialize Kubernetes client
	client, err := getKubernetesClient()
	if err != nil {
		t.Fatalf("Failed to initialize Kubernetes client: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout+2*time.Minute)
			defer cancel()

			t.Logf("🚀 Starting integration test for %s in namespace %s", tt.chartName, tt.namespace)

			// Step 1: Pre-test cleanup (if needed)
			if err := preTestCleanup(ctx, client, tt.chartName, tt.namespace); err != nil {
				t.Logf("⚠️  Pre-test cleanup warning: %v", err)
			}

			// Step 2: Deploy chart using deploy-cli
			t.Logf("📦 Deploying chart %s", tt.chartName)
			if err := deployChart(tt.chartName, tt.namespace); err != nil {
				t.Fatalf("❌ Chart deployment failed: %v", err)
			}

			// Step 3: Wait for pods to be ready
			t.Logf("⏳ Waiting for pods to be ready (timeout: %v)", tt.timeout)
			if err := waitForPodsReady(ctx, client, tt.namespace, tt.chartName, tt.timeout); err != nil {
				// Collect diagnostic information on failure
				collectDiagnosticInfo(client, tt.namespace, tt.chartName)
				t.Fatalf("❌ Pods not ready within timeout: %v", err)
			}

			// Step 4: Validate Helm metadata
			t.Logf("🔍 Validating Helm metadata")
			if err := validateHelmMetadata(ctx, client, tt.namespace, tt.chartName); err != nil {
				t.Errorf("❌ Helm metadata validation failed: %v", err)
			}

			// Step 5: Chart-specific validations
			switch tt.chartType {
			case "statefulset":
				if err := validateStatefulSetSpecific(ctx, client, tt.namespace, tt.chartName); err != nil {
					t.Errorf("❌ StatefulSet-specific validation failed: %v", err)
				}
			case "deployment":
				if err := validateDeploymentSpecific(ctx, client, tt.namespace, tt.chartName); err != nil {
					t.Errorf("❌ Deployment-specific validation failed: %v", err)
				}
			}

			// Step 6: Health check validation
			t.Logf("🏥 Running health checks")
			if err := validateHealthChecks(ctx, client, tt.namespace, tt.chartName); err != nil {
				t.Errorf("❌ Health check validation failed: %v", err)
			}

			// Step 7: Post-test cleanup
			if !keepTestResources() {
				t.Logf("🧹 Cleaning up test resources")
				cleanupChart(tt.chartName, tt.namespace)
			} else {
				t.Logf("🔒 Keeping test resources for manual inspection")
			}

			t.Logf("✅ Integration test completed successfully for %s", tt.chartName)
		})
	}
}

// getKubernetesClient initializes a Kubernetes client
func getKubernetesClient() (kubernetes.Interface, error) {
	// Try in-cluster config first
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig
		config, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		if err != nil {
			return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
		}
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return client, nil
}

// deployChart deploys a chart using the deploy-cli
func deployChart(chartName, namespace string) error {
	// This would integrate with the actual deploy-cli
	// For now, we simulate with helm directly
	// TODO: Integrate with actual deploy-cli after refactoring
	return fmt.Errorf("deploy-cli integration not yet implemented")
}

// waitForPodsReady waits for all pods in a chart to be ready
func waitForPodsReady(ctx context.Context, client kubernetes.Interface, namespace, chartName string, timeout time.Duration) error {
	return wait.PollImmediate(10*time.Second, timeout, func() (bool, error) {
		// Get pods with chart label
		pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", chartName),
		})
		if err != nil {
			return false, err
		}

		if len(pods.Items) == 0 {
			return false, nil // No pods found yet
		}

		readyCount := 0
		for _, pod := range pods.Items {
			if isPodReady(&pod) {
				readyCount++
			}
		}

		allReady := readyCount == len(pods.Items)
		if allReady {
			return true, nil
		}

		return false, nil // Keep waiting
	})
}

// isPodReady checks if a pod is ready
func isPodReady(pod *v1.Pod) bool {
	if pod.Status.Phase != v1.PodRunning {
		return false
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == v1.PodReady && condition.Status == v1.ConditionTrue {
			return true
		}
	}

	return false
}

// validateHelmMetadata validates that resources have proper Helm metadata
func validateHelmMetadata(ctx context.Context, client kubernetes.Interface, namespace, chartName string) error {
	// Check deployments/statefulsets
	deployments, err := client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", chartName),
	})
	if err != nil {
		return fmt.Errorf("failed to list deployments: %w", err)
	}

	for _, deployment := range deployments.Items {
		if err := validateResourceHelmMetadata(deployment.ObjectMeta); err != nil {
			return fmt.Errorf("deployment %s: %w", deployment.Name, err)
		}
	}

	statefulsets, err := client.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", chartName),
	})
	if err != nil {
		return fmt.Errorf("failed to list statefulsets: %w", err)
	}

	for _, sts := range statefulsets.Items {
		if err := validateResourceHelmMetadata(sts.ObjectMeta); err != nil {
			return fmt.Errorf("statefulset %s: %w", sts.Name, err)
		}
	}

	return nil
}

// validateResourceHelmMetadata validates Helm metadata on a resource
func validateResourceHelmMetadata(meta metav1.ObjectMeta) error {
	requiredLabels := []string{
		"app.kubernetes.io/managed-by",
	}

	requiredAnnotations := []string{
		"meta.helm.sh/release-name",
		"meta.helm.sh/release-namespace",
	}

	for _, label := range requiredLabels {
		if value, exists := meta.Labels[label]; !exists || value != "Helm" {
			return fmt.Errorf("missing or invalid label %s", label)
		}
	}

	for _, annotation := range requiredAnnotations {
		if _, exists := meta.Annotations[annotation]; !exists {
			return fmt.Errorf("missing annotation %s", annotation)
		}
	}

	return nil
}

// validateStatefulSetSpecific performs StatefulSet-specific validations
func validateStatefulSetSpecific(ctx context.Context, client kubernetes.Interface, namespace, chartName string) error {
	statefulsets, err := client.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", chartName),
	})
	if err != nil {
		return fmt.Errorf("failed to list statefulsets: %w", err)
	}

	for _, sts := range statefulsets.Items {
		// Check PVC claims
		if len(sts.Spec.VolumeClaimTemplates) > 0 {
			if err := validateStatefulSetPVCs(ctx, client, &sts); err != nil {
				return fmt.Errorf("PVC validation failed for %s: %w", sts.Name, err)
			}
		}
	}

	return nil
}

// validateStatefulSetPVCs validates PVCs for StatefulSets
func validateStatefulSetPVCs(ctx context.Context, client kubernetes.Interface, sts *appsv1.StatefulSet) error {
	for i := int32(0); i < *sts.Spec.Replicas; i++ {
		for _, vct := range sts.Spec.VolumeClaimTemplates {
			pvcName := fmt.Sprintf("%s-%s-%d", vct.Name, sts.Name, i)
			
			pvc, err := client.CoreV1().PersistentVolumeClaims(sts.Namespace).Get(ctx, pvcName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("PVC %s not found: %w", pvcName, err)
			}

			if pvc.Status.Phase != v1.ClaimBound {
				return fmt.Errorf("PVC %s not bound, status: %s", pvcName, pvc.Status.Phase)
			}
		}
	}

	return nil
}

// validateDeploymentSpecific performs Deployment-specific validations
func validateDeploymentSpecific(ctx context.Context, client kubernetes.Interface, namespace, chartName string) error {
	deployments, err := client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", chartName),
	})
	if err != nil {
		return fmt.Errorf("failed to list deployments: %w", err)
	}

	for _, deployment := range deployments.Items {
		if deployment.Status.ReadyReplicas != deployment.Status.Replicas {
			return fmt.Errorf("deployment %s: not all replicas ready (%d/%d)",
				deployment.Name, deployment.Status.ReadyReplicas, deployment.Status.Replicas)
		}
	}

	return nil
}

// validateHealthChecks performs basic health checks
func validateHealthChecks(ctx context.Context, client kubernetes.Interface, namespace, chartName string) error {
	// Check services
	services, err := client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", chartName),
	})
	if err != nil {
		return fmt.Errorf("failed to list services: %w", err)
	}

	// For each service, try to connect to endpoints
	for _, service := range services.Items {
		endpoints, err := client.CoreV1().Endpoints(namespace).Get(ctx, service.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get endpoints for service %s: %w", service.Name, err)
		}

		hasEndpoints := false
		for _, subset := range endpoints.Subsets {
			if len(subset.Addresses) > 0 {
				hasEndpoints = true
				break
			}
		}

		if !hasEndpoints {
			return fmt.Errorf("service %s has no ready endpoints", service.Name)
		}
	}

	return nil
}

// collectDiagnosticInfo collects diagnostic information on test failure
func collectDiagnosticInfo(client kubernetes.Interface, namespace, chartName string) {
	ctx := context.Background()
	
	// Log pod statuses
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", chartName),
	})
	if err == nil {
		for _, pod := range pods.Items {
			fmt.Printf("🔍 Pod %s status: Phase=%s, Ready=%t\n", pod.Name, pod.Status.Phase, isPodReady(&pod))
			
			// Get recent events
			events, err := client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
				FieldSelector: fmt.Sprintf("involvedObject.name=%s", pod.Name),
			})
			if err == nil {
				for _, event := range events.Items {
					fmt.Printf("📝 Event: %s - %s\n", event.Reason, event.Message)
				}
			}
		}
	}
}

// preTestCleanup performs pre-test cleanup
func preTestCleanup(ctx context.Context, client kubernetes.Interface, chartName, namespace string) error {
	// This would clean up any existing resources if needed
	return nil
}

// cleanupChart cleans up chart resources
func cleanupChart(chartName, namespace string) {
	// This would use deploy-cli or helm to cleanup
	fmt.Printf("🧹 Cleanup would remove chart %s from namespace %s\n", chartName, namespace)
}

// isIntegrationTestEnvironment checks if we're in an integration test environment
func isIntegrationTestEnvironment() bool {
	// Check for integration test flag or CI environment
	return strings.ToLower(getEnv("INTEGRATION_TESTS", "false")) == "true" || 
		   getEnv("CI", "") != ""
}

// keepTestResources checks if test resources should be kept for inspection
func keepTestResources() bool {
	return strings.ToLower(getEnv("KEEP_TEST_RESOURCES", "false")) == "true"
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}