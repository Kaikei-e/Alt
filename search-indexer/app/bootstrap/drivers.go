package bootstrap

import (
	"context"
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
		content, err := os.ReadFile(fileEnv)
		if err != nil {
			logger.Logger.Warn("failed to read secret file; falling back to env",
				"key", key,
				"file", fileEnv,
				"error", err,
			)
		} else {
			return strings.TrimSpace(string(content))
		}
	}
	return os.Getenv(key)
}

// meilisearchHTTPClient builds the client every meilisearch-go call runs on.
//
// The SDK's default client sets no response timeout: the 30s in its
// baseTransport is the net.Dialer's dial budget, which a saturated
// Meilisearch passes -- it accepts the connection and then never answers. A
// call that carries no context of its own therefore hangs forever, and the
// SDK's non-context methods (Health, FetchInfo, ...) bake context.Background()
// in, so no caller can rescue them. At startup that means the process never
// reaches ListenAndServe and :9300 / :9301 never bind at all.
//
// http.DefaultTransport carries the same proxy, dial timeout, keep-alive,
// idle-pool, TLS-handshake and expect-continue settings as the SDK's own
// baseTransport; only the per-host idle cap differs, so it is pinned to
// match.
func meilisearchHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = transport.MaxIdleConns
	return &http.Client{
		Timeout:   config.MeiliTimeout,
		Transport: transport,
	}
}

// probeMeilisearchHealth bounds the startup health probe twice over: by the
// caller's context, so SIGTERM during the retry loop is honoured, and by
// MeiliTimeout, so a context without a deadline (Run passes the bare signal
// context) still cannot wait forever.
func probeMeilisearchHealth(ctx context.Context, client meilisearch.ServiceManager) error {
	healthCtx, cancel := context.WithTimeout(ctx, config.MeiliTimeout)
	defer cancel()
	_, err := client.HealthWithContext(healthCtx)
	return err
}

// initMeilisearchClients initializes one admin client (required) and,
// if configured, a separate search-only client for read operations (L-001).
// Operators can provision the search key via MEILISEARCH_SEARCH_API_KEY or
// MEILISEARCH_SEARCH_API_KEY_FILE. When unset, the admin client is reused.
func initMeilisearchClients(ctx context.Context) (admin meilisearch.ServiceManager, search meilisearch.ServiceManager, err error) {
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
		admin = meilisearch.New(meilisearchHost,
			meilisearch.WithAPIKey(adminKey),
			meilisearch.WithCustomClient(meilisearchHTTPClient()),
		)

		if healthErr := probeMeilisearchHealth(ctx, admin); healthErr != nil {
			logger.Logger.Warn("Meilisearch not ready, retrying", "attempt", i+1, "max", maxRetries, "err", healthErr)
			if i < maxRetries-1 {
				if err := sleepOrDone(ctx, retryDelay); err != nil {
					return nil, nil, fmt.Errorf("meilisearch connect cancelled: %w", err)
				}
				continue
			}
			return nil, nil, fmt.Errorf("failed to connect to Meilisearch after %d attempts: %w", maxRetries, healthErr)
		}

		logger.Logger.Info("Connected to Meilisearch successfully")
		break
	}

	if searchKey != "" && searchKey != adminKey {
		search = meilisearch.New(meilisearchHost,
			meilisearch.WithAPIKey(searchKey),
			meilisearch.WithCustomClient(meilisearchHTTPClient()),
		)
	}

	return admin, search, nil
}

// sleepOrDone waits for d or returns ctx.Err() if the context is cancelled
// first, so Meilisearch retry loops do not block SIGTERM for up to 25s.
func sleepOrDone(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// initMeilisearchClient preserves the single-client API for existing callers
// (recap indexer today). New code should prefer initMeilisearchClients.
func initMeilisearchClient(ctx context.Context) (meilisearch.ServiceManager, error) {
	admin, _, err := initMeilisearchClients(ctx)
	return admin, err
}
