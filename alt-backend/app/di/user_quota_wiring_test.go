package di

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBackendComponents_WiresAndLogsUserQuotaLimiter(t *testing.T) {
	setDataHubClientEnv(t)
	cfg := splitTestConfig()
	cfg.RateLimit.PrefetchSlotWait = "250ms"
	cfg.ImageProxy.Enabled = false
	cfg.SearchIndexer.ConnectURL = "https://search-indexer:9301"
	cfg.PreProcessor.URL = "https://pre-processor:9200"
	cfg.PreProcessor.ConnectURL = "https://pre-processor:9202"
	cfg.Rag.OrchestratorURL = "http://rag-orchestrator:9010"
	cfg.Rag.OrchestratorConnectURL = "http://rag-orchestrator:9011"

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	components, err := NewBackendComponents(cfg)
	require.NoError(t, err)
	require.NotNil(t, components.UserQuotaLimiter)
	assert.Contains(t, buf.String(), `"msg":"user_quota_configured"`)
	assert.Contains(t, buf.String(), `"enabled":true`)
}
