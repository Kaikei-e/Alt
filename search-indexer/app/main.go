package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"search-indexer/driver"
	"search-indexer/gateway"
	"search-indexer/logger"
	"search-indexer/rest"
	"search-indexer/usecase"

	"github.com/meilisearch/meilisearch-go"
)

const (
	INDEX_INTERVAL       = 1 * time.Minute
	INDEX_BATCH_SIZE     = 200
	INDEX_RETRY_INTERVAL = 1 * time.Minute
	HTTP_ADDR            = ":9300"
	DB_TIMEOUT           = 10 * time.Second
	MEILI_TIMEOUT        = 15 * time.Second
)

func main() {
	// ──────────── init ────────────
	logger.Init()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
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
	indexUsecase := usecase.NewIndexArticlesUsecase(articleRepo, searchEngine)

	// ──────────── batch indexer ────────────
	go runIndexLoop(ctx, indexUsecase)

	// ──────────── HTTP server ────────────
	http.HandleFunc("/v1/search", func(w http.ResponseWriter, r *http.Request) {
		rest.SearchArticles(w, r, msClient.Index("articles"))
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

	<-ctx.Done() // Ctrl-C
	_ = srv.Shutdown(context.Background())
}

// initMeilisearchClient initializes the Meilisearch client with retry logic
func initMeilisearchClient() (meilisearch.ServiceManager, error) {
	const maxRetries = 5
	const retryDelay = 5 * time.Second

	meilisearchHost := os.Getenv("MEILISEARCH_HOST")
	meilisearchKey := os.Getenv("MEILISEARCH_API_KEY")

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

// runIndexLoop runs the indexing loop using clean architecture
func runIndexLoop(ctx context.Context, indexUsecase *usecase.IndexArticlesUsecase) {
	defer func() {
		if r := recover(); r != nil {
			logger.Logger.Error("index loop panic", "err", r)
		}
	}()

	var lastCreatedAt *time.Time
	var lastID string

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		result, err := indexUsecase.Execute(ctx, lastCreatedAt, lastID, INDEX_BATCH_SIZE)
		if err != nil {
			logger.Logger.Error("indexing error", "err", err)
			time.Sleep(INDEX_RETRY_INTERVAL)
			continue
		}

		if result.IndexedCount == 0 {
			logger.Logger.Info("no new articles")
			time.Sleep(INDEX_INTERVAL)
			continue
		}

		logger.Logger.Info("indexed", "count", result.IndexedCount)
		lastCreatedAt = result.LastCreatedAt
		lastID = result.LastID
	}
}
