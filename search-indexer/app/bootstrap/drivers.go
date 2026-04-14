package bootstrap

import (
	"fmt"
	"os"
	"strings"
	"time"

	"search-indexer/config"
	"search-indexer/driver/backend_api"
	"search-indexer/gateway"
	"search-indexer/logger"

	"github.com/meilisearch/meilisearch-go"
)

// initArticleDriver creates the backend API article driver.
func initArticleDriver(cfg *config.Config) (gateway.ArticleDriver, error) {
	logger.Logger.Info("Using backend API driver",
		"url", cfg.BackendAPI.URL,
	)
	client := backend_api.NewClient(cfg.BackendAPI.URL, cfg.BackendAPI.ServiceToken)
	return client, nil
}

// readSecretEnv returns the value of key, or the contents of the file named
// by key+"_FILE" if set (Docker Secrets convention).
func readSecretEnv(key string) string {
	if fileEnv := os.Getenv(key + "_FILE"); fileEnv != "" {
		if content, err := os.ReadFile(fileEnv); err == nil {
			return strings.TrimSpace(string(content))
		}
	}
	return os.Getenv(key)
}

// initMeilisearchClients initializes one admin client (required) and,
// if configured, a separate search-only client for read operations (L-001).
// Operators can provision the search key via MEILISEARCH_SEARCH_API_KEY or
// MEILISEARCH_SEARCH_API_KEY_FILE. When unset, the admin client is reused.
func initMeilisearchClients() (admin meilisearch.ServiceManager, search meilisearch.ServiceManager, err error) {
	const maxRetries = 5
	const retryDelay = 5 * time.Second

	meilisearchHost := os.Getenv("MEILISEARCH_HOST")
	if meilisearchHost == "" {
		return nil, nil, fmt.Errorf("MEILISEARCH_HOST environment variable is not set")
	}

	adminKey := readSecretEnv("MEILISEARCH_API_KEY")
	searchKey := readSecretEnv("MEILISEARCH_SEARCH_API_KEY")

	logger.Logger.Info("Connecting to Meilisearch",
		"host", meilisearchHost,
		"search_key_role_split", searchKey != "" && searchKey != adminKey,
	)

	for i := range maxRetries {
		admin = meilisearch.New(meilisearchHost, meilisearch.WithAPIKey(adminKey))

		if _, healthErr := admin.Health(); healthErr != nil {
			logger.Logger.Warn("Meilisearch not ready, retrying", "attempt", i+1, "max", maxRetries, "err", healthErr)
			if i < maxRetries-1 {
				time.Sleep(retryDelay)
				continue
			}
			return nil, nil, fmt.Errorf("failed to connect to Meilisearch after %d attempts: %w", maxRetries, healthErr)
		}

		logger.Logger.Info("Connected to Meilisearch successfully")
		break
	}

	if searchKey != "" && searchKey != adminKey {
		search = meilisearch.New(meilisearchHost, meilisearch.WithAPIKey(searchKey))
	}

	return admin, search, nil
}

// initMeilisearchClient preserves the single-client API for existing callers
// (recap indexer today). New code should prefer initMeilisearchClients.
func initMeilisearchClient() (meilisearch.ServiceManager, error) {
	admin, _, err := initMeilisearchClients()
	return admin, err
}
