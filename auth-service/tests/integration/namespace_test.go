package integration_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// NamespaceManifest represents a Kubernetes namespace
type NamespaceManifest struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name   string            `yaml:"name"`
		Labels map[string]string `yaml:"labels"`
	} `yaml:"metadata"`
}

func TestNamespaceConfiguration(t *testing.T) {
	tests := []struct {
		name              string
		namespaceConfig   NamespaceManifest
		expectedName      string
		expectedLabels    map[string]string
		validateStructure bool
	}{
		{
			name: "alt-auth namespace structure",
			namespaceConfig: NamespaceManifest{
				APIVersion: "v1",
				Kind:       "Namespace",
				Metadata: struct {
					Name   string            `yaml:"name"`
					Labels map[string]string `yaml:"labels"`
				}{
					Name: "alt-auth",
					Labels: map[string]string{
						"name":                          "alt-auth",
						"environment":                   "production",
						"service-type":                  "authentication",
						"app.kubernetes.io/managed-by":  "kustomize",
						"app.kubernetes.io/part-of":     "alt",
						"app.kubernetes.io/component":   "authentication",
					},
				},
			},
			expectedName: "alt-auth",
			expectedLabels: map[string]string{
				"name":                          "alt-auth",
				"environment":                   "production",
				"service-type":                  "authentication",
				"app.kubernetes.io/managed-by":  "kustomize",
				"app.kubernetes.io/part-of":     "alt",
				"app.kubernetes.io/component":   "authentication",
			},
			validateStructure: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate basic structure
			if tt.validateStructure {
				assert.Equal(t, "v1", tt.namespaceConfig.APIVersion)
				assert.Equal(t, "Namespace", tt.namespaceConfig.Kind)
			}

			// Validate namespace name
			assert.Equal(t, tt.expectedName, tt.namespaceConfig.Metadata.Name)

			// Validate required labels
			for key, expectedValue := range tt.expectedLabels {
				actualValue, exists := tt.namespaceConfig.Metadata.Labels[key]
				require.True(t, exists, "Label %s should exist", key)
				assert.Equal(t, expectedValue, actualValue, "Label %s should have correct value", key)
			}

			// Validate label count matches expected
			assert.Equal(t, len(tt.expectedLabels), len(tt.namespaceConfig.Metadata.Labels),
				"Number of labels should match expected count")
		})
	}
}

func TestNamespaceYAMLSerialization(t *testing.T) {
	namespace := NamespaceManifest{
		APIVersion: "v1",
		Kind:       "Namespace",
		Metadata: struct {
			Name   string            `yaml:"name"`
			Labels map[string]string `yaml:"labels"`
		}{
			Name: "alt-auth",
			Labels: map[string]string{
				"name":                          "alt-auth",
				"environment":                   "production",
				"service-type":                  "authentication",
				"app.kubernetes.io/managed-by":  "kustomize",
				"app.kubernetes.io/part-of":     "alt",
				"app.kubernetes.io/component":   "authentication",
			},
		},
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(namespace)
	require.NoError(t, err)
	assert.Contains(t, string(yamlData), "alt-auth")
	assert.Contains(t, string(yamlData), "authentication")

	// Test YAML deserialization
	var unmarshaled NamespaceManifest
	err = yaml.Unmarshal(yamlData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, namespace.Metadata.Name, unmarshaled.Metadata.Name)
	assert.Equal(t, namespace.Metadata.Labels, unmarshaled.Metadata.Labels)
}

func TestNamespaceLabelValidation(t *testing.T) {
	tests := []struct {
		name         string
		labels       map[string]string
		shouldPass   bool
		missingLabel string
	}{
		{
			name: "valid auth namespace labels",
			labels: map[string]string{
				"name":                          "alt-auth",
				"environment":                   "production",
				"service-type":                  "authentication",
				"app.kubernetes.io/managed-by":  "kustomize",
				"app.kubernetes.io/part-of":     "alt",
				"app.kubernetes.io/component":   "authentication",
			},
			shouldPass: true,
		},
		{
			name: "missing service-type label",
			labels: map[string]string{
				"name":                          "alt-auth",
				"environment":                   "production",
				"app.kubernetes.io/managed-by":  "kustomize",
				"app.kubernetes.io/part-of":     "alt",
				"app.kubernetes.io/component":   "authentication",
			},
			shouldPass:   false,
			missingLabel: "service-type",
		},
		{
			name: "missing environment label",
			labels: map[string]string{
				"name":                          "alt-auth",
				"service-type":                  "authentication",
				"app.kubernetes.io/managed-by":  "kustomize",
				"app.kubernetes.io/part-of":     "alt",
				"app.kubernetes.io/component":   "authentication",
			},
			shouldPass:   false,
			missingLabel: "environment",
		},
	}

	requiredLabels := []string{
		"name",
		"environment",
		"service-type",
		"app.kubernetes.io/managed-by",
		"app.kubernetes.io/part-of",
		"app.kubernetes.io/component",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allPresent := true
			var missingLabel string

			for _, requiredLabel := range requiredLabels {
				if _, exists := tt.labels[requiredLabel]; !exists {
					allPresent = false
					missingLabel = requiredLabel
					break
				}
			}

			if tt.shouldPass {
				assert.True(t, allPresent, "All required labels should be present")
			} else {
				assert.False(t, allPresent, "Should be missing required label")
				assert.Equal(t, tt.missingLabel, missingLabel, "Should identify correct missing label")
			}
		})
	}
}