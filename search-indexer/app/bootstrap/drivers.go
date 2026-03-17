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

// initMeilisearchClient initializes the Meilisearch client with retry logic.
func initMeilisearchClient() (meilisearch.ServiceManager, error) {
	const maxRetries = 5
	const retryDelay = 5 * time.Second

	meilisearchHost := os.Getenv("MEILISEARCH_HOST")

	// Support _FILE suffix for Docker Secrets (same pattern as alt-backend)
	meilisearchKey := os.Getenv("MEILISEARCH_API_KEY")
	if meilisearchKeyFile := os.Getenv("MEILISEARCH_API_KEY_FILE"); meilisearchKeyFile != "" {
		if content, err := os.ReadFile(meilisearchKeyFile); err == nil {
			meilisearchKey = strings.TrimSpace(string(content))
		}
	}

	if meilisearchHost == "" {
		return nil, fmt.Errorf("MEILISEARCH_HOST environment variable is not set")
	}

	logger.Logger.Info("Connecting to Meilisearch", "host", meilisearchHost)

	var msClient meilisearch.ServiceManager

	for i := range maxRetries {
		msClient = meilisearch.New(meilisearchHost, meilisearch.WithAPIKey(meilisearchKey))

		if _, healthErr := msClient.Health(); healthErr != nil {
			logger.Logger.Warn("Meilisearch not ready, retrying", "attempt", i+1, "max", maxRetries, "err", healthErr)
			if i < maxRetries-1 {
				time.Sleep(retryDelay)
				continue
			}
			return nil, fmt.Errorf("failed to connect to Meilisearch after %d attempts: %w", maxRetries, healthErr)
		}

		logger.Logger.Info("Connected to Meilisearch successfully")
		break
	}

	return msClient, nil
}
