package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"alt/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPClientFactory_CreateHTTPClient_ProxyStrategy(t *testing.T) {
	tests := []struct {
		name               string
		proxyStrategyEnv   string
		envoyBaseURLEnv    string
		expectedStrategy   ProxyStrategy
		expectedTimeout    time.Duration
		expectDirectClient bool
	}{
		{
			name:               "should_use_envoy_strategy_when_configured",
			proxyStrategyEnv:   "ENVOY",
			envoyBaseURLEnv:    "http://test-envoy:8080",
			expectedStrategy:   ProxyStrategyEnvoy,
			expectedTimeout:    60 * time.Second,
			expectDirectClient: false,
		},
		{
			name:               "should_use_direct_strategy_when_configured",
			proxyStrategyEnv:   "DIRECT",
			envoyBaseURLEnv:    "",
			expectedStrategy:   ProxyStrategyDirect,
			expectedTimeout:    30 * time.Second,
			expectDirectClient: true,
		},
		{
			name:               "should_default_to_direct_when_env_not_set",
			proxyStrategyEnv:   "",
			envoyBaseURLEnv:    "",
			expectedStrategy:   ProxyStrategyDirect,
			expectedTimeout:    30 * time.Second,
			expectDirectClient: true,
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
	// Setup environment for Envoy proxy strategy
	os.Setenv("PROXY_STRATEGY", "ENVOY")
	os.Setenv("ENVOY_PROXY_BASE_URL", "http://envoy.test:8080")
	defer func() {
		os.Unsetenv("PROXY_STRATEGY")
		os.Unsetenv("ENVOY_PROXY_BASE_URL")
	}()

	// Create HTTP client factory
	factory := NewHTTPClientFactory()
	client := factory.CreateHTTPClient()

	// Replace the underlying transport to avoid network access
	envoyTransport, ok := client.Transport.(*EnvoyProxyTransport)
	require.True(t, ok, "Expected EnvoyProxyTransport")
	envoyTransport.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
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

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("RSS feed content")),
			Header:     make(http.Header),
		}
		return resp, nil
	})

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
		defer func() {
			assert.NoError(t, resp.Body.Close())
		}()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected successful response")

		body, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		assert.Equal(t, "RSS feed content", string(body))
	} else {
		t.Fatal("Failed to get response - Envoy proxy transformation not working")
	}
}

// roundTripperFunc is a helper to stub http.RoundTripper in tests.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestSecureHTTPClient_PinnedDial_PreventsDNSRebinding(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok-from-server"))
	}))
	defer ts.Close()

	tsURL, err := url.Parse(ts.URL)
	require.NoError(t, err)
	tsHost, tsPort, err := net.SplitHostPort(tsURL.Host)
	require.NoError(t, err)

	// Custom domain that is allowlisted so loopback test server can be reached
	testDomain := "rebind.example.org"
	t.Setenv("FEED_ALLOWED_HOSTS", testDomain)

	resolveCount := 0
	fakeResolver := IPResolverFunc(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if host == testDomain {
			resolveCount++
			return []net.IPAddr{{IP: net.ParseIP(tsHost)}}, nil
		}
		return nil, errors.New("unknown host")
	})

	factory := NewHTTPClientFactory().WithResolver(fakeResolver)
	client := factory.CreateHTTPClient()

	req, err := http.NewRequestWithContext(context.Background(), "GET", fmt.Sprintf("http://%s:%s/feed.xml", testDomain, tsPort), nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, resp.Body.Close())
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "ok-from-server", string(body))
	assert.Equal(t, 1, resolveCount, "resolver must be called exactly once per dial")
}

func TestSecureHTTPClient_PrivateIPRejected_GenericErrorDoesNotLeakIP(t *testing.T) {
	fakeResolver := IPResolverFunc(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}}, nil
	})

	factory := NewHTTPClientFactory().WithResolver(fakeResolver)
	client := factory.CreateHTTPClient()

	req, err := http.NewRequestWithContext(context.Background(), "GET", "http://internal.example.org/feed.xml", nil)
	require.NoError(t, err)

	_, err = client.Do(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "destination not allowed")
	assert.NotContains(t, err.Error(), "10.0.0.1", "generic error must not leak probed IP")
}

func TestSecureHTTPClient_AllowlistedHostBypass(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("trusted-content"))
	}))
	defer ts.Close()

	tsURL, err := url.Parse(ts.URL)
	require.NoError(t, err)
	_, tsPort, err := net.SplitHostPort(tsURL.Host)
	require.NoError(t, err)

	trustedHost := "trusted.internal.local"
	t.Setenv("FEED_ALLOWED_HOSTS", trustedHost)

	fakeResolver := IPResolverFunc(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})

	factory := NewHTTPClientFactory().WithResolver(fakeResolver)
	client := factory.CreateHTTPClient()

	req, err := http.NewRequestWithContext(context.Background(), "GET", fmt.Sprintf("http://%s:%s/feed.xml", trustedHost, tsPort), nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, resp.Body.Close())
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "trusted-content", string(body))
}

func TestSecureHTTPClient_RedirectToPrivateTargetBlockedOnSecondHop(t *testing.T) {
	// Hop 1: Public server redirects to private destination
	hop1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://private-target.internal/secret", http.StatusFound)
	}))
	defer hop1.Close()

	hop1URL, err := url.Parse(hop1.URL)
	require.NoError(t, err)
	hop1Host, hop1Port, err := net.SplitHostPort(hop1URL.Host)
	require.NoError(t, err)

	publicDomain := "public-hop1.example.org"
	t.Setenv("FEED_ALLOWED_HOSTS", publicDomain)

	fakeResolver := IPResolverFunc(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if host == publicDomain {
			return []net.IPAddr{{IP: net.ParseIP(hop1Host)}}, nil
		}
		if host == "private-target.internal" {
			return []net.IPAddr{{IP: net.ParseIP("192.168.1.50")}}, nil
		}
		return nil, errors.New("unknown host")
	})

	factory := NewHTTPClientFactory().WithResolver(fakeResolver)
	client := factory.CreateHTTPClient()

	req, err := http.NewRequestWithContext(context.Background(), "GET", fmt.Sprintf("http://%s:%s/feed.xml", publicDomain, hop1Port), nil)
	require.NoError(t, err)

	_, err = client.Do(req)
	require.Error(t, err, "redirect to private target must be rejected")
	assert.Contains(t, err.Error(), "destination not allowed")
	assert.NotContains(t, err.Error(), "192.168.1.50", "error must not leak private IP")
}

func TestSecureHTTPClient_TLSSNIPreserved(t *testing.T) {
	receivedSNI := ""
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil {
			receivedSNI = r.TLS.ServerName
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("tls-ok"))
	}))
	defer ts.Close()

	tsURL, err := url.Parse(ts.URL)
	require.NoError(t, err)
	tsHost, tsPort, err := net.SplitHostPort(tsURL.Host)
	require.NoError(t, err)

	secureHost := "feed.example.com"
	t.Setenv("FEED_ALLOWED_HOSTS", secureHost)

	fakeResolver := IPResolverFunc(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if host == secureHost {
			return []net.IPAddr{{IP: net.ParseIP(tsHost)}}, nil
		}
		return nil, errors.New("unknown host")
	})

	factory := NewHTTPClientFactory().WithResolver(fakeResolver)
	client := factory.CreateHTTPClient()

	// Use test server's cert for verification
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	transport.TLSClientConfig = ts.Client().Transport.(*http.Transport).TLSClientConfig

	req, err := http.NewRequestWithContext(context.Background(), "GET", fmt.Sprintf("https://%s:%s/feed.xml", secureHost, tsPort), nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, resp.Body.Close())
	}()

	assert.Equal(t, secureHost, receivedSNI, "TLS SNI must be preserved as the original hostname")
}

func TestHTTPClientFactory_SidecarProxyRequestTransformation(t *testing.T) {
	t.Setenv("PROXY_STRATEGY", "SIDECAR")
	t.Setenv("SIDECAR_PROXY_BASE_URL", "http://sidecar.test:8085")

	factory := NewHTTPClientFactory()
	client := factory.CreateHTTPClient()

	sidecarTransport, ok := client.Transport.(*SidecarProxyTransport)
	require.True(t, ok, "Expected SidecarProxyTransport")

	sidecarTransport.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		expectedPath := "/proxy/https://example.com/rss.xml"
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("sidecar content")),
			Header:     make(http.Header),
		}, nil
	})

	req, err := http.NewRequestWithContext(context.Background(), "GET", "https://example.com/rss.xml", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, resp.Body.Close())
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "sidecar content", string(body))
}

func TestSecureHTTPClient_ResolverFailure_ReturnsDistinctErrorWithoutLeakingIP(t *testing.T) {
	testHost := "lookup-fail.example.org"
	fakeResolver := IPResolverFunc(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return nil, errors.New("no such host")
	})

	client := NewHTTPClientFactory().WithResolver(fakeResolver).CreateHTTPClient()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+testHost+"/feed.xml", nil)
	require.NoError(t, err)

	_, err = client.Do(req)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "destination not allowed", "DNS failure must not look like security block")
	assert.Contains(t, err.Error(), testHost, "error must include hostname")
	assert.True(t, errors.Is(err, ErrLookupFailed), "must wrap ErrLookupFailed")
}

func TestSecureHTTPClient_ConnectFailure_WrapsErrorWithoutLeakingIP(t *testing.T) {
	testHost := "connect-fail.example.org"
	testNetIP := "192.0.2.1" // RFC 5737 TEST-NET-1 (non-routable public documentation address)

	fakeResolver := IPResolverFunc(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP(testNetIP)}}, nil
	})

	// Use short dial timeout
	cfg := &config.HTTPConfig{
		ClientTimeout: 200 * time.Millisecond,
		DialTimeout:   50 * time.Millisecond,
	}
	client := SecureHTTPClientWithConfigAndResolver(cfg, fakeResolver)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+testHost+"/feed.xml", nil)
	require.NoError(t, err)

	_, err = client.Do(req)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), testNetIP, "connect failure must not leak pinned IP in error string")
	assert.Contains(t, err.Error(), testHost, "connect failure should mention target hostname")
}

func TestSecureHTTPClient_PartialDeadline_TriesNextAddressOnFirstUnreachable(t *testing.T) {
	// Create a real working local test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	tsURL, err := url.Parse(ts.URL)
	require.NoError(t, err)
	tsHost, tsPort, err := net.SplitHostPort(tsURL.Host)
	require.NoError(t, err)
	workingIP := net.ParseIP(tsHost)
	require.NotNil(t, workingIP)

	testHost := "dual-stack.example.org"
	t.Setenv("FEED_ALLOWED_HOSTS", testHost) // Allow host so both addresses proceed to dial

	// Unreachable IP that will time out / blackhole
	unreachableIP := net.ParseIP("192.0.2.1") // RFC 5737 TEST-NET-1

	fakeResolver := IPResolverFunc(func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if host == testHost {
			return []net.IPAddr{
				{IP: unreachableIP},
				{IP: workingIP},
			}, nil
		}
		return nil, errors.New("unknown host")
	})

	// Total client timeout 600ms, DialTimeout 600ms.
	// Without deadline splitting, unreachable IP consumes all 600ms and working IP is never reached.
	// With deadline splitting, unreachable IP gets 300ms, fails, and working IP connects within the remaining 300ms.
	cfg := &config.HTTPConfig{
		ClientTimeout: 600 * time.Millisecond,
		DialTimeout:   600 * time.Millisecond,
	}
	client := SecureHTTPClientWithConfigAndResolver(cfg, fakeResolver)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+testHost+":"+tsPort+"/feed.xml", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err, "request should succeed via second address within budget")
	defer func() {
		assert.NoError(t, resp.Body.Close())
	}()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
