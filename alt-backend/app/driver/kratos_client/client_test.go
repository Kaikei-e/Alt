package kratos_client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

			client := NewKratosClient(server.URL, tt.sharedSecret)
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

	client := NewKratosClient(server.URL, "test-secret")
	_, err := client.GetFirstIdentityID(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode")
}

func TestKratosClient_GetFirstIdentityID_ConnectionError(t *testing.T) {
	client := NewKratosClient("http://localhost:99999", "test-secret")
	_, err := client.GetFirstIdentityID(context.Background())

	require.Error(t, err)
}
