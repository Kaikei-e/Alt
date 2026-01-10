package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	connectv2 "search-indexer/connect/v2"
	"search-indexer/driver"
	"search-indexer/gateway"
	"search-indexer/logger"
	"search-indexer/rest"
	"search-indexer/tokenize"
	"search-indexer/usecase"
	"search-indexer/utils/otel"

	"github.com/meilisearch/meilisearch-go"
)

const (
	INDEX_INTERVAL       = 5 * time.Minute
	INDEX_BATCH_SIZE     = 200
	INDEX_RETRY_INTERVAL = 1 * time.Minute
	HTTP_ADDR            = ":9300"
	CONNECT_ADDR         = ":9301"
	DB_TIMEOUT           = 10 * time.Second
	MEILI_TIMEOUT        = 15 * time.Second
)

func main() {
	// ──────────── healthcheck subcommand ────────────
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}

	// ──────────── init OpenTelemetry ────────────
	ctx := context.Background()
	otelCfg := otel.ConfigFromEnv()
	otelShutdown, err := otel.InitProvider(ctx, otelCfg)
	if err != nil {
		fmt.Printf("Failed to initialize OpenTelemetry: %v\n", err)
		otelCfg.Enabled = false
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			fmt.Printf("Failed to shutdown OpenTelemetry: %v\n", err)
		}
	}()

	// ──────────── init logger ────────────
	logger.InitWithOTel(otelCfg.Enabled)
	tokenizer, err := tokenize.InitTokenizer()
	if err != nil {
		logger.Logger.Error("Failed to initialize tokenizer", "err", err)
	}

	logger.Logger.Info("Starting search-indexer",
		"service", otelCfg.ServiceName,
		"otel_enabled", otelCfg.Enabled,
	)

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	// Create drivers (infrastructure layer)
	dbDriver, err := driver.NewDatabaseDriverFromConfig(ctx)
	if err != nil {
		logger.Logger.Error("Failed to initialize database", "err", err)
		return
	}
	defer dbDriver.Close()

	// Initialize Meilisearch client
	msClient, err := initMeilisearchClient()
	if err != nil {
		logger.Logger.Error("Failed to initialize Meilisearch", "err", err)
		return
	}
	searchDriver := driver.NewMeilisearchDriver(msClient, "articles")

	// Create gateways (anti-corruption layer)
	articleRepo := gateway.NewArticleRepositoryGateway(dbDriver)
	searchEngine := gateway.NewSearchEngineGateway(searchDriver)

	// Ensure search index is properly set up
	if err := searchEngine.EnsureIndex(ctx); err != nil {
		logger.Logger.Error("Failed to ensure search index", "err", err)
		return
	}

	// Create use cases (application layer)
	indexUsecase := usecase.NewIndexArticlesUsecase(articleRepo, searchEngine, tokenizer)

	// ──────────── batch indexer ────────────
	go runIndexLoop(ctx, indexUsecase)

	// ──────────── HTTP server ────────────
	http.HandleFunc("/v1/search", func(w http.ResponseWriter, r *http.Request) {
		rest.SearchArticles(w, r, msClient.Index("articles"))
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})

	srv := &http.Server{
		Addr:              HTTP_ADDR,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Logger.Info("http listen", "addr", HTTP_ADDR)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Logger.Error("http", "err", err)
		}
	}()

	// ──────────── Connect-RPC server ────────────
	connectServer := connectv2.CreateConnectServer(msClient.Index("articles"))
	connectSrv := &http.Server{
		Addr:              CONNECT_ADDR,
		Handler:           connectServer,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Logger.Info("connect-rpc listen", "addr", CONNECT_ADDR)
		if err := connectSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Logger.Error("connect-rpc", "err", err)
		}
	}()

	<-ctx.Done() // Ctrl-C
	_ = srv.Shutdown(context.Background())
	_ = connectSrv.Shutdown(context.Background())
}

// initMeilisearchClient initializes the Meilisearch client with retry logic
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

		// Test the connection by checking if Meilisearch is healthy
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

// runIndexLoop runs the dual-phase indexing loop using clean architecture
// Phase 1 (Backfill): Index all existing articles from latest to oldest
// Phase 2 (Incremental): Poll for new articles and sync deletions
func runIndexLoop(ctx context.Context, indexUsecase *usecase.IndexArticlesUsecase) {
	defer func() {
		if r := recover(); r != nil {
			logger.Logger.Error("index loop panic", "err", r)
		}
	}()

	// Phase 1: Backfill
	var lastCreatedAt *time.Time
	var lastID string
	var incrementalMark *time.Time

	logger.Logger.Info("starting Phase 1: Backfill")

	// Get incrementalMark (latest created_at) at the start
	mark, err := indexUsecase.GetIncrementalMark(ctx)
	if err != nil {
		logger.Logger.Error("failed to get incremental mark", "err", err)
		// Use current time as fallback
		now := time.Now()
		incrementalMark = &now
		logger.Logger.Info("using current time as incremental mark fallback", "mark", incrementalMark)
	} else if mark == nil {
		// No articles exist yet, use current time as fallback
		now := time.Now()
		incrementalMark = &now
		logger.Logger.Info("no articles found, using current time as incremental mark", "mark", incrementalMark)
	} else {
		incrementalMark = mark
		logger.Logger.Info("incremental mark set", "mark", incrementalMark)
	}

	// Phase 1: Backfill loop (past direction)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		result, err := indexUsecase.ExecuteBackfill(ctx, lastCreatedAt, lastID, INDEX_BATCH_SIZE)
		if err != nil {
			logger.Logger.Error("backfill error", "err", err)
			time.Sleep(INDEX_RETRY_INTERVAL)
			continue
		}

		if result.IndexedCount == 0 {
			logger.Logger.Info("Phase 1 complete: backfill done")
			break
		}

		logger.Logger.Info("backfill indexed", "count", result.IndexedCount)
		lastCreatedAt = result.LastCreatedAt
		lastID = result.LastID
	}

	// Phase 2: Incremental loop (future direction + deletion sync)
	logger.Logger.Info("starting Phase 2: Incremental")

	// Reset cursor for incremental phase
	lastCreatedAt = nil
	lastID = ""
	var lastDeletedAt *time.Time

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		result, err := indexUsecase.ExecuteIncremental(ctx, incrementalMark, lastCreatedAt, lastID, lastDeletedAt, INDEX_BATCH_SIZE)
		if err != nil {
			logger.Logger.Error("incremental indexing error", "err", err)
			time.Sleep(INDEX_RETRY_INTERVAL)
			continue
		}

		if result.IndexedCount > 0 {
			logger.Logger.Info("incremental indexed", "count", result.IndexedCount)
			lastCreatedAt = result.LastCreatedAt
			lastID = result.LastID
		}

		if result.DeletedCount > 0 {
			logger.Logger.Info("deleted from index", "count", result.DeletedCount)
			lastDeletedAt = result.LastDeletedAt
		}

		if result.IndexedCount == 0 && result.DeletedCount == 0 {
			logger.Logger.Info("no new articles or deletions")
		}

		time.Sleep(INDEX_INTERVAL)
	}
}

// runHealthcheck performs a health check against the local HTTP server.
// Returns 0 on success, 1 on failure.
func runHealthcheck() int {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://127.0.0.1" + HTTP_ADDR + "/health")
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return 0
	}
	fmt.Fprintf(os.Stderr, "healthcheck failed: status %d\n", resp.StatusCode)
	return 1
}
