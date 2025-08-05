package utils

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPClientFactory_CreateHTTPClient_ProxyStrategy(t *testing.T) {
	tests := []struct {
		name                string
		proxyStrategyEnv    string
		envoyBaseURLEnv     string
		expectedStrategy    ProxyStrategy
		expectedTimeout     time.Duration
		expectDirectClient  bool
	}{
		{
			name:                "should_use_envoy_strategy_when_configured",
			proxyStrategyEnv:    "ENVOY",
			envoyBaseURLEnv:     "http://test-envoy:8080",
			expectedStrategy:    ProxyStrategyEnvoy,
			expectedTimeout:     60 * time.Second,
			expectDirectClient:  false,
		},
		{
			name:                "should_use_direct_strategy_when_configured",
			proxyStrategyEnv:    "DIRECT",
			envoyBaseURLEnv:     "",
			expectedStrategy:    ProxyStrategyDirect,
			expectedTimeout:     30 * time.Second,
			expectDirectClient:  true,
		},
		{
			name:                "should_default_to_direct_when_env_not_set",
			proxyStrategyEnv:    "",
			envoyBaseURLEnv:     "",
			expectedStrategy:    ProxyStrategyDirect,
			expectedTimeout:     30 * time.Second,
			expectDirectClient:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment variables
			os.Setenv("PROXY_STRATEGY", tt.proxyStrategyEnv)
			os.Setenv("ENVOY_PROXY_BASE_URL", tt.envoyBaseURLEnv)
			defer func() {
				os.Unsetenv("PROXY_STRATEGY")
				os.Unsetenv("ENVOY_PROXY_BASE_URL")
			}()

			// Create factory
			factory := NewHTTPClientFactory()

			// Verify factory configuration
			if factory.proxyStrategy != tt.expectedStrategy {
				t.Errorf("Expected proxy strategy %v, got %v", tt.expectedStrategy, factory.proxyStrategy)
			}

			// Create HTTP client
			client := factory.CreateHTTPClient()

			// Verify client is created
			if client == nil {
				t.Fatal("Expected HTTP client to be created, got nil")
			}

			// Verify timeout configuration
			if client.Timeout != tt.expectedTimeout {
				t.Errorf("Expected timeout %v, got %v", tt.expectedTimeout, client.Timeout)
			}

			// Verify transport configuration
			if client.Transport == nil {
				t.Fatal("Expected transport to be configured, got nil")
			}
		})
	}
}

func TestHTTPClientFactory_UnifiedStrategy_AllComponents(t *testing.T) {
	// Test that all components use the same HTTP client strategy
	tests := []struct {
		name             string
		proxyStrategy    string
		expectedStrategy ProxyStrategy
	}{
		{
			name:             "gateway_and_job_use_same_envoy_strategy",
			proxyStrategy:    "ENVOY",
			expectedStrategy: ProxyStrategyEnvoy,
		},
		{
			name:             "gateway_and_job_use_same_direct_strategy",
			proxyStrategy:    "DIRECT",
			expectedStrategy: ProxyStrategyDirect,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment
			os.Setenv("PROXY_STRATEGY", tt.proxyStrategy)
			defer os.Unsetenv("PROXY_STRATEGY")

			// Create multiple factories (simulating different components)
			gatewayFactory := NewHTTPClientFactory()
			jobFactory := NewHTTPClientFactory()

			// Verify both factories have same strategy
			if gatewayFactory.proxyStrategy != tt.expectedStrategy {
				t.Errorf("Gateway factory strategy mismatch: expected %v, got %v",
					tt.expectedStrategy, gatewayFactory.proxyStrategy)
			}

			if jobFactory.proxyStrategy != tt.expectedStrategy {
				t.Errorf("Job factory strategy mismatch: expected %v, got %v",
					tt.expectedStrategy, jobFactory.proxyStrategy)
			}

			// Verify both create clients with same timeout
			gatewayClient := gatewayFactory.CreateHTTPClient()
			jobClient := jobFactory.CreateHTTPClient()

			if gatewayClient.Timeout != jobClient.Timeout {
				t.Errorf("Client timeout mismatch: gateway=%v, job=%v",
					gatewayClient.Timeout, jobClient.Timeout)
			}
		})
	}
}

func TestSecureHTTPClient_BackwardCompatibility(t *testing.T) {
	// Test that the deprecated SecureHTTPClient function still works
	client := SecureHTTPClient()

	if client == nil {
		t.Fatal("Expected SecureHTTPClient to return client, got nil")
	}

	if client.Transport == nil {
		t.Fatal("Expected transport to be configured, got nil")
	}

	// Should use factory internally
	if client.Timeout == 0 {
		t.Error("Expected timeout to be configured")
	}
}

// RED: Test for Envoy proxy request transformation - this should fail initially
func TestHTTPClientFactory_EnvoyProxyRequestTransformation(t *testing.T) {
	// Mock Envoy proxy server that expects transformed requests
	envoyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request follows Envoy Dynamic Forward Proxy format
		expectedPath := "/proxy/https://example.com/rss.xml"
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
		}

		// Verify X-Target-Domain header
		expectedDomain := "example.com"
		actualDomain := r.Header.Get("X-Target-Domain")
		if actualDomain != expectedDomain {
			t.Errorf("Expected X-Target-Domain header %s, got %s", expectedDomain, actualDomain)
		}

		// Return success response
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "RSS feed content")
	}))
	defer envoyServer.Close()

	// Parse the test server URL to get host and port
	serverURL, err := url.Parse(envoyServer.URL)
	require.NoError(t, err)

	// Setup environment for Envoy proxy strategy
	os.Setenv("PROXY_STRATEGY", "ENVOY")
	os.Setenv("ENVOY_PROXY_BASE_URL", envoyServer.URL)
	defer func() {
		os.Unsetenv("PROXY_STRATEGY")
		os.Unsetenv("ENVOY_PROXY_BASE_URL")
	}()

	// Create HTTP client factory
	factory := NewHTTPClientFactory()
	client := factory.CreateHTTPClient()

	// RED: This request should be transformed to go through Envoy proxy
	// Target URL: https://example.com/rss.xml
	// Should be transformed to: {envoyServer.URL}/proxy/https://example.com/rss.xml
	// With header: X-Target-Domain: example.com
	targetURL := "https://example.com/rss.xml"
	
	req, err := http.NewRequestWithContext(context.Background(), "GET", targetURL, nil)
	require.NoError(t, err)

	// Execute the request - this should fail initially because CreateHTTPClient 
	// doesn't transform requests to go through Envoy proxy
	resp, err := client.Do(req)
	
	// We expect this to succeed (transformation working)
	assert.NoError(t, err, "Expected request to succeed through Envoy proxy")
	assert.NotNil(t, resp, "Expected response to be non-nil")
	
	if resp != nil {
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected successful response")
		
		body, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		assert.Equal(t, "RSS feed content", string(body))
	} else {
		t.Log("Host was:", serverURL.Host)
		t.Fatal("Failed to get response - Envoy proxy transformation not working")
	}
}