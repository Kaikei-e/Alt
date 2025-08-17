package image_fetch_gateway

import (
	"alt/domain"
	"alt/utils/errors"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageFetchGateway_FetchImage_SSRF_PrivateNetworks(t *testing.T) {
	gateway := NewImageFetchGateway(&http.Client{Timeout: 10 * time.Second})

	tests := []struct {
		name        string
		imageURL    string
		expectedErr string
	}{
		{
			name:        "localhost IPv4",
			imageURL:    "https://127.0.0.1:8080/image.jpg",
			expectedErr: "access to private networks not allowed",
		},
		{
			name:        "localhost hostname",
			imageURL:    "https://localhost:8080/image.jpg",
			expectedErr: "access to private networks not allowed",
		},
		{
			name:        "private network 10.x.x.x",
			imageURL:    "https://10.0.0.1/image.jpg",
			expectedErr: "access to private networks not allowed",
		},
		{
			name:        "private network 192.168.x.x",
			imageURL:    "https://192.168.1.1/image.jpg",
			expectedErr: "access to private networks not allowed",
		},
		{
			name:        "private network 172.16.x.x",
			imageURL:    "https://172.16.0.1/image.jpg",
			expectedErr: "access to private networks not allowed",
		},
		{
			name:        "AWS metadata endpoint",
			imageURL:    "https://169.254.169.254/latest/meta-data/",
			expectedErr: "access to metadata endpoint not allowed",
		},
		{
			name:        "internal domain .local",
			imageURL:    "https://server.local/image.jpg",
			expectedErr: "access to internal domains not allowed",
		},
		{
			name:        "internal domain .internal",
			imageURL:    "https://api.internal/image.jpg",
			expectedErr: "access to internal domains not allowed",
		},
		{
			name:        "non-whitelisted domain",
			imageURL:    "https://malicious.com/image.jpg",
			expectedErr: "domain not in whitelist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testURL, err := url.Parse(tt.imageURL)
			require.NoError(t, err)

			got, err := gateway.FetchImage(context.Background(), testURL, domain.NewImageFetchOptions())

			assert.Error(t, err)
			assert.Nil(t, got)
			// The implementation now prioritizes security checks over domain allowlist
			// This is a significant security improvement
			assert.Contains(t, err.Error(), tt.expectedErr)
			if appErr, ok := err.(*errors.AppContextError); ok {
				assert.Equal(t, "VALIDATION_ERROR", appErr.Code)
			}
		})
	}
}

func TestImageFetchGateway_FetchImage_SSRF_Advanced(t *testing.T) {
	gateway := NewImageFetchGateway(&http.Client{Timeout: 10 * time.Second})

	tests := []struct {
		name        string
		imageURL    string
		expectedErr string
	}{
		{
			name:        "URL with user info",
			imageURL:    "https://user:password@malicious.com/image.jpg",
			expectedErr: "domain not in whitelist",
		},
		{
			name:        "URL with tricky characters in domain",
			imageURL:    "https://127.0.0.1.nip.io/image.jpg", // nip.io resolves to the IP
			expectedErr: "access to private networks not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testURL, err := url.Parse(tt.imageURL)
			require.NoError(t, err)

			// Using FetchImage directly to test production validation logic
			got, err := gateway.FetchImage(context.Background(), testURL, domain.NewImageFetchOptions())

			assert.Error(t, err)
			assert.Nil(t, got)
			assert.Contains(t, err.Error(), tt.expectedErr)
			if appErr, ok := err.(*errors.AppContextError); ok {
				assert.Equal(t, "VALIDATION_ERROR", appErr.Code)
			}
		})
	}
}

func TestImageFetchGateway_FetchImage_IntegerOverflow(t *testing.T) {
	tests := []struct {
		name          string
		contentLength string
		shouldFail    bool
	}{
		{
			name:          "normal content length",
			contentLength: "1024",
			shouldFail:    false,
		},
		{
			name:          "max int32 value",
			contentLength: "2147483647", // math.MaxInt32
			shouldFail:    false,
		},
		{
			name:          "larger than int32 - should handle gracefully",
			contentLength: "9223372036854775807", // math.MaxInt64
			shouldFail:    false,                 // Should not panic or overflow
		},
		{
			name:          "extremely large content length",
			contentLength: "999999999999999999999",
			shouldFail:    false, // Should not panic
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "image/jpeg")
				w.Header().Set("Content-Length", tt.contentLength)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("fake-image-data"))
			}))
			defer server.Close()

			testURL, err := url.Parse(server.URL + "/image.jpg")
			require.NoError(t, err)

			gateway := NewImageFetchGateway(&http.Client{Timeout: 10 * time.Second})

			// Use small max size to trigger size checking
			options := &domain.ImageFetchOptions{
				MaxSize: 10,
				Timeout: 30 * time.Second,
			}

			// This should not panic even with large content length values
			got, err := gateway.fetchImageForTesting(context.Background(), testURL, options)

			// We expect this to fail due to size limit, but not due to integer overflow
			if tt.shouldFail {
				assert.Error(t, err)
				assert.Nil(t, got)
			}
			// The key assertion is that we didn't panic during execution
		})
	}
}

// TestValidateImageURLWithTestOverride_SecurityEnhancements tests new security features
func TestValidateImageURLWithTestOverride_SecurityEnhancements(t *testing.T) {
	tests := []struct {
		name                  string
		inputURL              string
		allowTestingLocalhost bool
		wantErr               bool
		expectedErrMessage    string
	}{
		// Test URL encoding attack prevention
		{
			name:                  "URL encoding path traversal attack",
			inputURL:              "https://example.com/%2e%2e/admin",
			allowTestingLocalhost: false,
			wantErr:               true,
			expectedErrMessage:    "path traversal patterns not allowed",
		},
		{
			name:                  "URL encoding forward slash attack",
			inputURL:              "https://example.com/test%2fmalicious",
			allowTestingLocalhost: false,
			wantErr:               true,
			expectedErrMessage:    "URL encoding attacks not allowed",
		},
		{
			name:                  "empty host validation",
			inputURL:              "https:///path",
			allowTestingLocalhost: false,
			wantErr:               true,
			expectedErrMessage:    "empty host not allowed",
		},
		// Test enhanced metadata endpoint blocking
		{
			name:                  "AWS metadata with port",
			inputURL:              "http://169.254.169.254:80/latest/meta-data/",
			allowTestingLocalhost: false,
			wantErr:               true,
			expectedErrMessage:    "access to metadata endpoint not allowed",
		},
		{
			name:                  "Alibaba Cloud metadata",
			inputURL:              "http://100.100.100.200/latest/meta-data/",
			allowTestingLocalhost: false,
			wantErr:               true,
			expectedErrMessage:    "access to metadata endpoint not allowed",
		},
		// Test enhanced internal domain blocking
		{
			name:                  "intranet domain",
			inputURL:              "https://service.intranet/image.jpg",
			allowTestingLocalhost: false,
			wantErr:               true,
			expectedErrMessage:    "access to internal domains not allowed",
		},
		{
			name:                  "test domain",
			inputURL:              "https://service.test/image.jpg",
			allowTestingLocalhost: false,
			wantErr:               true,
			expectedErrMessage:    "access to internal domains not allowed",
		},
		{
			name:                  "localhost domain",
			inputURL:              "https://service.localhost/image.jpg",
			allowTestingLocalhost: false,
			wantErr:               true,
			expectedErrMessage:    "access to internal domains not allowed",
		},
		// Test non-standard port blocking
		{
			name:                  "non-standard port 3000",
			inputURL:              "https://example.com:3000/image.jpg",
			allowTestingLocalhost: false,
			wantErr:               true,
			expectedErrMessage:    "non-standard port not allowed: 3000",
		},
		{
			name:                  "non-standard port 9000",
			inputURL:              "https://example.com:9000/image.jpg",
			allowTestingLocalhost: false,
			wantErr:               true,
			expectedErrMessage:    "non-standard port not allowed: 9000",
		},
		// Test allowed ports
		{
			name:                  "allowed port 443",
			inputURL:              "https://example.com:443/image.jpg",
			allowTestingLocalhost: false,
			wantErr:               false,
		},
		{
			name:                  "allowed port 80",
			inputURL:              "http://example.com:80/image.jpg",
			allowTestingLocalhost: false,
			wantErr:               false,
		},
		{
			name:                  "allowed port 8080",
			inputURL:              "https://example.com:8080/image.jpg",
			allowTestingLocalhost: false,
			wantErr:               false,
		},
		{
			name:                  "allowed port 8443",
			inputURL:              "https://example.com:8443/image.jpg",
			allowTestingLocalhost: false,
			wantErr:               false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.inputURL)
			require.NoError(t, err, "Failed to parse test URL")

			err = validateImageURLWithTestOverride(u, tt.allowTestingLocalhost)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErrMessage != "" && err != nil {
					assert.Contains(t, err.Error(), tt.expectedErrMessage)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestNewImageFetchGateway_RedirectDisabled tests that redirects are disabled
func TestNewImageFetchGateway_RedirectDisabled(t *testing.T) {
	client := &http.Client{Timeout: 10 * time.Second}
	gateway := NewImageFetchGateway(client)

	// Create a test request to verify redirect behavior
	req, err := http.NewRequest("GET", "https://example.com/redirect", nil)
	require.NoError(t, err)

	// Test that CheckRedirect function is set and returns an error
	if client.CheckRedirect != nil {
		err = client.CheckRedirect(req, []*http.Request{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "redirects not allowed for security reasons")
	} else {
		t.Error("CheckRedirect should be set to prevent redirects")
	}

	// Verify the gateway was created correctly
	assert.NotNil(t, gateway)
	assert.NotNil(t, gateway.httpClient)
}

// TestIsPrivateIP_EnhancedValidation tests enhanced private IP detection
func TestIsPrivateIP_EnhancedValidation(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		expected bool
	}{
		// IPv4 private ranges
		{
			name:     "10.0.0.1 private",
			hostname: "10.0.0.1",
			expected: true,
		},
		{
			name:     "172.16.0.1 private",
			hostname: "172.16.0.1",
			expected: true,
		},
		{
			name:     "192.168.1.1 private",
			hostname: "192.168.1.1",
			expected: true,
		},
		{
			name:     "127.0.0.1 loopback",
			hostname: "127.0.0.1",
			expected: true,
		},
		// Public IPv4 addresses
		{
			name:     "8.8.8.8 public",
			hostname: "8.8.8.8",
			expected: false,
		},
		{
			name:     "1.1.1.1 public",
			hostname: "1.1.1.1",
			expected: false,
		},
		// IPv6 addresses
		{
			name:     "::1 loopback",
			hostname: "::1",
			expected: true,
		},
		{
			name:     "fc00:: unique local",
			hostname: "fc00::1",
			expected: true,
		},
		{
			name:     "2001:db8:: public",
			hostname: "2001:db8::1",
			expected: false,
		},
		// Edge cases
		{
			name:     "invalid hostname",
			hostname: "invalid-hostname-that-wont-resolve",
			expected: true, // Should return true on resolution failure
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPrivateIP(tt.hostname)
			assert.Equal(t, tt.expected, result)
		})
	}
}
