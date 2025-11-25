package server

import (
	"context"
	"fmt"
	"net/http"
	"search-indexer/port"
	"search-indexer/usecase"
	"time"
)

type IndexerServer struct {
	indexUsecase  *usecase.IndexArticlesUsecase
	searchUsecase *usecase.SearchArticlesUsecase
	searchEngine  port.SearchEngine
	httpServer    *http.Server
}

type Config struct {
	HTTPAddr        string
	IndexInterval   time.Duration
	IndexBatchSize  int
	IndexRetryDelay time.Duration
	DBTimeout       time.Duration
	MeiliTimeout    time.Duration
}

func NewIndexerServer(
	indexUsecase *usecase.IndexArticlesUsecase,
	searchUsecase *usecase.SearchArticlesUsecase,
	searchEngine port.SearchEngine,
	config Config,
) *IndexerServer {
	return &IndexerServer{
		indexUsecase:  indexUsecase,
		searchUsecase: searchUsecase,
		searchEngine:  searchEngine,
		httpServer: &http.Server{
			Addr:              config.HTTPAddr,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

func (s *IndexerServer) Start(ctx context.Context, config Config) error {
	if err := s.searchEngine.EnsureIndex(ctx); err != nil {
		return fmt.Errorf("failed to ensure search index: %w", err)
	}

	go s.runIndexLoop(ctx, config)

	s.setupRoutes()

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("HTTP server error: %v\n", err)
		}
	}()

	return nil
}

func (s *IndexerServer) Stop(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *IndexerServer) setupRoutes() {
	http.HandleFunc("/v1/search", s.handleSearch)
}

func (s *IndexerServer) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	limit := 20
	if limitParam := r.URL.Query().Get("limit"); limitParam != "" {
		fmt.Sscanf(limitParam, "%d", &limit)
	}

	result, err := s.searchUsecase.Execute(r.Context(), query, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"results": %d, "query": "%s"}`, len(result.Documents), query)
}

func (s *IndexerServer) runIndexLoop(ctx context.Context, config Config) {
	// Phase 1: Backfill
	var lastCreatedAt *time.Time
	var lastID string
	var incrementalMark *time.Time

	fmt.Println("Starting Phase 1: Backfill")

	// Get incrementalMark (latest created_at) at the start
	mark, err := s.indexUsecase.GetIncrementalMark(ctx)
	if err != nil {
		fmt.Printf("Failed to get incremental mark: %v\n", err)
		// Use current time as fallback
		now := time.Now()
		incrementalMark = &now
		fmt.Printf("Using current time as incremental mark fallback: %v\n", incrementalMark)
	} else if mark == nil {
		// No articles exist yet, use current time as fallback
		now := time.Now()
		incrementalMark = &now
		fmt.Printf("No articles found, using current time as incremental mark: %v\n", incrementalMark)
	} else {
		incrementalMark = mark
		fmt.Printf("Incremental mark set: %v\n", incrementalMark)
	}

	// Phase 1: Backfill loop (past direction)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		result, err := s.indexUsecase.ExecuteBackfill(ctx, lastCreatedAt, lastID, config.IndexBatchSize)
		if err != nil {
			fmt.Printf("Backfill error: %v\n", err)
			time.Sleep(config.IndexRetryDelay)
			continue
		}

		if result.IndexedCount == 0 {
			fmt.Println("Phase 1 complete: backfill done")
			break
		}

		fmt.Printf("Backfill indexed %d articles\n", result.IndexedCount)
		lastCreatedAt = result.LastCreatedAt
		lastID = result.LastID
	}

	// Phase 2: Incremental loop (future direction + deletion sync)
	fmt.Println("Starting Phase 2: Incremental")

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

		result, err := s.indexUsecase.ExecuteIncremental(ctx, incrementalMark, lastCreatedAt, lastID, lastDeletedAt, config.IndexBatchSize)
		if err != nil {
			fmt.Printf("Incremental indexing error: %v\n", err)
			time.Sleep(config.IndexRetryDelay)
			continue
		}

		if result.IndexedCount > 0 {
			fmt.Printf("Incremental indexed %d articles\n", result.IndexedCount)
			lastCreatedAt = result.LastCreatedAt
			lastID = result.LastID
		}

		if result.DeletedCount > 0 {
			fmt.Printf("Deleted %d articles from index\n", result.DeletedCount)
			lastDeletedAt = result.LastDeletedAt
		}

		if result.IndexedCount == 0 && result.DeletedCount == 0 {
			fmt.Println("No new articles or deletions")
		}

		time.Sleep(config.IndexInterval)
	}
}
