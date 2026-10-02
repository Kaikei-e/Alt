package preprocessor_connect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/shared/domain/authcontext"
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
