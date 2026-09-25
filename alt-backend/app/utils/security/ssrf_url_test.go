package security

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			url:     "https://93.184.216.34/image.jpg",
			wantErr: false,
		},
		{
			name:    "valid http URL",
			url:     "http://93.184.216.34/image.jpg",
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
			url:     "https://93.184.216.34/image.jpg",
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
			url:     "https://93.184.216.34/image.jpg",
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
			errorType: "PATH_TRAVERSAL_BLOCKED",
		},
		{
			name:    "URL encoded slash in path (legitimate CDN pattern)",
			url:     "https://example.com/test%2fmalicious",
			wantErr: false,
		},
		{
			name:    "safe path",
			url:     "https://93.184.216.34/safe/path/image.jpg",
			wantErr: false,
		},
		{
			name:    "encoded characters in query parameters (allowed)",
			url:     "https://example.com/path?redirect=https%3A%2F%2Fother.com",
			wantErr: false,
		},
		{
			name:    "complex redirect URL in query parameters (allowed)",
			url:     "https://medium.com/m/global-identity-2?redirectUrl=https%3A%2F%2Fuxplanet.org%2Frobots.txt",
			wantErr: false,
		},
		{
			name:    "CDN URL with encoded commas and slashes in path (dev.to)",
			url:     "https://93.184.216.34/dynamic/image/width=800%2Cheight=%2Cfit=scale-down/https%3A%2F%2Fexample.com%2Fimage.jpg",
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
			url:     "https://93.184.216.34:443/image.jpg",
			wantErr: false,
		},
		{
			name:    "allowed port 80",
			url:     "http://93.184.216.34:80/image.jpg",
			wantErr: false,
		},
		{
			name:    "non-standard port 8080 now blocked",
			url:     "https://93.184.216.34:8080/image.jpg",
			wantErr: true,
		},
		{
			name:    "disallowed port 3000",
			url:     "https://93.184.216.34:3000/image.jpg",
			wantErr: true,
		},
		{
			name:    "disallowed port 22",
			url:     "https://93.184.216.34:22/image.jpg",
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
			url:       "https://еxample.com/image.jpg",
			wantErr:   true,
			errorType: "MIXED_SCRIPT_BLOCKED",
		},
		{
			name:      "mixed Latin and Cyrillic",
			url:       "https://gооgle.com/image.jpg",
			wantErr:   true,
			errorType: "MIXED_SCRIPT_BLOCKED",
		},
		{
			name:    "normal ASCII domain",
			url:     "https://93.184.216.34/image.jpg",
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

	u, err := url.Parse("http://127.0.0.1:8080/test")
	require.NoError(t, err)

	err = validator.ValidateURL(context.Background(), u)
	assert.Error(t, err)

	validator.SetTestingMode(true)

	err = validator.ValidateURL(context.Background(), u)
	assert.NoError(t, err)
}

func TestSSRFValidator_HasMixedScripts(t *testing.T) {
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
			hostname: "exаmple.com",
			expected: true,
		},
		{
			name:     "mixed with numbers",
			hostname: "example123.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasMixedScripts(tt.hostname)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSSRFValidator_HasConfusableChars(t *testing.T) {
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
			hostname: "gооgle.cоm",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasConfusableChars(tt.hostname)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSSRFValidator_CanonicalRequestURL(t *testing.T) {
	validator := NewSSRFValidator()
	validator.SetTestingMode(true)

	u, err := url.Parse("https://example.com/a/b?x=1#frag")
	require.NoError(t, err)

	got, err := validator.CanonicalRequestURL(context.Background(), u)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/a/b?x=1", got)
	assert.NotContains(t, got, "#frag")

	bad, err := url.Parse("http://10.0.0.1/secret")
	require.NoError(t, err)
	_, err = validator.CanonicalRequestURL(context.Background(), bad)
	require.Error(t, err)
}

func TestCanonicalRequestURLRe(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "https with path and query", raw: "https://example.com/a/b?x=1", want: true},
		{name: "http with path", raw: "http://example.com/article", want: true},
		{name: "public IPv4", raw: "https://93.184.216.34/article", want: true},
		{name: "query may contain at-sign", raw: "https://example.com/a?email=user@host.com", want: true},
		{name: "IPv6 host", raw: "https://[2001:db8::1]/", want: true},
		{name: "userinfo rejected", raw: "https://user@example.com/foo", want: false},
		{name: "javascript scheme", raw: "javascript:alert(1)", want: false},
		{name: "ftp scheme", raw: "ftp://example.com/file", want: false},
		{name: "empty", raw: "", want: false},
		{name: "scheme relative", raw: "//example.com/foo", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, canonicalRequestURLRe.MatchString(tt.raw))
		})
	}
}

func TestSSRFValidator_CanonicalRequestURL_AllowlistedSchemeOnly(t *testing.T) {
	validator := NewSSRFValidator()
	validator.SetTestingMode(true)

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "https reconstructed without fragment or userinfo",
			raw:  "https://example.com/a/b?x=1#frag",
			want: "https://example.com/a/b?x=1",
		},
		{
			name: "http allowlisted scheme",
			raw:  "http://example.com/article",
			want: "http://example.com/article",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.raw)
			require.NoError(t, err)

			got, err := validator.CanonicalRequestURL(context.Background(), u)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.True(t, canonicalRequestURLRe.MatchString(got))
			assert.True(t, strings.HasPrefix(got, "https://") || strings.HasPrefix(got, "http://"))
			assert.NotContains(t, strings.SplitN(got, "/", 3)[2], "@")
		})
	}

	userinfo, err := url.Parse("https://user:pass@example.com/secret")
	require.NoError(t, err)
	_, err = validator.CanonicalRequestURL(context.Background(), userinfo)
	require.Error(t, err)
}

func TestSSRFValidator_CanonicalRequestURL_PreservesPercentEncodedPath(t *testing.T) {
	validator := NewSSRFValidator()
	validator.SetTestingMode(true)

	tests := []struct {
		name string
		raw  string
	}{
		{
			"Cloudinary transformation path with encoded commas and double-encoded text",
			"https://example.com/zenn/image/upload/s--sig--/c_fit%2Cg_north_west%2Cl_text:font_55:%25E9%259A%259C%2Cw_1010/v1/base.png?_a=X",
		},
		{
			"encoded slashes and colons in nested image URL",
			"https://example.com/image/scale/abc/https%3A%2F%2Fexample.com%2Fimg%2Fphoto.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.raw)
			require.NoError(t, err)

			got, err := validator.CanonicalRequestURL(context.Background(), u)
			require.NoError(t, err)
			assert.Equal(t, tt.raw, got)
		})
	}
}

func TestIsMetadataHost_ConsolidatedUnion(t *testing.T) {
	tests := []struct {
		host     string
		expected bool
	}{
		{"169.254.169.254", true},
		{"metadata.google.internal", true},
		{"100.100.100.200", true},
		{"192.0.0.192", true},
		{"169.254.169.254:80", true},
		{"169.254.169.254:8080", true},
		{"100.100.100.200:80", true},
		{"METADATA.GOOGLE.INTERNAL", true},
		{"example.com", false},
		{"not-metadata.google.internal.example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsMetadataHost(tt.host))
		})
	}
}

func TestHasInternalDomainSuffix(t *testing.T) {
	suffixes := []string{".local", ".internal", ".corp"}

	assert.True(t, hasInternalDomainSuffix("server.local", suffixes))
	assert.True(t, hasInternalDomainSuffix("db.INTERNAL", suffixes))
	assert.True(t, hasInternalDomainSuffix("host.corp", suffixes))
	assert.False(t, hasInternalDomainSuffix("example.com", suffixes))
}

func TestIsSuspiciousDomain(t *testing.T) {
	assert.True(t, isSuspiciousDomain("attack-rebind.nip.io"))
	assert.True(t, isSuspiciousDomain("test-toctou.com"))
	assert.True(t, isSuspiciousDomain("malicious.tk"))
	assert.False(t, isSuspiciousDomain("news.ycombinator.com"))
}

func BenchmarkSSRFValidator_ValidateURL(b *testing.B) {
	validator := NewSSRFValidator()
	u, _ := url.Parse("https://93.184.216.34/image.jpg")
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
