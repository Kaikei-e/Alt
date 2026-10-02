package kratos_client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewKratosClient_ProductionSecurityInvariants(t *testing.T) {
	t.Run("rejects plaintext http URL fail-closed", func(t *testing.T) {
		assert.Panics(t, func() {
			NewKratosClient("http://auth-hub:8888", "secret", &http.Client{})
		}, "NewKratosClient must panic on plaintext http URL")
	})

	t.Run("rejects nil httpClient fail-closed", func(t *testing.T) {
		assert.Panics(t, func() {
			NewKratosClient("https://auth-hub:9443", "secret", nil)
		}, "NewKratosClient must panic on nil httpClient")
	})

	t.Run("configures timeout and prevents credential redirect forward", func(t *testing.T) {
		rawClient := &http.Client{}
		client := NewKratosClient("https://auth-hub:9443", "secret", rawClient)
		impl, ok := client.(*authHubClientImpl)
		require.True(t, ok)
		assert.Equal(t, 10*time.Second, impl.httpClient.Timeout)
		require.NotNil(t, impl.httpClient.CheckRedirect)

		err := impl.httpClient.CheckRedirect(nil, nil)
		assert.ErrorIs(t, err, http.ErrUseLastResponse, "must not follow redirects (prevent credential forwarding)")
	})
}

func TestKratosClient_GetFirstIdentityID(t *testing.T) {
	tests := []struct {
		name           string
		sharedSecret   string
		responseBody   any
		responseStatus int
		wantID         string
		wantErr        bool
		errContains    string
	}{
		{
			name:         "success - valid user_id with auth header",
			sharedSecret: "test-shared-secret-value",
			responseBody: map[string]string{
				"user_id": "user-123-uuid",
			},
			responseStatus: http.StatusOK,
			wantID:         "user-123-uuid",
			wantErr:        false,
		},
		{
			name:         "error - empty user_id",
			sharedSecret: "test-shared-secret-value",
			responseBody: map[string]string{
				"user_id": "",
			},
			responseStatus: http.StatusOK,
			wantErr:        true,
			errContains:    "empty user_id",
		},
		{
			name:           "error - server error",
			sharedSecret:   "test-shared-secret-value",
			responseBody:   map[string]string{"error": "internal error"},
			responseStatus: http.StatusInternalServerError,
			wantErr:        true,
			errContains:    "failed to fetch system user",
		},
		{
			name:           "error - not found",
			sharedSecret:   "test-shared-secret-value",
			responseBody:   map[string]string{"error": "not found"},
			responseStatus: http.StatusNotFound,
			wantErr:        true,
			errContains:    "failed to fetch system user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/internal/system-user", r.URL.Path)
				assert.Equal(t, tt.sharedSecret, r.Header.Get("X-Internal-Auth"))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.responseStatus)
				json.NewEncoder(w).Encode(tt.responseBody)
			}))
			defer server.Close()

			client := NewKratosClientForTest(server.URL, tt.sharedSecret, nil)
			id, err := client.GetFirstIdentityID(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantID, id)
		})
	}
}

func TestKratosClient_GetFirstIdentityID_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-secret", r.Header.Get("X-Internal-Auth"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewKratosClientForTest(server.URL, "test-secret", nil)
	_, err := client.GetFirstIdentityID(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode")
}

func TestKratosClient_GetFirstIdentityID_ConnectionError(t *testing.T) {
	client := NewKratosClientForTest("http://localhost:99999", "test-secret", nil)
	_, err := client.GetFirstIdentityID(context.Background())

	require.Error(t, err)
}
