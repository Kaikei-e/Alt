package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMQHubAuth_SecretFileValidation(t *testing.T) {
	t.Run("valid token file loads token", func(t *testing.T) {
		tmpDir := t.TempDir()
		tokenFile := filepath.Join(tmpDir, "mqhub_token.txt")
		require.NoError(t, os.WriteFile(tokenFile, []byte("valid-mqhub-token-123\n"), 0600))

		cfg := &MQHubConfig{
			AuthTokenFile: tokenFile,
		}
		err := loadMQHubAuthToken(cfg, os.ReadFile)
		require.NoError(t, err)
		assert.Equal(t, "valid-mqhub-token-123", cfg.AuthToken)
	})

	t.Run("empty token file fails fast", func(t *testing.T) {
		tmpDir := t.TempDir()
		tokenFile := filepath.Join(tmpDir, "empty_token.txt")
		require.NoError(t, os.WriteFile(tokenFile, []byte("   \n\t  \n"), 0600))

		cfg := &MQHubConfig{
			AuthTokenFile: tokenFile,
		}
		err := loadMQHubAuthToken(cfg, os.ReadFile)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty token")
	})

	t.Run("unreadable token file fails fast", func(t *testing.T) {
		cfg := &MQHubConfig{
			AuthTokenFile: "/nonexistent/mqhub_token.txt",
		}
		err := loadMQHubAuthToken(cfg, os.ReadFile)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MQHUB_AUTH_TOKEN_FILE")
	})
}

func TestMQHubAuth_EnabledRequiresToken(t *testing.T) {
	t.Run("ValidateBackendConfig fails when MQHUB_ENABLED=true and token is empty", func(t *testing.T) {
		cfg := &Config{
			AppEnv:        "development",
			SearchIndexer: SearchIndexerConfig{ConnectURL: "https://search-indexer:9443"},
			Rag: RAGConfig{
				OrchestratorURL:        "http://rag:8000",
				OrchestratorConnectURL: "http://rag:8001",
				APIAuth:                "disabled",
			},
			PreProcessor: PreProcessorConfig{
				URL:        "https://pre-processor:9443",
				ConnectURL: "https://pre-processor:9443",
			},
			WebPush: WebPushConfig{PublicKey: "vapid-key"},
			MQHub: MQHubConfig{
				Enabled:    true,
				ConnectURL: "http://mq-hub:9500",
				AuthToken:  "",
			},
		}
		err := ValidateBackendConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MQHUB_AUTH_TOKEN")
	})

	t.Run("ValidateDataHubConfig fails when MQHUB_ENABLED=true and token is empty", func(t *testing.T) {
		cfg := &Config{
			AppEnv:  "development",
			AuthHub: AuthHubConfig{URL: "http://auth-hub:8888"},
			Auth: AuthConfig{
				BackendTokenSecret: "secret-123456789012345678901234",
				InternalAuthSecret: "internal-123456789012345678901234",
			},
			Sovereign: SovereignConfig{
				URL:       "http://sovereign:9500",
				EventAuth: "disabled",
			},
			MQHub: MQHubConfig{
				Enabled:    true,
				ConnectURL: "http://mq-hub:9500",
				AuthToken:  "",
			},
		}
		err := ValidateDataHubConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MQHUB_AUTH_TOKEN")
	})

	t.Run("ValidateHarvesterConfig fails when MQHUB_ENABLED=true and token is empty", func(t *testing.T) {
		cfg := &Config{
			AppEnv: "development",
			Sovereign: SovereignConfig{
				URL:       "http://sovereign:9500",
				EventAuth: "disabled",
			},
			Rag: RAGConfig{
				OrchestratorURL:        "http://rag:8000",
				OrchestratorConnectURL: "http://rag:8001",
				APIAuth:                "disabled",
			},
			MQHub: MQHubConfig{
				Enabled:    true,
				ConnectURL: "http://mq-hub:9500",
				AuthToken:  "",
			},
		}
		err := ValidateHarvesterConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MQHUB_AUTH_TOKEN")
	})
}
