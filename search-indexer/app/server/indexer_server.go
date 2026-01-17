package server

import (
	"context"
	"fmt"
	"log/slog"
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
			slog.Error("http server error", "err", err)
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
		_, _ = fmt.Sscanf(limitParam, "%d", &limit)
	}

	result, err := s.searchUsecase.Execute(r.Context(), query, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"results": %d, "query": "%s"}`, len(result.Documents), query)
}

func (s *IndexerServer) runIndexLoop(ctx context.Context, config Config) {
	// Phase 1: Backfill
	var lastCreatedAt *time.Time
	var lastID string
	var incrementalMark *time.Time

	slog.InfoContext(ctx, "starting Phase 1: Backfill")

	// Get incrementalMark (latest created_at) at the start
	mark, err := s.indexUsecase.GetIncrementalMark(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to get incremental mark", "err", err)
		// Use current time as fallback
		now := time.Now()
		incrementalMark = &now
		slog.InfoContext(ctx, "using current time as incremental mark fallback", "mark", incrementalMark)
	} else if mark == nil {
		// No articles exist yet, use current time as fallback
		now := time.Now()
		incrementalMark = &now
		slog.InfoContext(ctx, "no articles found, using current time as incremental mark", "mark", incrementalMark)
	} else {
		incrementalMark = mark
		slog.InfoContext(ctx, "incremental mark set", "mark", incrementalMark)
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
			slog.ErrorContext(ctx, "backfill error", "err", err)
			time.Sleep(config.IndexRetryDelay)
			continue
		}

		if result.IndexedCount == 0 {
			slog.InfoContext(ctx, "Phase 1 complete: backfill done")
			break
		}

		slog.InfoContext(ctx, "backfill indexed", "count", result.IndexedCount)
		lastCreatedAt = result.LastCreatedAt
		lastID = result.LastID
	}

	// Phase 2: Incremental loop (future direction + deletion sync)
	slog.InfoContext(ctx, "starting Phase 2: Incremental")

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
			slog.ErrorContext(ctx, "incremental indexing error", "err", err)
			time.Sleep(config.IndexRetryDelay)
			continue
		}

		if result.IndexedCount > 0 {
			slog.InfoContext(ctx, "incremental indexed", "count", result.IndexedCount)
			lastCreatedAt = result.LastCreatedAt
			lastID = result.LastID
		}

		if result.DeletedCount > 0 {
			slog.InfoContext(ctx, "deleted from index", "count", result.DeletedCount)
			lastDeletedAt = result.LastDeletedAt
		}

		if result.IndexedCount == 0 && result.DeletedCount == 0 {
			slog.InfoContext(ctx, "no new articles or deletions")
		}

		time.Sleep(config.IndexInterval)
	}
}
