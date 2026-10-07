package di

import (
	"context"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"alt/orchestrator/port/rag_integration_port"
)

func TestNewHarvesterComponents_RagClient_TLSWiring(t *testing.T) {
	dir := t.TempDir()
	caCert, caKey, caPath, caPool := newDITestCA(t, dir, "alt-ca")

	_, _, serverCert := issueDITestCert(
		t,
		dir,
		"rag-orchestrator",
		caCert,
		caKey,
		true,
		[]string{"rag-orchestrator"},
		[]net.IP{net.ParseIP("127.0.0.1")},
	)
	clientCertPath, clientKeyPath, _ := issueDITestCert(
		t,
		dir,
		"alt-harvester",
		caCert,
		caKey,
		false,
		[]string{"alt-harvester"},
		nil,
	)

	var serverHit atomic.Bool
	srv := startTestMTLSServer(t, serverCert, caPool, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/rag/index/upsert" && r.Method == http.MethodPost {
			serverHit.Store(true)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		http.NotFound(w, r)
	}))

	t.Setenv("DATA_HUB_MTLS_URL", "https://alt-data-hub:9443")
	t.Setenv("DATA_HUB_SERVER_NAME", "alt-data-hub")
	t.Setenv("MTLS_CERT_FILE", clientCertPath)
	t.Setenv("MTLS_KEY_FILE", clientKeyPath)
	t.Setenv("MTLS_CA_FILE", caPath)

	cfg := splitTestConfig()
	cfg.Sovereign.URL = "http://localhost:9500"
	cfg.ImageProxy.Enabled = false
	cfg.Rag.OrchestratorURL = srv.URL
	cfg.Rag.APIToken = "test-token"

	container := NewHarvesterComponents(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := container.RagIntegration.UpsertArticle(ctx, rag_integration_port.UpsertArticleInput{
		ArticleID: "00000000-0000-0000-0000-000000000001",
		UserID:    "00000000-0000-0000-0000-000000000002",
		Title:     "Test Title",
		Body:      "Test Body",
		URL:       "https://example.com/test",
	})
	require.NoError(t, err)
	require.True(t, serverHit.Load())
}
