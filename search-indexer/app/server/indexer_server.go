package server

import (
	"context"
	"fmt"
	"net/http"
	"time"
	"search-indexer/port"
	"search-indexer/usecase"
)

type IndexerServer struct {
	indexUsecase  *usecase.IndexArticlesUsecase
	searchUsecase *usecase.SearchArticlesUsecase
	searchEngine  port.SearchEngine
	httpServer    *http.Server
}

type Config struct {
	HTTPAddr         string
	IndexInterval    time.Duration
	IndexBatchSize   int
	IndexRetryDelay  time.Duration
	DBTimeout        time.Duration
	MeiliTimeout     time.Duration
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
	var lastCreatedAt *time.Time
	var lastID string

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		result, err := s.indexUsecase.Execute(ctx, lastCreatedAt, lastID, config.IndexBatchSize)
		if err != nil {
			fmt.Printf("Index error: %v\n", err)
			time.Sleep(config.IndexRetryDelay)
			continue
		}

		if result.IndexedCount == 0 {
			time.Sleep(config.IndexInterval)
			continue
		}

		fmt.Printf("Indexed %d articles\n", result.IndexedCount)
		lastCreatedAt = result.LastCreatedAt
		lastID = result.LastID
	}
}