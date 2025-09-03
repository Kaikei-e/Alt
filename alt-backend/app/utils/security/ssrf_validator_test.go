package security

import (
	"context"
	"net/url"
	"testing"

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
		name     string
		url      string
		wantErr  bool
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
		name     string
		url      string
		wantErr  bool
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
		name     string
		url      string
		wantErr  bool
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
		name     string
		url      string
		wantErr  bool
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
		name     string
		url      string
		wantErr  bool
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