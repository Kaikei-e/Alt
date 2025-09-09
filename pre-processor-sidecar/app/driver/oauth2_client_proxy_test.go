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
	assert.Equal(t, 10*time.Second, transport.TLSHandshakeTimeout)
	assert.Equal(t, 30*time.Second, transport.ResponseHeaderTimeout)
	assert.Equal(t, 90*time.Second, transport.IdleConnTimeout)
	assert.Equal(t, 10, transport.MaxIdleConns)
	assert.Equal(t, 2, transport.MaxIdleConnsPerHost)

	// Verify client timeout
	assert.Equal(t, 60*time.Second, client.httpClient.Timeout)
}