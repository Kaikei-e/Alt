package kubectl_driver

import (
	"testing"

	"deploy-cli/port/kubectl_port"
)

func TestNewKubectlDriver(t *testing.T) {
	driver := NewKubectlDriver()
	if driver == nil {
		t.Fatal("NewKubectlDriver() returned nil")
	}
}

func TestBuildSecretManifest(t *testing.T) {
	driver := NewKubectlDriver()

	secret := &kubectl_port.KubernetesSecret{
		Name:      "test-secret",
		Namespace: "test-namespace",
		Type:      "kubernetes.io/tls",
		Data: map[string][]byte{
			"tls.crt": []byte("test-cert"),
			"tls.key": []byte("test-key"),
		},
		Labels: map[string]string{
			"app.kubernetes.io/name":       "test",
			"app.kubernetes.io/managed-by": "deploy-cli",
		},
		Annotations: map[string]string{
			"deploy-cli/created-at": "2025-07-19T12:00:00Z",
		},
	}

	manifest, err := driver.buildSecretManifest(secret)
	if err != nil {
		t.Fatalf("buildSecretManifest() failed: %v", err)
	}

	if len(manifest) == 0 {
		t.Fatal("buildSecretManifest() returned empty manifest")
	}

	// Check if manifest contains required fields
	manifestStr := string(manifest)
	requiredFields := []string{
		"test-secret",
		"test-namespace",
		"kubernetes.io/tls",
		"app.kubernetes.io/name",
		"deploy-cli/created-at",
	}

	for _, field := range requiredFields {
		if !contains(manifestStr, field) {
			t.Errorf("buildSecretManifest() manifest missing required field: %s", field)
		}
	}
}

func TestSSLSecretTypeValidation(t *testing.T) {
	tests := []struct {
		name       string
		secretType string
		want       bool
	}{
		{
			name:       "valid TLS secret type",
			secretType: "kubernetes.io/tls",
			want:       true,
		},
		{
			name:       "invalid ssl secret type",
			secretType: "ssl",
			want:       false,
		},
		{
			name:       "valid opaque secret type",
			secretType: "Opaque",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidSecretType(tt.secretType)
			if got != tt.want {
				t.Errorf("isValidSecretType(%s) = %v, want %v", tt.secretType, got, tt.want)
			}
		})
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) &&
			(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
				containsInner(s, substr))))
}

func containsInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Helper function to validate secret types
func isValidSecretType(secretType string) bool {
	validTypes := []string{
		"kubernetes.io/tls",
		"Opaque",
		"kubernetes.io/service-account-token",
		"kubernetes.io/dockercfg",
		"kubernetes.io/dockerconfigjson",
	}

	for _, validType := range validTypes {
		if secretType == validType {
			return true
		}
	}

	// Check for deprecated/invalid types
	invalidTypes := []string{"ssl", "api", "database"}
	for _, invalidType := range invalidTypes {
		if secretType == invalidType {
			return false
		}
	}

	return true // Allow custom types
}
