package preprocessor_connect

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/utils/authcontext"
)

func TestConnectPreProcessorClient_PropagatesJWTHeader(t *testing.T) {
	var receivedToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedToken = r.Header.Get("X-Alt-Backend-Token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"summary": "article summary"}`))
	}))
	defer server.Close()

	client := NewConnectPreProcessorClientWithHTTPClient(server.URL, server.Client())
	ctx := authcontext.WithJWT(context.Background(), "user-preprocessor-token")

	summary, err := client.Summarize(ctx, "content", "art-1", "title")
	require.NoError(t, err)
	assert.Equal(t, "article summary", summary)
	assert.Equal(t, "user-preprocessor-token", receivedToken)
}

func TestConnectPreProcessorClient_NewClient_RequiresHTTPS(t *testing.T) {
	_, err := NewConnectPreProcessorClient("http://pre-processor:9443", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must use https:// scheme")
}

func TestConnectPreProcessorClient_NewClient_FailsMissingCertEnv(t *testing.T) {
	t.Setenv("MTLS_CERT_FILE", "")
	t.Setenv("MTLS_KEY_FILE", "")
	t.Setenv("MTLS_CA_FILE", "")

	_, err := NewConnectPreProcessorClient("https://pre-processor:9443", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing MTLS cert/key/ca")
}

func TestConnectPreProcessorClient_QueueSummarize_SendsExplicitOrigin(t *testing.T) {
	tests := []struct {
		name           string
		origin         SummarizeOrigin
		expectedOrigin string
	}{
		{
			name:           "user origin",
			origin:         SummarizeOriginUser,
			expectedOrigin: "SUMMARIZE_ORIGIN_USER",
		},
		{
			name:           "system origin",
			origin:         SummarizeOriginSystem,
			expectedOrigin: "SUMMARIZE_ORIGIN_SYSTEM",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var receivedBody map[string]string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/services.preprocessor.v2.PreProcessorService/QueueSummarize", r.URL.Path)
				require.NoError(t, json.NewDecoder(r.Body).Decode(&receivedBody))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"jobId":"job-1","status":"pending","message":"queued"}`))
			}))
			defer server.Close()

			client := NewConnectPreProcessorClientWithHTTPClient(server.URL, server.Client())
			jobID, err := client.QueueSummarize(context.Background(), "art-1", "title", tc.origin)
			require.NoError(t, err)
			assert.Equal(t, "job-1", jobID)
			assert.Equal(t, tc.expectedOrigin, receivedBody["origin"])
		})
	}
}

func TestConnectPreProcessorClient_QueueSummarize_RejectsInvalidOrigin(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewConnectPreProcessorClientWithHTTPClient(server.URL, server.Client())

	_, err := client.QueueSummarize(context.Background(), "art-1", "title", "")
	require.Error(t, err)
	assert.False(t, called)

	_, err = client.QueueSummarize(context.Background(), "art-1", "title", SummarizeOrigin("unknown"))
	require.Error(t, err)
	assert.False(t, called)
}
