package security

import (
	"context"
	"net"
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

func TestSSRFValidator_BasicValidation(t *testing.T) {
	validator := NewSSRFValidator()

	tests := []struct {
		name        string
		url         string
		wantErr     bool
		expectedErr string
	}{
		{
			name:    "valid https URL",
			url:     "https://example.com/image.jpg",
			wantErr: false,
		},
		{
			name:    "valid http URL",
			url:     "http://example.com/image.jpg",
			wantErr: false,
		},
		{
			name:        "invalid scheme ftp",
			url:         "ftp://example.com/file.txt",
			wantErr:     true,
			expectedErr: "SCHEME_VALIDATION_ERROR",
		},
		{
			name:        "empty host",
			url:         "https:///path",
			wantErr:     true,
			expectedErr: "BASIC_VALIDATION_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.url)
			require.NoError(t, err)

			err = validator.ValidateURL(context.Background(), u)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSSRFValidator_MetadataEndpoints(t *testing.T) {
	validator := NewSSRFValidator()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "AWS metadata endpoint",
			url:     "http://169.254.169.254/latest/meta-data/",
			wantErr: true,
		},
		{
			name:    "Oracle Cloud metadata",
			url:     "http://192.0.0.192/opc/v1/instance/",
			wantErr: true,
		},
		{
			name:    "GCP metadata",
			url:     "http://metadata.google.internal/computeMetadata/v1/",
			wantErr: true,
		},
		{
			name:    "safe external domain",
			url:     "https://example.com/image.jpg",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.url)
			require.NoError(t, err)

			err = validator.ValidateURL(context.Background(), u)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "METADATA_ENDPOINT_BLOCKED")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSSRFValidator_InternalDomains(t *testing.T) {
	validator := NewSSRFValidator()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "local domain",
			url:     "https://service.local/api",
			wantErr: true,
		},
		{
			name:    "internal domain",
			url:     "https://api.internal/data",
			wantErr: true,
		},
		{
			name:    "kubernetes cluster domain",
			url:     "https://service.default.svc.cluster.local/api",
			wantErr: true,
		},
		{
			name:    "safe external domain",
			url:     "https://example.com/image.jpg",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.url)
			require.NoError(t, err)

			err = validator.ValidateURL(context.Background(), u)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "INTERNAL_DOMAIN_BLOCKED")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSSRFValidator_PathTraversal(t *testing.T) {
	validator := NewSSRFValidator()

	tests := []struct {
		name      string
		url       string
		wantErr   bool
		errorType string
	}{
		{
			name:      "path traversal with dots",
			url:       "https://example.com/../admin",
			wantErr:   true,
			errorType: "PATH_TRAVERSAL_BLOCKED",
		},
		{
			name:      "path traversal with /.",
			url:       "https://example.com/./config",
			wantErr:   true,
			errorType: "PATH_TRAVERSAL_BLOCKED",
		},
		{
			name:      "URL encoded dot attack",
			url:       "https://example.com/%2e%2e/admin",
			wantErr:   true,
			errorType: "URL_ENCODING_BLOCKED",
		},
		{
			name:      "URL encoded slash attack",
			url:       "https://example.com/test%2fmalicious",
			wantErr:   true,
			errorType: "URL_ENCODING_BLOCKED",
		},
		{
			name:    "safe path",
			url:     "https://example.com/safe/path/image.jpg",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.url)
			require.NoError(t, err)

			err = validator.ValidateURL(context.Background(), u)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorType)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSSRFValidator_PortValidation(t *testing.T) {
	validator := NewSSRFValidator()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "allowed port 443",
			url:     "https://example.com:443/image.jpg",
			wantErr: false,
		},
		{
			name:    "allowed port 80",
			url:     "http://example.com:80/image.jpg",
			wantErr: false,
		},
		{
			name:    "allowed port 8080",
			url:     "https://example.com:8080/image.jpg",
			wantErr: false,
		},
		{
			name:    "disallowed port 3000",
			url:     "https://example.com:3000/image.jpg",
			wantErr: true,
		},
		{
			name:    "disallowed port 22",
			url:     "https://example.com:22/image.jpg",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.url)
			require.NoError(t, err)

			err = validator.ValidateURL(context.Background(), u)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "PORT_BLOCKED")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSSRFValidator_UnicodeValidation(t *testing.T) {
	validator := NewSSRFValidator()

	tests := []struct {
		name      string
		url       string
		wantErr   bool
		errorType string
	}{
		{
			name:      "Cyrillic confusable domain",
			url:       "https://еxample.com/image.jpg", // Cyrillic 'е' instead of 'e'
			wantErr:   true,
			errorType: "MIXED_SCRIPT_BLOCKED", // This is detected as mixed script first
		},
		{
			name:      "mixed Latin and Cyrillic",
			url:       "https://gооgle.com/image.jpg", // Mixed scripts
			wantErr:   true,
			errorType: "MIXED_SCRIPT_BLOCKED",
		},
		{
			name:    "normal ASCII domain",
			url:     "https://example.com/image.jpg",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.url)
			require.NoError(t, err)

			err = validator.ValidateURL(context.Background(), u)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorType)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSSRFValidator_TestingMode(t *testing.T) {
	validator := NewSSRFValidator()

	// Test that localhost is blocked by default
	u, err := url.Parse("http://localhost:8080/test")
	require.NoError(t, err)

	err = validator.ValidateURL(context.Background(), u)
	assert.Error(t, err)

	// Enable testing mode
	validator.SetTestingMode(true)

	// Now localhost should be allowed for testing
	err = validator.ValidateURL(context.Background(), u)
	assert.NoError(t, err)
}

func TestSSRFValidator_HasMixedScripts(t *testing.T) {
	validator := NewSSRFValidator()

	tests := []struct {
		name     string
		hostname string
		expected bool
	}{
		{
			name:     "pure Latin",
			hostname: "example.com",
			expected: false,
		},
		{
			name:     "pure Cyrillic",
			hostname: "пример.рф",
			expected: false,
		},
		{
			name:     "mixed Latin and Cyrillic",
			hostname: "exаmple.com", // 'а' is Cyrillic
			expected: true,
		},
		{
			name:     "mixed with numbers",
			hostname: "example123.com",
			expected: false, // numbers don't count as separate script
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.hasMixedScripts(tt.hostname)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSSRFValidator_HasConfusableChars(t *testing.T) {
	validator := NewSSRFValidator()

	tests := []struct {
		name     string
		hostname string
		expected bool
	}{
		{
			name:     "normal ASCII",
			hostname: "example.com",
			expected: false,
		},
		{
			name:     "Cyrillic 'а' instead of Latin 'a'",
			hostname: "exаmple.com",
			expected: true,
		},
		{
			name:     "Cyrillic 'е' instead of Latin 'e'",
			hostname: "еxample.com",
			expected: true,
		},
		{
			name:     "multiple confusables",
			hostname: "gооgle.cоm", // Multiple Cyrillic 'о'
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.hasConfusableChars(tt.hostname)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSSRFValidator_PrivateIPRanges(t *testing.T) {
	validator := NewSSRFValidator()

	tests := []struct {
		name      string
		url       string
		wantErr   bool
		errorType string
	}{
		// Private IPv4 ranges - 10.0.0.0/8
		{
			name:      "private IP 10.0.0.1",
			url:       "http://10.0.0.1/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		{
			name:      "private IP 10.255.255.255",
			url:       "http://10.255.255.255/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		// Private IPv4 ranges - 172.16.0.0/12
		{
			name:      "private IP 172.16.0.1",
			url:       "http://172.16.0.1/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		{
			name:      "private IP 172.31.255.255",
			url:       "http://172.31.255.255/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		// 172.15.x.x and 172.32.x.x should be allowed (not in private range)
		{
			name:    "non-private IP 172.15.0.1",
			url:     "http://172.15.0.1/api",
			wantErr: false,
		},
		{
			name:    "non-private IP 172.32.0.1",
			url:     "http://172.32.0.1/api",
			wantErr: false,
		},
		// Private IPv4 ranges - 192.168.0.0/16
		{
			name:      "private IP 192.168.0.1",
			url:       "http://192.168.0.1/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		{
			name:      "private IP 192.168.255.255",
			url:       "http://192.168.255.255/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		// Loopback addresses
		{
			name:      "loopback 127.0.0.1",
			url:       "http://127.0.0.1/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		{
			name:      "loopback 127.0.0.2",
			url:       "http://127.0.0.2/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		{
			name:      "localhost hostname",
			url:       "http://localhost/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		// Link-local addresses
		{
			name:      "link-local 169.254.0.1",
			url:       "http://169.254.0.1/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		// Public IP should pass
		{
			name:    "public IP 8.8.8.8",
			url:     "http://8.8.8.8/api",
			wantErr: false,
		},
		{
			name:    "public IP 1.1.1.1",
			url:     "http://1.1.1.1/api",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.url)
			require.NoError(t, err)

			err = validator.ValidateURL(context.Background(), u)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorType != "" {
					assert.Contains(t, err.Error(), tt.errorType)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSSRFValidator_IsPrivateOrDangerous(t *testing.T) {
	validator := NewSSRFValidator()

	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		// Private IPv4 ranges
		{"10.0.0.1", "10.0.0.1", true},
		{"172.16.0.1", "172.16.0.1", true},
		{"172.31.255.255", "172.31.255.255", true},
		{"192.168.1.1", "192.168.1.1", true},
		{"127.0.0.1", "127.0.0.1", true},
		{"169.254.169.254", "169.254.169.254", true},
		// Public IPs
		{"8.8.8.8", "8.8.8.8", false},
		{"1.1.1.1", "1.1.1.1", false},
		{"172.15.0.1", "172.15.0.1", false},
		{"172.32.0.1", "172.32.0.1", false},
		// IPv6 private ranges
		{"fc00::1", "fc00::1", true},
		{"fd00::1", "fd00::1", true},
		{"::1", "::1", true},         // IPv6 loopback
		{"fe80::1", "fe80::1", true}, // Link-local
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			require.NotNil(t, ip, "failed to parse IP: %s", tt.ip)

			result := validator.isPrivateOrDangerous(ip)
			assert.Equal(t, tt.expected, result, "IP: %s", tt.ip)
		})
	}
}

func TestSSRFValidator_CreateSecureHTTPClient(t *testing.T) {
	validator := NewSSRFValidator()

	// Test that client is created successfully
	client := validator.CreateSecureHTTPClient(30 * time.Second)
	assert.NotNil(t, client)
	assert.Equal(t, 30*time.Second, client.Timeout)

	// Test that redirects are blocked
	req := httptest.NewRequest("GET", "http://example.com", nil)
	err := client.CheckRedirect(req, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redirects not allowed")
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
			url:         "http://127.0.0.1:8080/admin",
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
			description: "Encoded path traversal",
			wantErr:     true,
			errorType:   "URL_ENCODING_BLOCKED",
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
			url:         "https://xn--e1afmkfd.xn--p1ai/api", // российский domain
			description: "Internationalized domain (may fail DNS in test environment)",
			wantErr:     false, // Note: May fail with DNS_RESOLUTION_ERROR in isolated environments
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
				// Allow DNS resolution errors in isolated test environments
				// The validator correctly validates everything except actual DNS resolution
				if err != nil && strings.Contains(err.Error(), "DNS_RESOLUTION_ERROR") {
					t.Skipf("Skipping test due to DNS resolution failure (isolated environment): %s", tt.description)
				}
				assert.NoError(t, err, "Unexpected error for: %s", tt.description)
			}
		})
	}
}

// Benchmarks
func BenchmarkSSRFValidator_ValidateURL(b *testing.B) {
	validator := NewSSRFValidator()
	u, _ := url.Parse("https://example.com/image.jpg")
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.ValidateURL(ctx, u)
	}
}

func BenchmarkSSRFValidator_UnicodeValidation(b *testing.B) {
	validator := NewSSRFValidator()
	u, _ := url.Parse("https://еxample.com/image.jpg")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.validateUnicodeAndPunycode(u)
	}
}
