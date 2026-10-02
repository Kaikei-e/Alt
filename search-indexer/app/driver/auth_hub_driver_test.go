package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthHubDriver_HTTPSEnforcement(t *testing.T) {
	_, err := NewAuthHubDriver("http://auth-hub:9443/internal/token/introspect")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must use HTTPS scheme")

	_, err = NewAuthHubDriverWithClient("http://auth-hub:9443/internal/token/introspect", http.DefaultClient)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must use HTTPS scheme")
}

func TestAuthHubDriver_IntrospectToken_Success(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req map[string]string
		err := json.NewDecoder(r.Body).Decode(&req)
		assert.NoError(t, err)
		assert.Equal(t, "valid-token", req["token"])

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"active":    true,
			"sub":       "00000000-0000-0000-0000-000000000001",
			"tenant_id": "00000000-0000-0000-0000-000000000002",
			"exp":       9999999999,
		})
	}))
	defer server.Close()

	driver, err := NewAuthHubDriverWithClient(server.URL, server.Client())
	require.NoError(t, err)

	resp, err := driver.IntrospectToken(context.Background(), "valid-token")
	require.NoError(t, err)
	assert.True(t, resp.Active)
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", resp.Sub)
	assert.Equal(t, "00000000-0000-0000-0000-000000000002", resp.TenantID)
	assert.Equal(t, int64(9999999999), resp.Exp)
}

func TestAuthHubDriver_RejectsTrailingGarbageAndSecondJSON(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"active":true,"sub":"00000000-0000-0000-0000-000000000001","tenant_id":"00000000-0000-0000-0000-000000000002","exp":9999999999}{"extra":"data"}`))
	}))
	defer server.Close()

	driver, err := NewAuthHubDriverWithClient(server.URL, server.Client())
	require.NoError(t, err)

	_, err = driver.IntrospectToken(context.Background(), "token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trailing garbage or multiple JSON values")
}

func TestAuthHubDriver_RejectsOversizedResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Write 1MB + 10 bytes
		_, _ = w.Write(bytes.Repeat([]byte(" "), (1024*1024)+10))
	}))
	defer server.Close()

	driver, err := NewAuthHubDriverWithClient(server.URL, server.Client())
	require.NoError(t, err)

	_, err = driver.IntrospectToken(context.Background(), "token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1MB limit")
}

func TestAuthHubDriver_RejectsInvalidTenantUUID(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"active":true,"sub":"00000000-0000-0000-0000-000000000001","tenant_id":"not-a-uuid","exp":9999999999}`))
	}))
	defer server.Close()

	driver, err := NewAuthHubDriverWithClient(server.URL, server.Client())
	require.NoError(t, err)

	_, err = driver.IntrospectToken(context.Background(), "token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tenant_id uuid")
}

func TestAuthHubDriver_NoRedirect(t *testing.T) {
	redirectServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://malicious.example.com", http.StatusFound)
	}))
	defer redirectServer.Close()

	client := redirectServer.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	driver, err := NewAuthHubDriverWithClient(redirectServer.URL, client)
	require.NoError(t, err)

	// Since redirect is stopped and status is 302, it should return an error rather than following
	_, err = driver.IntrospectToken(context.Background(), "secret-jwt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authhub returned status 302")
}
