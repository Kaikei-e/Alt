package di

import (
	"alt/config"
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLogSovereignWiringState pins CLAUDE.md rule 8 / di-wiring.md: a missing
// SOVEREIGN_URL must be loudly distinguishable from a deliberately disabled
// client, and must fail fast in production instead of starting in limp mode
// with every Knowledge Home mutation silently no-op'ing.
func TestLogSovereignWiringState(t *testing.T) {
	t.Run("enabled when SOVEREIGN_URL is set with event auth enabled", func(t *testing.T) {
		assert.True(t, LogSovereignWiringState("alt-backend", "http://knowledge-sovereign:9500", "development", true))
	})

	t.Run("enabled when SOVEREIGN_URL is set with event auth disabled", func(t *testing.T) {
		assert.True(t, LogSovereignWiringState("alt-backend", "http://knowledge-sovereign:9500", "development", false))
	})

	t.Run("disabled but non-fatal outside production", func(t *testing.T) {
		assert.False(t, LogSovereignWiringState("alt-backend", "", "development", false))
		assert.False(t, LogSovereignWiringState("alt-backend", "", "", false))
	})

	t.Run("panics when unset in production", func(t *testing.T) {
		assert.Panics(t, func() {
			LogSovereignWiringState("alt-backend", "", "production", false)
		})
	})
}

func TestNewKnowledgeModule_OperatorWiring(t *testing.T) {
	t.Run("panics when SOVEREIGN_URL is set but operator token is missing", func(t *testing.T) {
		t.Setenv("SOVEREIGN_URL", "http://knowledge-sovereign:9500")
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN_FILE", "")
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN", "")

		cfg := &config.Config{
			Sovereign: config.SovereignConfig{
				URL: "http://knowledge-sovereign:9500",
			},
		}
		infra := &InfraModule{Config: cfg}
		article := &ArticleModule{}

		assert.Panics(t, func() {
			newKnowledgeModule(infra, article)
		})
	})

	t.Run("wires distinct operator client when operator token is provided", func(t *testing.T) {
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN", "valid-operator-token-long-enough-12345")
		cfg := &config.Config{
			Sovereign: config.SovereignConfig{
				URL:        "http://knowledge-sovereign:9500",
				EventToken: "valid-backend-token-long-enough-123456",
			},
		}
		infra := &InfraModule{Config: cfg}
		article := &ArticleModule{}

		km := newKnowledgeModule(infra, article)
		assert.NotNil(t, km)
		assert.True(t, km.SovereignClient.Enabled())
		assert.True(t, km.SovereignOperatorClient.Enabled())
		assert.NotSame(t, km.SovereignClient, km.SovereignOperatorClient)
	})
}

func TestNewBackendComponents_EmitsHomeFeedSyncStartupLogs(t *testing.T) {
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
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	components, err := NewBackendComponents(cfg)
	require.NoError(t, err)
	require.NotNil(t, components)

	logs := buf.String()
	for _, wantMsg := range []string{
		"track_home_action_article_status_sync_enabled",
		"articles_reading_status_home_dismiss_enabled",
		"feeds_reading_status_home_dismiss_enabled",
		"register_favorite_feed_home_open_enabled",
	} {
		assert.Contains(t, logs, `"msg":"`+wantMsg+`"`, "missing startup log %s", wantMsg)
	}
}
