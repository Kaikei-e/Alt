package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyMode_String(t *testing.T) {
	tests := []struct {
		name     string
		mode     Mode
		expected string
	}{
		{"sidecar mode", ModeSidecar, "sidecar"},
		{"envoy mode", ModeEnvoy, "envoy"},
		{"nginx mode", ModeNginx, "nginx"},
		{"disabled mode", ModeDisabled, "disabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.mode))
		})
	}
}

func TestGetStrategy_Priority(t *testing.T) {
	// Save original env vars and restore after test
	origSidecar := os.Getenv("SIDECAR_PROXY_ENABLED")
	origSidecarURL := os.Getenv("SIDECAR_PROXY_URL")
	origEnvoy := os.Getenv("ENVOY_PROXY_ENABLED")
	origEnvoyURL := os.Getenv("ENVOY_PROXY_URL")
	origNginx := os.Getenv("NGINX_PROXY_ENABLED")
	origNginxURL := os.Getenv("NGINX_PROXY_URL")

	defer func() {
		os.Setenv("SIDECAR_PROXY_ENABLED", origSidecar)
		os.Setenv("SIDECAR_PROXY_URL", origSidecarURL)
		os.Setenv("ENVOY_PROXY_ENABLED", origEnvoy)
		os.Setenv("ENVOY_PROXY_URL", origEnvoyURL)
		os.Setenv("NGINX_PROXY_ENABLED", origNginx)
		os.Setenv("NGINX_PROXY_URL", origNginxURL)
	}()

	tests := []struct {
		name             string
		sidecarEnabled   string
		sidecarURL       string
		envoyEnabled     string
		envoyURL         string
		nginxEnabled     string
		nginxURL         string
		expectedMode     Mode
		expectedEnabled  bool
		expectedBaseURL  string
		expectedTemplate string
	}{
		{
			name:             "all disabled returns disabled mode",
			sidecarEnabled:   "false",
			envoyEnabled:     "false",
			nginxEnabled:     "false",
			expectedMode:     ModeDisabled,
			expectedEnabled:  false,
			expectedBaseURL:  "",
			expectedTemplate: "",
		},
		{
			name:             "sidecar takes priority over envoy and nginx",
			sidecarEnabled:   "true",
			sidecarURL:       "http://sidecar:8085",
			envoyEnabled:     "true",
			envoyURL:         "http://envoy:8080",
			nginxEnabled:     "true",
			nginxURL:         "http://nginx:8889",
			expectedMode:     ModeSidecar,
			expectedEnabled:  true,
			expectedBaseURL:  "http://sidecar:8085",
			expectedTemplate: "/proxy/{scheme}://{host}{path}",
		},
		{
			name:             "envoy takes priority over nginx when sidecar disabled",
			sidecarEnabled:   "false",
			envoyEnabled:     "true",
			envoyURL:         "http://envoy:8080",
			nginxEnabled:     "true",
			nginxURL:         "http://nginx:8889",
			expectedMode:     ModeEnvoy,
			expectedEnabled:  true,
			expectedBaseURL:  "http://envoy:8080",
			expectedTemplate: "/proxy/{scheme}://{host}{path}",
		},
		{
			name:             "nginx mode when sidecar and envoy disabled",
			sidecarEnabled:   "false",
			envoyEnabled:     "false",
			nginxEnabled:     "true",
			nginxURL:         "http://nginx:8889",
			expectedMode:     ModeNginx,
			expectedEnabled:  true,
			expectedBaseURL:  "http://nginx:8889",
			expectedTemplate: "/rss-proxy/{scheme}://{host}{path}",
		},
		{
			name:             "sidecar with default URL",
			sidecarEnabled:   "true",
			sidecarURL:       "",
			expectedMode:     ModeSidecar,
			expectedEnabled:  true,
			expectedBaseURL:  DefaultSidecarProxyURL,
			expectedTemplate: "/proxy/{scheme}://{host}{path}",
		},
		{
			name:             "envoy with default URL",
			sidecarEnabled:   "false",
			envoyEnabled:     "true",
			envoyURL:         "",
			expectedMode:     ModeEnvoy,
			expectedEnabled:  true,
			expectedBaseURL:  DefaultEnvoyProxyURL,
			expectedTemplate: "/proxy/{scheme}://{host}{path}",
		},
		{
			name:             "nginx with default URL",
			sidecarEnabled:   "false",
			envoyEnabled:     "false",
			nginxEnabled:     "true",
			nginxURL:         "",
			expectedMode:     ModeNginx,
			expectedEnabled:  true,
			expectedBaseURL:  DefaultNginxProxyURL,
			expectedTemplate: "/rss-proxy/{scheme}://{host}{path}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all env vars first
			os.Unsetenv("SIDECAR_PROXY_ENABLED")
			os.Unsetenv("SIDECAR_PROXY_URL")
			os.Unsetenv("ENVOY_PROXY_ENABLED")
			os.Unsetenv("ENVOY_PROXY_URL")
			os.Unsetenv("NGINX_PROXY_ENABLED")
			os.Unsetenv("NGINX_PROXY_URL")

			// Set test-specific env vars
			if tt.sidecarEnabled != "" {
				os.Setenv("SIDECAR_PROXY_ENABLED", tt.sidecarEnabled)
			}
			if tt.sidecarURL != "" {
				os.Setenv("SIDECAR_PROXY_URL", tt.sidecarURL)
			}
			if tt.envoyEnabled != "" {
				os.Setenv("ENVOY_PROXY_ENABLED", tt.envoyEnabled)
			}
			if tt.envoyURL != "" {
				os.Setenv("ENVOY_PROXY_URL", tt.envoyURL)
			}
			if tt.nginxEnabled != "" {
				os.Setenv("NGINX_PROXY_ENABLED", tt.nginxEnabled)
			}
			if tt.nginxURL != "" {
				os.Setenv("NGINX_PROXY_URL", tt.nginxURL)
			}

			strategy := GetStrategy()

			assert.Equal(t, tt.expectedMode, strategy.Mode, "Mode mismatch")
			assert.Equal(t, tt.expectedEnabled, strategy.Enabled, "Enabled mismatch")
			assert.Equal(t, tt.expectedBaseURL, strategy.BaseURL, "BaseURL mismatch")
			assert.Equal(t, tt.expectedTemplate, strategy.PathTemplate, "PathTemplate mismatch")
		})
	}
}

func TestConvertToProxyURL_Basic(t *testing.T) {
	tests := []struct {
		name        string
		originalURL string
		strategy    *Strategy
		expected    string
	}{
		{
			name:        "nil strategy returns original URL",
			originalURL: "https://example.com/feed.xml",
			strategy:    nil,
			expected:    "https://example.com/feed.xml",
		},
		{
			name:        "disabled strategy returns original URL",
			originalURL: "https://example.com/feed.xml",
			strategy: &Strategy{
				Mode:    ModeDisabled,
				Enabled: false,
			},
			expected: "https://example.com/feed.xml",
		},
		{
			name:        "sidecar mode converts URL correctly",
			originalURL: "https://example.com/feed.xml",
			strategy: &Strategy{
				Mode:         ModeSidecar,
				BaseURL:      "http://sidecar:8085",
				PathTemplate: "/proxy/{scheme}://{host}{path}",
				Enabled:      true,
			},
			expected: "http://sidecar:8085/proxy/https://example.com/feed.xml",
		},
		{
			name:        "envoy mode converts URL correctly",
			originalURL: "https://zenn.dev/topics/typescript/feed",
			strategy: &Strategy{
				Mode:         ModeEnvoy,
				BaseURL:      "http://envoy:8080",
				PathTemplate: "/proxy/{scheme}://{host}{path}",
				Enabled:      true,
			},
			expected: "http://envoy:8080/proxy/https://zenn.dev/topics/typescript/feed",
		},
		{
			name:        "preserves query parameters",
			originalURL: "https://example.com/feed.xml?page=1&limit=10",
			strategy: &Strategy{
				Mode:         ModeSidecar,
				BaseURL:      "http://sidecar:8085",
				PathTemplate: "/proxy/{scheme}://{host}{path}",
				Enabled:      true,
			},
			expected: "http://sidecar:8085/proxy/https://example.com/feed.xml?page=1&limit=10",
		},
		{
			name:        "handles HTTP URLs",
			originalURL: "http://example.com/feed.xml",
			strategy: &Strategy{
				Mode:         ModeSidecar,
				BaseURL:      "http://sidecar:8085",
				PathTemplate: "/proxy/{scheme}://{host}{path}",
				Enabled:      true,
			},
			expected: "http://sidecar:8085/proxy/http://example.com/feed.xml",
		},
		{
			name:        "nginx mode uses rss-proxy path",
			originalURL: "https://example.com/feed.xml",
			strategy: &Strategy{
				Mode:         ModeNginx,
				BaseURL:      "http://nginx:8889",
				PathTemplate: "/rss-proxy/{scheme}://{host}{path}",
				Enabled:      true,
			},
			expected: "http://nginx:8889/rss-proxy/https://example.com/feed.xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToProxyURL(tt.originalURL, tt.strategy)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertToProxyURL_SecurityValidation(t *testing.T) {
	strategy := &Strategy{
		Mode:         ModeSidecar,
		BaseURL:      "http://sidecar:8085",
		PathTemplate: "/proxy/{scheme}://{host}{path}",
		Enabled:      true,
	}

	tests := []struct {
		name        string
		originalURL string
		expectOrig  bool // true if we expect the original URL to be returned (invalid input)
	}{
		{
			name:        "invalid URL returns original",
			originalURL: "not-a-valid-url",
			expectOrig:  true,
		},
		{
			name:        "empty scheme returns original",
			originalURL: "://example.com/feed.xml",
			expectOrig:  true,
		},
		{
			name:        "empty host returns original",
			originalURL: "https:///feed.xml",
			expectOrig:  true,
		},
		{
			name:        "valid URL is converted",
			originalURL: "https://example.com/feed.xml",
			expectOrig:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToProxyURL(tt.originalURL, strategy)
			if tt.expectOrig {
				assert.Equal(t, tt.originalURL, result, "Should return original URL for invalid input")
			} else {
				assert.NotEqual(t, tt.originalURL, result, "Should convert valid URL")
				assert.Contains(t, result, "sidecar:8085/proxy/", "Should contain proxy path")
			}
		})
	}
}

func TestConvertToProxyURL_InvalidBaseURL(t *testing.T) {
	strategy := &Strategy{
		Mode:         ModeSidecar,
		BaseURL:      "://invalid-base-url",
		PathTemplate: "/proxy/{scheme}://{host}{path}",
		Enabled:      true,
	}

	originalURL := "https://example.com/feed.xml"
	result := ConvertToProxyURL(originalURL, strategy)

	// Should return original URL when base URL is invalid
	assert.Equal(t, originalURL, result)
}

func TestEnvoyProxyRoundTripper_HostHeaderFix(t *testing.T) {
	// Create a test server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	tests := []struct {
		name               string
		requestPath        string
		expectedHost       string
		expectedXTarget    string
		shouldModifyHeader bool
	}{
		{
			name:               "HTTPS proxy request sets Host header",
			requestPath:        "/proxy/https://zenn.dev/topics/typescript/feed",
			expectedHost:       "zenn.dev",
			expectedXTarget:    "zenn.dev",
			shouldModifyHeader: true,
		},
		{
			name:               "HTTP proxy request sets Host header",
			requestPath:        "/proxy/http://example.com/feed.xml",
			expectedHost:       "example.com",
			expectedXTarget:    "example.com",
			shouldModifyHeader: true,
		},
		{
			name:               "non-proxy request does not modify Host",
			requestPath:        "/api/feeds",
			expectedHost:       "",
			expectedXTarget:    "",
			shouldModifyHeader: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse test server URL
			testURL, err := url.Parse(testServer.URL)
			require.NoError(t, err)

			// Create round tripper with real transport
			rt := &EnvoyProxyRoundTripper{
				Transport: http.DefaultTransport,
			}

			// Create request
			reqURL := testServer.URL + tt.requestPath
			req, err := http.NewRequestWithContext(context.Background(), "GET", reqURL, nil)
			require.NoError(t, err)
			req.URL.Host = testURL.Host

			// Execute request
			resp, err := rt.RoundTrip(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			if tt.shouldModifyHeader {
				assert.Equal(t, tt.expectedHost, req.Host, "Host should be set")
				assert.Equal(t, tt.expectedXTarget, req.Header.Get("X-Target-Domain"), "X-Target-Domain should be set")
			}
		})
	}
}

func TestEnvoyProxyRoundTripper_ExtractTargetHost(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		expectedHost string
		shouldMatch  bool
	}{
		{
			name:         "HTTPS URL",
			path:         "/proxy/https://example.com/path",
			expectedHost: "example.com",
			shouldMatch:  true,
		},
		{
			name:         "HTTP URL",
			path:         "/proxy/http://example.com/path",
			expectedHost: "example.com",
			shouldMatch:  true,
		},
		{
			name:         "URL with port",
			path:         "/proxy/https://example.com:8443/path",
			expectedHost: "example.com:8443",
			shouldMatch:  true,
		},
		{
			name:         "non-proxy path",
			path:         "/api/v1/feeds",
			expectedHost: "",
			shouldMatch:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, ok := ExtractTargetHost(tt.path)
			assert.Equal(t, tt.shouldMatch, ok, "Match result mismatch")
			if tt.shouldMatch {
				assert.Equal(t, tt.expectedHost, host, "Host mismatch")
			}
		})
	}
}

func TestStrategy_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		strategy *Strategy
		expected bool
	}{
		{
			name:     "nil strategy is not enabled",
			strategy: nil,
			expected: false,
		},
		{
			name: "enabled strategy returns true",
			strategy: &Strategy{
				Mode:    ModeSidecar,
				Enabled: true,
			},
			expected: true,
		},
		{
			name: "disabled strategy returns false",
			strategy: &Strategy{
				Mode:    ModeDisabled,
				Enabled: false,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.strategy.IsEnabled()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultProxyURLConstants(t *testing.T) {
	// Verify default URLs are reasonable values
	assert.Contains(t, DefaultSidecarProxyURL, "8085", "Sidecar should use port 8085")
	assert.Contains(t, DefaultEnvoyProxyURL, "8080", "Envoy should use port 8080")
	assert.Contains(t, DefaultNginxProxyURL, "8889", "Nginx should use port 8889")

	// Verify they are HTTP URLs (internal cluster communication)
	assert.True(t, len(DefaultSidecarProxyURL) > 0, "Sidecar URL should not be empty")
	assert.True(t, len(DefaultEnvoyProxyURL) > 0, "Envoy URL should not be empty")
	assert.True(t, len(DefaultNginxProxyURL) > 0, "Nginx URL should not be empty")
}
