package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"alt/config"
	"alt/orchestrator/gateway/rag_gateway"
	"alt/tlsutil"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultBatchSize          = 500
	defaultBatchTimeout       = 60 * time.Second
	defaultRAGOrchestratorURL = "https://rag-orchestrator:9010"
)

func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func requiredEnv(key string) (string, error) {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value, nil
	}
	return "", fmt.Errorf("%s is required", key)
}

func resolveRAGOrchestratorURL() string {
	return getEnv("RAG_ORCHESTRATOR_URL", defaultRAGOrchestratorURL)
}

func newRAGHTTPClient(ragURL string, log *slog.Logger) (*http.Client, error) {
	var client *http.Client
	switch {
	case strings.HasPrefix(ragURL, "https://"):
		certFile := strings.TrimSpace(os.Getenv("MTLS_CERT_FILE"))
		keyFile := strings.TrimSpace(os.Getenv("MTLS_KEY_FILE"))
		caFile := strings.TrimSpace(os.Getenv("MTLS_CA_FILE"))
		if certFile == "" || keyFile == "" || caFile == "" {
			return nil, fmt.Errorf("https RAG_ORCHESTRATOR_URL (%s) requires MTLS_CERT_FILE, MTLS_KEY_FILE, and MTLS_CA_FILE", ragURL)
		}
		var err error
		client, err = tlsutil.NewMTLSClient(certFile, keyFile, caFile)
		if err != nil {
			return nil, fmt.Errorf("build mTLS client for %s: %w", ragURL, err)
		}
		log.Info("rag_mtls_enabled", "binary", "rag_owner_backfill", "url", ragURL)
	case strings.HasPrefix(ragURL, "http://"):
		log.Warn("rag_mtls_disabled", "binary", "rag_owner_backfill", "url", ragURL,
			"reason", "RAG_ORCHESTRATOR_URL is http (explicit opt-out)")
		client = &http.Client{}
	default:
		return nil, fmt.Errorf("RAG_ORCHESTRATOR_URL=%q must use https:// or http:// scheme", ragURL)
	}

	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client, nil
}

func newRAGClient(ragURL string, log *slog.Logger) (*rag_gateway.ClientWithResponses, error) {
	ragToken, ragAuthEnabled, err := config.LoadRAGAuth()
	if err != nil {
		return nil, fmt.Errorf("load RAG API authentication config: %w", err)
	}
	if ragAuthEnabled {
		log.Info("rag_api_auth_enabled", "binary", "rag_owner_backfill")
	} else {
		log.Warn("rag_api_auth_disabled", "binary", "rag_owner_backfill", "reason", "RAG_API_AUTH=disabled (explicit opt-out)")
	}

	httpClient, err := newRAGHTTPClient(ragURL, log)
	if err != nil {
		return nil, err
	}

	ragOpts := make([]rag_gateway.ClientOption, 0, 2)
	ragOpts = append(ragOpts, rag_gateway.WithHTTPClient(httpClient))
	if ragAuthEnabled {
		ragOpts = append(ragOpts, rag_gateway.WithBearerToken(ragToken))
	}

	ragClient, err := rag_gateway.NewClientWithResponses(ragURL, ragOpts...)
	if err != nil {
		return nil, fmt.Errorf("initialize rag-orchestrator client: %w", err)
	}
	return ragClient, nil
}

func loadDBPassword() (string, error) {
	if passwordFile := strings.TrimSpace(os.Getenv("DB_PASSWORD_FILE")); passwordFile != "" {
		data, err := os.ReadFile(filepath.Clean(passwordFile)) //#nosec G304,G703 -- path is operator-supplied via DB_PASSWORD_FILE env var
		if err != nil {
			return "", fmt.Errorf("read DB_PASSWORD_FILE %s: %w", passwordFile, err)
		}
		password := strings.TrimSpace(string(data))
		if password == "" {
			return "", fmt.Errorf("DB_PASSWORD_FILE %s resolved to an empty password", passwordFile)
		}
		return password, nil
	}
	if password := os.Getenv("DB_PASSWORD"); strings.TrimSpace(password) != "" {
		return password, nil
	}
	return "", fmt.Errorf("DB_PASSWORD_FILE or DB_PASSWORD is required")
}

func buildPGXPoolConfig() (*pgxpool.Config, error) {
	dbHost, err := requiredEnv("DB_HOST")
	if err != nil {
		return nil, err
	}
	dbUser, err := requiredEnv("DB_USER")
	if err != nil {
		return nil, err
	}
	dbName, err := requiredEnv("DB_NAME")
	if err != nil {
		return nil, err
	}
	dbPassword, err := loadDBPassword()
	if err != nil {
		return nil, err
	}

	dbPort := getEnv("DB_PORT", "5432")

	dsnURL := &url.URL{
		Scheme:   "postgres",
		User:     url.User(dbUser),
		Host:     net.JoinHostPort(dbHost, dbPort),
		Path:     "/" + dbName,
		RawQuery: "sslmode=prefer",
	}

	poolCfg, err := pgxpool.ParseConfig(dsnURL.String())
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}
	// Assign password directly on ConnConfig so special characters never break URL parsing.
	poolCfg.ConnConfig.Password = dbPassword
	return poolCfg, nil
}

// ArticleOwnerPair represents an article ID and its owner user_id from alt-db.
type ArticleOwnerPair struct {
	ArticleID string
	UserID    string
}

// OwnerBackfillClient is the interface for sending owner backfills to rag-orchestrator.
type OwnerBackfillClient interface {
	BackfillDocumentOwnersWithResponse(ctx context.Context, body rag_gateway.BackfillDocumentOwnersJSONRequestBody, reqEditors ...rag_gateway.RequestEditorFn) (*rag_gateway.BackfillDocumentOwnersResponse, error)
}

// BackfillStats tracks cumulative statistics across processed batches.
type BackfillStats struct {
	TotalBatches    int64
	TotalProcessed  int64
	TotalUpdated    int64
	TotalAlreadySet int64
	TotalNotFound   int64
}

// ProcessBatch sends one batch of items to rag-orchestrator (or logs in dry-run mode)
// and updates the running stats. It returns an error if the RPC fails or returns 4xx/5xx.
func ProcessBatch(
	ctx context.Context,
	client OwnerBackfillClient,
	items []rag_gateway.OwnerBackfillItem,
	dryRun bool,
	log *slog.Logger,
	stats *BackfillStats,
) error {
	if len(items) == 0 {
		return nil
	}

	stats.TotalBatches++
	stats.TotalProcessed += int64(len(items))

	if dryRun {
		log.Info("[DRY-RUN] would backfill document owners batch",
			"batch_size", len(items),
			"total_processed", stats.TotalProcessed,
		)
		return nil
	}

	batchCtx, cancel := context.WithTimeout(ctx, defaultBatchTimeout)
	defer cancel()

	resp, err := client.BackfillDocumentOwnersWithResponse(batchCtx, rag_gateway.BackfillDocumentOwnersJSONRequestBody{
		Items: items,
	})
	if err != nil {
		return fmt.Errorf("call BackfillDocumentOwners: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("BackfillDocumentOwners returned non-OK status %d: %s", resp.StatusCode(), string(resp.Body))
	}

	if resp.JSON200 == nil {
		return fmt.Errorf("BackfillDocumentOwners returned nil JSON200 on status 200")
	}

	stats.TotalUpdated += resp.JSON200.Updated
	stats.TotalAlreadySet += resp.JSON200.AlreadySet
	stats.TotalNotFound += resp.JSON200.NotFound

	log.Info("Processed document owners backfill batch",
		"batch_size", len(items),
		"updated", resp.JSON200.Updated,
		"already_set", resp.JSON200.AlreadySet,
		"not_found", resp.JSON200.NotFound,
		"running_total_processed", stats.TotalProcessed,
		"running_total_updated", stats.TotalUpdated,
		"running_total_already_set", stats.TotalAlreadySet,
		"running_total_not_found", stats.TotalNotFound,
	)

	return nil
}

// ProcessPairs batches items from a slice of ArticleOwnerPair and sends them via ProcessBatch.
// If runningStats is provided, stats are accumulated into runningStats[0]; otherwise a new BackfillStats is created.
func ProcessPairs(
	ctx context.Context,
	client OwnerBackfillClient,
	pairs []ArticleOwnerPair,
	batchSize int,
	dryRun bool,
	log *slog.Logger,
	runningStats ...*BackfillStats,
) (*BackfillStats, error) {
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

	var stats *BackfillStats
	if len(runningStats) > 0 && runningStats[0] != nil {
		stats = runningStats[0]
	} else {
		stats = &BackfillStats{}
	}

	batch := make([]rag_gateway.OwnerBackfillItem, 0, batchSize)

	for _, pair := range pairs {
		if strings.TrimSpace(pair.ArticleID) == "" || strings.TrimSpace(pair.UserID) == "" {
			continue
		}
		batch = append(batch, rag_gateway.OwnerBackfillItem{
			ArticleId: pair.ArticleID,
			UserId:    pair.UserID,
		})

		if len(batch) >= batchSize {
			if err := ProcessBatch(ctx, client, batch, dryRun, log, stats); err != nil {
				return stats, err
			}
			batch = batch[:0]
		}
	}

	if len(batch) > 0 {
		if err := ProcessBatch(ctx, client, batch, dryRun, log, stats); err != nil {
			return stats, err
		}
	}

	return stats, nil
}

// ArticlePageFetcher defines the contract for fetching pages of article owner pairs.
type ArticlePageFetcher interface {
	FetchPage(ctx context.Context, startAfterID string, limit int) ([]ArticleOwnerPair, error)
}

type dbArticlePageFetcher struct {
	pool *pgxpool.Pool
}

func (f *dbArticlePageFetcher) FetchPage(ctx context.Context, startAfterID string, limit int) ([]ArticleOwnerPair, error) {
	lastID := strings.TrimSpace(startAfterID)
	if lastID == "" {
		lastID = "00000000-0000-0000-0000-000000000000"
	}

	query := `SELECT id::text, user_id::text FROM articles WHERE id > $1 AND user_id IS NOT NULL ORDER BY id LIMIT $2`
	rows, err := f.pool.Query(ctx, query, lastID, limit)
	if err != nil {
		return nil, fmt.Errorf("query articles: %w", err)
	}
	defer rows.Close()

	pairs := make([]ArticleOwnerPair, 0, limit)
	for rows.Next() {
		var articleID, userID string
		if err := rows.Scan(&articleID, &userID); err != nil {
			return nil, fmt.Errorf("scan article row: %w", err)
		}
		if strings.TrimSpace(articleID) == "" || strings.TrimSpace(userID) == "" {
			continue
		}
		pairs = append(pairs, ArticleOwnerPair{
			ArticleID: articleID,
			UserID:    userID,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate article rows: %w", err)
	}
	return pairs, nil
}

func streamAndBackfill(
	ctx context.Context,
	fetcher ArticlePageFetcher,
	client OwnerBackfillClient,
	batchSize int,
	startAfterID string,
	dryRun bool,
	log *slog.Logger,
) (*BackfillStats, error) {
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

	stats := &BackfillStats{}
	lastID := startAfterID

	for {
		pairs, err := fetcher.FetchPage(ctx, lastID, batchSize)
		if err != nil {
			return stats, fmt.Errorf("fetch article page: %w", err)
		}

		if len(pairs) == 0 {
			break
		}

		lastID = pairs[len(pairs)-1].ArticleID

		if _, err := ProcessPairs(ctx, client, pairs, batchSize, dryRun, log, stats); err != nil {
			return stats, err
		}

		if len(pairs) < batchSize {
			break
		}
	}

	return stats, nil
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	var dryRun bool
	var batchSize int
	var startAfterID string
	flag.BoolVar(&dryRun, "dry-run", false, "simulate backfill without making mutations")
	flag.IntVar(&batchSize, "batch-size", defaultBatchSize, "batch size for document owner updates")
	flag.StringVar(&startAfterID, "start-after-id", "", "article UUID to resume keyset pagination after")
	flag.Parse()

	if strings.TrimSpace(startAfterID) != "" {
		if _, err := uuid.Parse(strings.TrimSpace(startAfterID)); err != nil {
			log.Error("Invalid --start-after-id UUID", "id", startAfterID, "error", err)
			os.Exit(1)
		}
	}

	poolCfg, err := buildPGXPoolConfig()
	if err != nil {
		log.Error("Invalid database configuration", "error", err)
		os.Exit(1)
	}

	ragURL := resolveRAGOrchestratorURL()
	ragClient, err := newRAGClient(ragURL, log)
	if err != nil {
		log.Error("Unable to initialize rag-orchestrator client", "error", err, "url", ragURL)
		os.Exit(1)
	}

	ctx := context.Background()

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		log.Error("Unable to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	log.Info("Starting RAG document owners backfill",
		"dry_run", dryRun,
		"batch_size", batchSize,
		"start_after_id", startAfterID,
		"rag_url", ragURL,
	)

	fetcher := &dbArticlePageFetcher{pool: pool}
	stats, err := streamAndBackfill(ctx, fetcher, ragClient, batchSize, startAfterID, dryRun, log)
	if err != nil {
		log.Error("Backfill failed", "error", err, "stats", stats)
		os.Exit(1)
	}

	log.Info("RAG document owners backfill completed successfully",
		"total_processed", stats.TotalProcessed,
		"total_updated", stats.TotalUpdated,
		"total_already_set", stats.TotalAlreadySet,
		"total_not_found", stats.TotalNotFound,
		"dry_run", dryRun,
	)
}
