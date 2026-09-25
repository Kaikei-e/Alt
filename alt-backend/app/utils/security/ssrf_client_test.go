package security

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSSRFValidator(t *testing.T) {
	validator := NewSSRFValidator()

	assert.NotNil(t, validator)
	assert.False(t, validator.allowTestingLocalhost)
	assert.NotEmpty(t, validator.metadataEndpoints)
	assert.NotEmpty(t, validator.internalDomains)
	assert.NotEmpty(t, validator.allowedPorts)
}

func TestSSRFValidator_CreateSecureHTTPClient(t *testing.T) {
	validator := NewSSRFValidator()

	// Test that client is created successfully
	client := validator.CreateSecureHTTPClient(30 * time.Second)
	assert.NotNil(t, client)
	assert.Equal(t, 30*time.Second, client.Timeout)

	// Test a valid redirect (should be allowed)
	req := httptest.NewRequest("GET", "http://93.184.216.34", nil)
	err := client.CheckRedirect(req, nil)
	assert.NoError(t, err)

	// Test an invalid redirect (should be blocked)
	reqBlocked := httptest.NewRequest("GET", "http://169.254.169.254/latest/meta-data/", nil)
	err = client.CheckRedirect(reqBlocked, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redirect blocked by SSRF policy")
}

func TestSSRFValidator_ComprehensiveAttackScenarios(t *testing.T) {
	validator := NewSSRFValidator()

	tests := []struct {
		name        string
		url         string
		description string
		wantErr     bool
		errorType   string
	}{
		{
			name:        "SSRF to AWS metadata",
			url:         "http://169.254.169.254/latest/meta-data/iam/security-credentials/",
			description: "Attempt to access AWS instance metadata",
			wantErr:     true,
			errorType:   "METADATA_ENDPOINT_BLOCKED",
		},
		{
			name:        "DNS Rebinding to localhost",
			url:         "http://127.0.0.1/admin",
			description: "Attempt to access localhost admin panel",
			wantErr:     true,
			errorType:   "DNS_REBINDING_BLOCKED",
		},
		{
			name:        "Private network scan",
			url:         "http://192.168.1.1/api",
			description: "Attempt to scan private network",
			wantErr:     true,
			errorType:   "DNS_REBINDING_BLOCKED",
		},
		{
			name:        "Kubernetes API server",
			url:         "https://kubernetes.default.svc.cluster.local/api",
			description: "Attempt to access k8s API from within cluster",
			wantErr:     true,
			errorType:   "INTERNAL_DOMAIN_BLOCKED",
		},
		{
			name:        "Path traversal to sensitive file",
			url:         "https://example.com/../../../etc/passwd",
			description: "Path traversal attack",
			wantErr:     true,
			errorType:   "PATH_TRAVERSAL_BLOCKED",
		},
		{
			name:        "URL encoded path traversal",
			url:         "https://example.com/%2e%2e%2f%2e%2e%2fadmin",
			description: "Encoded path traversal (Go decodes %2e to dot, caught by decoded-path check)",
			wantErr:     true,
			errorType:   "PATH_TRAVERSAL_BLOCKED",
		},
		{
			name:        "Suspicious port access",
			url:         "http://example.com:22/api",
			description: "Attempt to access SSH port",
			wantErr:     true,
			errorType:   "PORT_BLOCKED",
		},
		{
			name:        "Punycode domain validation",
			url:         "https://xn--e1afmkfd.xn--p1ai/api",
			description: "Internationalized domain (may fail DNS in test environment)",
			wantErr:     false,
		},
		{
			name:        "Safe public IP",
			url:         "https://8.8.8.8/api",
			description: "Legitimate public IP API call",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.url)
			require.NoError(t, err, "Failed to parse URL for test: %s", tt.description)

			err = validator.ValidateURL(context.Background(), u)

			if tt.wantErr {
				assert.Error(t, err, "Expected error for: %s", tt.description)
				if tt.errorType != "" {
					assert.Contains(t, err.Error(), tt.errorType, "Test: %s", tt.description)
				}
			} else {
				if err != nil && strings.Contains(err.Error(), "DNS_RESOLUTION_ERROR") {
					t.Skipf("Skipping test due to DNS resolution failure (isolated environment): %s", tt.description)
				}
				assert.NoError(t, err, "Unexpected error for: %s", tt.description)
			}
		})
	}
}

func TestSSRFValidator_ValidateDialIP(t *testing.T) {
	validator := NewSSRFValidator()

	assert.Error(t, validator.ValidateDialIP("tcp", "127.0.0.1:80"))
	assert.Error(t, validator.ValidateDialIP("tcp", "10.0.0.1:80"))
	assert.Error(t, validator.ValidateDialIP("tcp", "169.254.169.254:80"))
	assert.Error(t, validator.ValidateDialIP("tcp", "not-an-ip:80"))
	assert.Error(t, validator.ValidateDialIP("tcp", "bad-format"))
	assert.NoError(t, validator.ValidateDialIP("tcp", "8.8.8.8:80"))
}

func TestSSRFValidator_ValidateConnectionAddress(t *testing.T) {
	validator := NewSSRFValidator()

	assert.Error(t, validator.validateConnectionAddress("tcp", "invalid-format"))
	assert.Error(t, validator.validateConnectionAddress("tcp", "not-an-ip:443"))
	assert.Error(t, validator.validateConnectionAddress("tcp", "127.0.0.1:443"))
	assert.Error(t, validator.validateConnectionAddress("tcp", "8.8.8.8:22")) // Port 22 blocked
	assert.NoError(t, validator.validateConnectionAddress("tcp", "8.8.8.8:443"))
}
