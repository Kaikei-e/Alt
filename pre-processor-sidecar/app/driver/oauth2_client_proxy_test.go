package driver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOAuth2Client_ProxyDisabled(t *testing.T) {
	// Create a buffer to capture log output
	var logOutput strings.Builder
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client := NewOAuth2Client("test_client_id", "test_client_secret", "https://test.example.com", logger)

	// Verify proxy is disabled in transport
	transport, ok := client.httpClient.Transport.(*http.Transport)
	require.True(t, ok, "Transport should be *http.Transport")
	assert.Nil(t, transport.Proxy, "Proxy should be explicitly disabled")

	// Verify log output contains proxy disabled message
	logText := logOutput.String()
	assert.Contains(t, logText, "OAuth2 client configured without proxy")
	assert.Contains(t, logText, "proxy_disabled=true")
}

func TestOAuth2Client_RefreshToken_WithoutProxy(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify this is the oauth2/token endpoint
		assert.Equal(t, "/oauth2/token", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		assert.Equal(t, "pre-processor-sidecar/1.0", r.Header.Get("User-Agent"))

		// Parse and verify form data
		err := r.ParseForm()
		require.NoError(t, err)

		assert.Equal(t, "refresh_token", r.Form.Get("grant_type"))
		assert.Equal(t, "test_refresh_token", r.Form.Get("refresh_token"))
		assert.Equal(t, "test_client_id", r.Form.Get("client_id"))
		assert.Equal(t, "test_client_secret", r.Form.Get("client_secret"))

		// Return successful token response
		response := map[string]interface{}{
			"access_token":  "new_access_token",
			"token_type":    "Bearer",
			"expires_in":    86400,
			"refresh_token": "new_refresh_token",
			"scope":         "read",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create a buffer to capture log output
	var logOutput strings.Builder
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client := NewOAuth2Client("test_client_id", "test_client_secret", server.URL, logger)

	// Verify transport has no proxy
	transport, ok := client.httpClient.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Nil(t, transport.Proxy, "Proxy should be disabled")

	// Execute token refresh
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := client.RefreshToken(ctx, "test_refresh_token")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify response
	assert.Equal(t, "new_access_token", result.AccessToken)
	assert.Equal(t, "Bearer", result.TokenType)
	assert.Equal(t, 86400, result.ExpiresIn)
	assert.Equal(t, "new_refresh_token", result.RefreshToken)

	// Verify debug logs
	logText := logOutput.String()
	assert.Contains(t, logText, "Executing OAuth2 token refresh")
	assert.Contains(t, logText, "proxy_disabled=true")
	assert.Contains(t, logText, server.URL+"/oauth2/token")
}

func TestOAuth2Client_RefreshToken_TimeoutWithoutProxy(t *testing.T) {
	// Create mock server that delays response beyond timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Sleep longer than client timeout
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"token"}`))
	}))
	defer server.Close()

	// Create a buffer to capture log output
	var logOutput strings.Builder
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client := NewOAuth2Client("test_client_id", "test_client_secret", server.URL, logger)

	// Set a short timeout for the client
	client.httpClient.Timeout = 500 * time.Millisecond

	// Execute token refresh with short context timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	result, err := client.RefreshToken(ctx, "test_refresh_token")

	// Should get timeout error
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to execute refresh token request")

	// Verify error logs contain proxy disabled info
	logText := logOutput.String()
	assert.Contains(t, logText, "OAuth2 token refresh request failed")
	assert.Contains(t, logText, "proxy_disabled=true")
}

func TestOAuth2Client_HTTPTransportConfiguration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&strings.Builder{}, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client := NewOAuth2Client("test_id", "test_secret", "https://test.example.com", logger)

	transport, ok := client.httpClient.Transport.(*http.Transport)
	require.True(t, ok)

	// Verify transport configuration
	assert.Nil(t, transport.Proxy, "Proxy should be explicitly disabled")
	assert.Equal(t, 15*time.Second, transport.TLSHandshakeTimeout)
	assert.Equal(t, 60*time.Second, transport.ResponseHeaderTimeout)
	assert.Equal(t, 90*time.Second, transport.IdleConnTimeout)
	assert.Equal(t, 10, transport.MaxIdleConns)
	assert.Equal(t, 2, transport.MaxIdleConnsPerHost)

	// Verify client timeout
	assert.Equal(t, 120*time.Second, client.httpClient.Timeout)
}

// TestOAuth2Client_WithProxyConfiguration tests proxy configuration with mock environment
func TestOAuth2Client_WithProxyConfiguration(t *testing.T) {
	// Create mock proxy server
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This simulates a proxy server - it should forward requests
		if r.Method == "CONNECT" {
			// Handle HTTPS CONNECT requests
			w.WriteHeader(http.StatusOK)
			return
		}
		// For HTTP requests, act as a transparent proxy
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"proxy_token","token_type":"Bearer","expires_in":3600}`))
	}))
	defer proxyServer.Close()

	// Create target server
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"access_token":  "direct_token",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": "new_refresh",
			"scope":         "read",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer targetServer.Close()

	// Test with proxy environment variable set
	t.Setenv("HTTPS_PROXY", proxyServer.URL)

	var logOutput strings.Builder
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create client that should respect proxy settings
	client := NewOAuth2ClientWithProxy("test_id", "test_secret", targetServer.URL, logger)

	// Verify proxy is configured
	transport, ok := client.httpClient.Transport.(*http.Transport)
	require.True(t, ok)
	assert.NotNil(t, transport.Proxy, "Proxy should be configured when HTTPS_PROXY is set")

	// Test token refresh through proxy (mocked)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := client.RefreshToken(ctx, "test_refresh_token")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify we got a response (proxy or direct depending on implementation)
	assert.NotEmpty(t, result.AccessToken)
	assert.Equal(t, "Bearer", result.TokenType)

	// Verify logs show proxy configuration
	logText := logOutput.String()
	assert.Contains(t, logText, "OAuth2 client configured with proxy")
}

// TestOAuth2Client_ProxyFallbackToDirect tests fallback to direct connection when proxy fails
func TestOAuth2Client_ProxyFallbackToDirect(t *testing.T) {
	// Create target server that works directly
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"access_token":  "direct_token",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": "new_refresh",
			"scope":         "read",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer targetServer.Close()

	var logOutput strings.Builder
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create client with fallback capability but point to wrong URL for primary
	// This will cause a "connection refused" error on primary client
	client := NewOAuth2ClientWithFallback("test_id", "test_secret", "http://127.0.0.1:1", logger)

	// Override fallback client to use the working server
	fallbackTransport := &http.Transport{
		Proxy:                 nil,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   2,
	}
	client.fallbackClient = &http.Client{
		Timeout:   60 * time.Second,
		Transport: fallbackTransport,
	}
	// Set the correct base URL for fallback to work
	client.baseURL = targetServer.URL

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// This should fail with primary and succeed with fallback
	result, err := client.RefreshToken(ctx, "test_refresh_token")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "direct_token", result.AccessToken)
	assert.Equal(t, "Bearer", result.TokenType)

	// Verify logs show fallback occurred
	logText := logOutput.String()
	assert.Contains(t, logText, "Primary connection failed, attempting fallback to direct connection")
	assert.Contains(t, logText, "Falling back to direct connection")
}

// TestOAuth2Client_TimeoutConfiguration tests different timeout scenarios with mocks
func TestOAuth2Client_TimeoutConfiguration(t *testing.T) {
	tests := []struct {
		name          string
		clientTimeout time.Duration
		serverDelay   time.Duration
		expectSuccess bool
		expectError   string
	}{
		{
			name:          "Fast_Response_Success",
			clientTimeout: 5 * time.Second,
			serverDelay:   100 * time.Millisecond,
			expectSuccess: true,
		},
		{
			name:          "Timeout_Failure",
			clientTimeout: 1 * time.Second,
			serverDelay:   2 * time.Second,
			expectSuccess: false,
			expectError:   "context deadline exceeded",
		},
		{
			name:          "Extended_Timeout_Success",
			clientTimeout: 10 * time.Second,
			serverDelay:   500 * time.Millisecond,
			expectSuccess: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create server with configurable delay
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(tt.serverDelay)
				response := map[string]interface{}{
					"access_token":  "test_token",
					"token_type":    "Bearer",
					"expires_in":    3600,
					"refresh_token": "new_refresh",
					"scope":         "read",
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			var logOutput strings.Builder
			logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))

			client := NewOAuth2Client("test_id", "test_secret", server.URL, logger)
			client.httpClient.Timeout = tt.clientTimeout

			ctx, cancel := context.WithTimeout(context.Background(), tt.clientTimeout+1*time.Second)
			defer cancel()

			result, err := client.RefreshToken(ctx, "test_refresh_token")

			if tt.expectSuccess {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, "test_token", result.AccessToken)
			} else {
				assert.Error(t, err)
				assert.Nil(t, result)
				if tt.expectError != "" {
					assert.Contains(t, err.Error(), tt.expectError)
				}
			}
		})
	}
}

// TestOAuth2Client_EnvironmentTimeoutConfiguration tests timeout configuration from environment variables
func TestOAuth2Client_EnvironmentTimeoutConfiguration(t *testing.T) {
	tests := []struct {
		name                    string
		httpClientTimeout       string
		tlsHandshakeTimeout     string
		responseHeaderTimeout   string
		expectedClientTimeout   time.Duration
		expectedTLSTimeout      time.Duration
		expectedResponseTimeout time.Duration
	}{
		{
			name:                    "Default_Timeouts",
			httpClientTimeout:       "",
			tlsHandshakeTimeout:     "",
			responseHeaderTimeout:   "",
			expectedClientTimeout:   120 * time.Second,
			expectedTLSTimeout:      15 * time.Second,
			expectedResponseTimeout: 60 * time.Second,
		},
		{
			name:                    "Custom_Duration_Strings",
			httpClientTimeout:       "180s",
			tlsHandshakeTimeout:     "30s",
			responseHeaderTimeout:   "90s",
			expectedClientTimeout:   180 * time.Second,
			expectedTLSTimeout:      30 * time.Second,
			expectedResponseTimeout: 90 * time.Second,
		},
		{
			name:                    "Custom_Seconds_Numbers",
			httpClientTimeout:       "240",
			tlsHandshakeTimeout:     "20",
			responseHeaderTimeout:   "45",
			expectedClientTimeout:   240 * time.Second,
			expectedTLSTimeout:      20 * time.Second,
			expectedResponseTimeout: 45 * time.Second,
		},
		{
			name:                    "Invalid_Values_Use_Defaults",
			httpClientTimeout:       "invalid",
			tlsHandshakeTimeout:     "999999",
			responseHeaderTimeout:   "-10s",
			expectedClientTimeout:   120 * time.Second,
			expectedTLSTimeout:      15 * time.Second,
			expectedResponseTimeout: 60 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables for this test
			if tt.httpClientTimeout != "" {
				t.Setenv("HTTP_CLIENT_TIMEOUT", tt.httpClientTimeout)
			}
			if tt.tlsHandshakeTimeout != "" {
				t.Setenv("TLS_HANDSHAKE_TIMEOUT", tt.tlsHandshakeTimeout)
			}
			if tt.responseHeaderTimeout != "" {
				t.Setenv("RESPONSE_HEADER_TIMEOUT", tt.responseHeaderTimeout)
			}

			var logOutput strings.Builder
			logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelInfo}))

			client := NewOAuth2Client("test_id", "test_secret", "https://test.example.com", logger)

			// Verify client timeout
			assert.Equal(t, tt.expectedClientTimeout, client.httpClient.Timeout)

			// Verify transport timeouts
			transport, ok := client.httpClient.Transport.(*http.Transport)
			require.True(t, ok)
			assert.Equal(t, tt.expectedTLSTimeout, transport.TLSHandshakeTimeout)
			assert.Equal(t, tt.expectedResponseTimeout, transport.ResponseHeaderTimeout)

			// Verify logs contain timeout configuration
			logText := logOutput.String()
			assert.Contains(t, logText, "OAuth2 client timeout configuration")
		})
	}
}

// TestGetTimeoutFromEnv tests the timeout parsing function directly
func TestGetTimeoutFromEnv(t *testing.T) {
	tests := []struct {
		name           string
		envValue       string
		defaultTimeout time.Duration
		expected       time.Duration
	}{
		{
			name:           "Empty_Uses_Default",
			envValue:       "",
			defaultTimeout: 60 * time.Second,
			expected:       60 * time.Second,
		},
		{
			name:           "Valid_Duration_String",
			envValue:       "2m30s",
			defaultTimeout: 60 * time.Second,
			expected:       150 * time.Second,
		},
		{
			name:           "Valid_Seconds_Number",
			envValue:       "90",
			defaultTimeout: 60 * time.Second,
			expected:       90 * time.Second,
		},
		{
			name:           "Invalid_Duration_Uses_Default",
			envValue:       "invalid",
			defaultTimeout: 60 * time.Second,
			expected:       60 * time.Second,
		},
		{
			name:           "Too_Large_Duration_Uses_Default",
			envValue:       "20m",
			defaultTimeout: 60 * time.Second,
			expected:       60 * time.Second,
		},
		{
			name:           "Negative_Duration_Uses_Default",
			envValue:       "-30s",
			defaultTimeout: 60 * time.Second,
			expected:       60 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TEST_TIMEOUT", tt.envValue)
			result := getTimeoutFromEnv("TEST_TIMEOUT", tt.defaultTimeout)
			assert.Equal(t, tt.expected, result)
		})
	}
}
