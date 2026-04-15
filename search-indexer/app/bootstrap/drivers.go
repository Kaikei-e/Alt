package bootstrap

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"search-indexer/config"
	"search-indexer/driver/backend_api"
	"search-indexer/gateway"
	"search-indexer/logger"
	"search-indexer/tlsutil"

	"github.com/meilisearch/meilisearch-go"
)

// buildBackendHTTPClient returns the *http.Client used for outbound
// Connect-RPC calls to alt-backend. When MTLS_ENFORCE=true the client is
// built from tlsutil.LoadClientConfig; otherwise backend_api's default.
func buildBackendHTTPClient() (*http.Client, error) {
	if os.Getenv("MTLS_ENFORCE") != "true" {
		return backend_api.DefaultHTTPClient(), nil
	}
	tlsCfg, err := tlsutil.LoadClientConfig(
		os.Getenv("MTLS_CERT_FILE"),
		os.Getenv("MTLS_KEY_FILE"),
		os.Getenv("MTLS_CA_FILE"),
	)
	if err != nil {
		return nil, fmt.Errorf("backend mTLS client (fail-closed): %w", err)
	}
	if sn := os.Getenv("BACKEND_MTLS_SERVER_NAME"); sn != "" {
		tlsCfg.ServerName = sn
	}
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig:     tlsCfg,
			IdleConnTimeout:     30 * time.Second,
			MaxIdleConnsPerHost: 4,
		},
	}, nil
}

// initArticleDriver creates the backend API article driver.
func initArticleDriver(cfg *config.Config) (gateway.ArticleDriver, error) {
	url := cfg.BackendAPI.URL
	if mtlsURL := os.Getenv("BACKEND_API_MTLS_URL"); mtlsURL != "" && os.Getenv("MTLS_ENFORCE") == "true" {
		url = mtlsURL
	}
	httpClient, err := buildBackendHTTPClient()
	if err != nil {
		return nil, err
	}
	logger.Logger.Info("Using backend API driver",
		"url", url,
		"mtls_enforce", os.Getenv("MTLS_ENFORCE") == "true",
	)
	client := backend_api.NewClient(url, "", httpClient)
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
