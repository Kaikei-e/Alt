package search_indexer_connect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/shared/domain/authcontext"
)

func TestClient_PropagatesJWTHeader(t *testing.T) {
	var receivedToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedToken = r.Header.Get("X-Alt-Backend-Token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits": []}`))
	}))
	defer server.Close()

	client := NewClientWithHTTPClient(server.URL, server.Client())
	ctx := authcontext.WithJWT(context.Background(), "test-user-jwt-token")

	hits, err := client.SearchArticles(ctx, "test query", "user-123")
	require.NoError(t, err)
	assert.Empty(t, hits)
	assert.Equal(t, "test-user-jwt-token", receivedToken)
}

func TestClient_NewClient_RequiresHTTPS(t *testing.T) {
	_, err := NewClient("http://search-indexer:9443")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must use https:// scheme")
}

func TestClient_NewClient_FailsMissingCertEnv(t *testing.T) {
	t.Setenv("MTLS_CERT_FILE", "")
	t.Setenv("MTLS_KEY_FILE", "")
	t.Setenv("MTLS_CA_FILE", "")

	_, err := NewClient("https://search-indexer:9443")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing MTLS cert/key/ca")
}
