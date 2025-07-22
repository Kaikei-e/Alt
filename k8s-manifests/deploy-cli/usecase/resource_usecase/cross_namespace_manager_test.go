package resource_usecase

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"deploy-cli/domain"
)

// mockKubectlPort implements port.KubectlPort for testing
type mockKubectlPort struct {
	resources map[string]map[string]interface{} // namespace/type/name -> resource
}

func newMockKubectlPort() *mockKubectlPort {
	return &mockKubectlPort{
		resources: make(map[string]map[string]interface{}),
	}
}

func (m *mockKubectlPort) GetResourceByName(ctx context.Context, namespace, resourceType, name string) (map[string]interface{}, error) {
	key := namespace + "/" + resourceType + "/" + name
	if resource, exists := m.resources[key]; exists {
		return resource, nil
	}
	return nil, &KubernetesResourceNotFoundError{Resource: name, Namespace: namespace}
}

func (m *mockKubectlPort) ListResources(ctx context.Context, namespace, resourceType string) (map[string]interface{}, error) {
	var items []interface{}
	prefix := namespace + "/" + resourceType + "/"
	
	for key, resource := range m.resources {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			items = append(items, resource)
		}
	}
	
	return map[string]interface{}{
		"items": items,
	}, nil
}

func (m *mockKubectlPort) ExecuteCommand(ctx context.Context, command string) error {
	// Mock implementation - just log the command
	return nil
}

func (m *mockKubectlPort) ApplyYaml(ctx context.Context, yaml string) error {
	// Mock implementation - just log the YAML
	return nil
}

// addMockResource adds a mock resource for testing
func (m *mockKubectlPort) addMockResource(namespace, resourceType, name string, annotations map[string]string) {
	key := namespace + "/" + resourceType + "/" + name
	resource := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
			"annotations": func() map[string]interface{} {
				result := make(map[string]interface{})
				for k, v := range annotations {
					result[k] = v
				}
				return result
			}(),
			"creationTimestamp": "2025-07-22T11:00:00Z",
		},
	}
	m.resources[key] = resource
}

// KubernetesResourceNotFoundError represents a resource not found error
type KubernetesResourceNotFoundError struct {
	Resource  string
	Namespace string
}

func (e *KubernetesResourceNotFoundError) Error() string {
	return "resource not found: " + e.Resource + " in namespace " + e.Namespace
}

func TestCrossNamespaceResourceManager_ManageMultiNamespaceDeployment(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mockKubectl := newMockKubectlPort()
	
	// Add some conflicting resources
	mockKubectl.addMockResource("alt-apps", "Secret", "server-ssl-secret", map[string]string{
		"meta.helm.sh/release-name":      "common-ssl",
		"meta.helm.sh/release-namespace": "alt-apps",
	})
	
	mockKubectl.addMockResource("alt-auth", "Secret", "server-ssl-secret", map[string]string{
		"meta.helm.sh/release-name":      "auth-service",
		"meta.helm.sh/release-namespace": "alt-auth",
	})

	manager := NewCrossNamespaceResourceManager(mockKubectl, logger)

	// Create deployment plan
	plan := MultiNamespaceDeploymentPlan{
		Environment:      domain.Production,
		TargetNamespaces: []string{"alt-apps", "alt-auth"},
		SharedResources: []SharedResource{
			{
				Type:       "Secret",
				Name:       "server-ssl-secret",
				OwnerChart: "common-ssl",
				Namespaces: []string{"alt-apps", "alt-auth"},
				Priority:   1,
			},
		},
		ChartDeployments: []NamespaceChartDeployment{
			{
				Chart: domain.Chart{
					Name:    "common-ssl",
					Version: "0.1.0",
				},
				Namespace: "alt-apps",
				Priority:  1,
			},
			{
				Chart: domain.Chart{
					Name:    "auth-service",
					Version: "0.1.0",
				},
				Namespace: "alt-auth",
				Priority:  2,
			},
		},
		DependencyGraph: map[string][]string{
			"auth-service": {"common-ssl"},
		},
	}

	// Test deployment management
	err := manager.ManageMultiNamespaceDeployment(context.Background(), plan)
	if err != nil {
		t.Fatalf("ManageMultiNamespaceDeployment failed: %v", err)
	}

	t.Log("Multi-namespace deployment management test passed")
}

func TestResourceConflictDetector_DetectConflicts(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mockKubectl := newMockKubectlPort()
	
	// Add conflicting resource
	mockKubectl.addMockResource("alt-auth", "Secret", "server-ssl-secret", map[string]string{
		"meta.helm.sh/release-name":      "existing-release",
		"meta.helm.sh/release-namespace": "alt-auth",
	})

	detector := NewResourceConflictDetector(mockKubectl, logger)

	// Create deployment plan with potential conflicts
	plan := MultiNamespaceDeploymentPlan{
		Environment:      domain.Production,
		TargetNamespaces: []string{"alt-apps", "alt-auth"},
		SharedResources: []SharedResource{
			{
				Type:       "Secret",
				Name:       "server-ssl-secret",
				OwnerChart: "new-owner",
				Namespaces: []string{"alt-auth"},
				Priority:   1,
			},
		},
	}

	// Test conflict detection
	conflicts, err := detector.DetectConflicts(context.Background(), plan)
	if err != nil {
		t.Fatalf("DetectConflicts failed: %v", err)
	}

	if len(conflicts) == 0 {
		t.Error("Expected conflicts to be detected, but none were found")
	}

	// Verify conflict details
	for _, conflict := range conflicts {
		t.Logf("Detected conflict: %s for resource %s (severity: %s)",
			conflict.ConflictType, conflict.ResourceName, conflict.Severity)
		
		if conflict.ResourceName != "server-ssl-secret" {
			t.Errorf("Expected conflict for server-ssl-secret, got %s", conflict.ResourceName)
		}
		
		if conflict.ConflictType != "ownership" {
			t.Errorf("Expected ownership conflict, got %s", conflict.ConflictType)
		}
	}

	t.Log("Conflict detection test passed")
}

func TestNamespaceResourceTracker_TrackResource(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mockKubectl := newMockKubectlPort()
	
	// Add test resource
	mockKubectl.addMockResource("alt-apps", "Secret", "test-secret", map[string]string{
		"meta.helm.sh/release-name":      "test-chart",
		"meta.helm.sh/release-namespace": "alt-apps",
	})

	tracker := NewNamespaceResourceTracker(mockKubectl, logger)

	// Test resource tracking
	err := tracker.TrackResource(context.Background(), "alt-apps", "Secret", "test-secret")
	if err != nil {
		t.Fatalf("TrackResource failed: %v", err)
	}

	// Test ownership retrieval
	ownership, err := tracker.GetResourceOwnership(context.Background(), "alt-apps", "Secret", "test-secret")
	if err != nil {
		t.Fatalf("GetResourceOwnership failed: %v", err)
	}

	if ownership.OwnerChart != "test-chart" {
		t.Errorf("Expected owner chart 'test-chart', got '%s'", ownership.OwnerChart)
	}

	if ownership.Namespace != "alt-apps" {
		t.Errorf("Expected namespace 'alt-apps', got '%s'", ownership.Namespace)
	}

	t.Log("Resource tracking test passed")
}

func TestConflictResolver_ResolveConflicts(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mockKubectl := newMockKubectlPort()
	
	// Add conflicting resource
	mockKubectl.addMockResource("alt-auth", "Secret", "conflicted-secret", map[string]string{
		"meta.helm.sh/release-name":      "old-owner",
		"meta.helm.sh/release-namespace": "alt-auth",
	})

	resolver := NewConflictResolver(mockKubectl, logger)

	// Create test conflicts
	conflicts := []ResourceConflict{
		{
			ResourceType: "Secret",
			ResourceName: "conflicted-secret",
			ConflictType: "ownership",
			SourceChart:  "new-owner",
			TargetChart:  "old-owner",
			Namespaces:   []string{"alt-auth"},
			Severity:     "critical",
		},
	}

	// Test conflict resolution
	err := resolver.ResolveConflicts(context.Background(), conflicts)
	if err != nil {
		t.Fatalf("ResolveConflicts failed: %v", err)
	}

	t.Log("Conflict resolution test passed")
}

func TestIntegration_FullConflictDetectionAndResolution(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mockKubectl := newMockKubectlPort()
	
	// Setup conflicting resources scenario
	mockKubectl.addMockResource("alt-apps", "Secret", "server-ssl-secret", map[string]string{
		"meta.helm.sh/release-name":      "common-ssl",
		"meta.helm.sh/release-namespace": "alt-apps",
	})
	
	mockKubectl.addMockResource("alt-auth", "Secret", "server-ssl-secret", map[string]string{
		"meta.helm.sh/release-name":      "auth-service",
		"meta.helm.sh/release-namespace": "alt-auth",
	})

	manager := NewCrossNamespaceResourceManager(mockKubectl, logger)

	// Test state validation
	err := manager.ValidateMultiNamespaceState(context.Background(), []string{"alt-apps", "alt-auth"})
	if err != nil {
		t.Logf("Expected validation to find conflicts: %v", err)
	} else {
		t.Log("No conflicts found in current state")
	}

	t.Log("Integration test completed")
}